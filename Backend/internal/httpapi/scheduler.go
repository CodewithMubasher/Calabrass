package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/ai"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

const (
	// scheduleTick is how often the loop looks for work that has come due. The
	// timer is only as exact as this, which is the right trade for a task the
	// user thinks of in minutes.
	scheduleTick = 5 * time.Second
	// scheduleDrain is how many tasks one pass runs, so a backlog cannot hold
	// the loop open indefinitely.
	scheduleDrain = 5
	// scheduleGrace is how late a task may still run. A machine that was off
	// when its time came — a laptop shut down at ten, opened again at noon —
	// must not shut itself down the moment it wakes, so anything older than
	// this is recorded as missed instead.
	scheduleGrace = time.Hour
)

// RunScheduler fires the due tasks until ctx is done. main starts it with the
// process, so the loop lives and dies with the server rather than with a
// request.
func (s *Server) RunScheduler(ctx context.Context) {
	ticker := time.NewTicker(scheduleTick)
	defer ticker.Stop()

	// A task booked and due before the first tick, or one whose time passed
	// while the process was down, is picked up on the tick that follows. An
	// immediate pass is what makes a task due in the next few seconds sane.
	s.runDueTasks(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runDueTasks(ctx)
		}
	}
}

// runDueTasks runs what is due, and writes off what is far too late.
func (s *Server) runDueTasks(ctx context.Context) {
	now := time.Now()
	tasks, err := s.store.DueScheduledTasks(ctx, now.UnixMilli(), scheduleDrain)
	if err != nil {
		s.log.Error("could not read the scheduled tasks", "error", err)
		return
	}

	for _, task := range tasks {
		if now.Sub(time.UnixMilli(task.RunAt)) > scheduleGrace {
			s.log.Info("a scheduled task was missed", "task", task.ID)
			s.recordRun(ctx, task, "missed",
				"this ran more than "+scheduleGrace.String()+" after its time, so it was not run")
			continue
		}
		if err := s.runScheduledTask(ctx, task); err != nil {
			s.log.Warn("a scheduled task failed", "task", task.ID, "error", err)
		}
	}
}

// runScheduledTask runs one task's instruction as a turn of its own: the same
// harness a chat gets, tools and all, so a task that was booked to shut the
// laptop down can still do it. The question and the answer are written into the
// conversation, which is where the user reads them.
func (s *Server) runScheduledTask(ctx context.Context, task store.ScheduledTask) error {
	chatID := task.ChatID
	if chatID == "" {
		// A task booked from the Schedules page has no conversation yet. It
		// gets one, named after what it was asked to do, so the answer has
		// somewhere to land and the user can read what it did.
		created, err := s.store.CreateChat(ctx, "", "", "")
		if err != nil {
			s.recordRun(ctx, task, "error", "could not open a conversation: "+err.Error())
			return err
		}
		chatID = created.ID
		if _, err := s.store.UpdateChat(ctx, chatID, titleFrom(task.Prompt), "", ""); err != nil {
			s.log.Warn("could not title the task's conversation", "chat", chatID, "error", err)
		}
		if err := s.store.BindScheduledTaskChat(ctx, task.ID, chatID); err != nil {
			s.log.Warn("could not remember the task's conversation", "task", task.ID, "error", err)
		}
	}

	chat, err := s.store.GetChat(ctx, chatID)
	if err != nil {
		s.recordRun(ctx, task, "error", "the conversation is gone: "+err.Error())
		return err
	}

	providerID, modelID := s.resolveModel(ctx, chat.Provider, chat.Model)
	if providerID == "" || modelID == "" {
		err := errors.New("no model is available")
		s.recordRun(ctx, task, "error", err.Error())
		return err
	}

	// The instruction is a user turn: that is what the model is being asked to
	// act on, and what the user sees above the answer.
	if _, err := s.store.AddMessage(ctx, store.Message{
		ChatID:  chatID,
		Role:    ai.RoleUser,
		Content: task.Prompt,
	}); err != nil {
		s.recordRun(ctx, task, "error", "could not write the instruction: "+err.Error())
		return err
	}
	if _, err := s.store.UpdateChat(ctx, chatID, "", providerID, modelID); err != nil {
		s.log.Warn("could not remember the task's model", "chat", chatID, "error", err)
	}

	turns := make([]ai.Message, 0, len(chat.Messages)+1)
	for _, m := range chat.Messages {
		turns = append(turns, ai.Message{Role: m.Role, Content: m.Content, Replay: m.Replay})
	}
	turns = append(turns, ai.Message{Role: ai.RoleUser, Content: task.Prompt})

	setup := s.setupTurn(ctx, s.workspaceFor(chat.Chat), chatID)

	started := time.Now()
	reply, genErr := s.registry.Stream(ctx, ai.StreamRequest{
		ProviderID: providerID,
		Model:      modelID,
		System:     setup.system,
		Messages:   turns,
		Tools:      setup.tools,
		MaxSteps:   agent.DefaultMaxSteps,
	}, ai.Handlers{})
	elapsed := time.Since(started)

	content := reply.Text
	if answer := reply.Trace.AnswerText(); strings.TrimSpace(answer) != "" {
		content = answer
	}
	if strings.TrimSpace(content) == "" && genErr != nil {
		content = "The scheduled task could not run: " + genErr.Error()
	}

	var savedTrace json.RawMessage
	if len(reply.Trace.Steps) > 0 {
		if data, err := json.Marshal(reply.Trace); err == nil {
			savedTrace = data
		}
	}

	// Persist whatever came back, on the same terms a chat does: a turn that
	// produced nothing at all is not worth a message.
	if strings.TrimSpace(content) != "" || strings.TrimSpace(reply.Reasoning) != "" || len(savedTrace) > 0 {
		// Detached from the loop's context: a task that was interrupted is
		// still worth recording, and a shutdown must not lose an answer that
		// already arrived.
		saveCtx := context.WithoutCancel(ctx)
		if _, err := s.store.AddMessage(saveCtx, store.Message{
			ChatID:       chatID,
			Role:         ai.RoleAssistant,
			Content:      content,
			Reasoning:    reply.Reasoning,
			ThinkingMs:   reply.ThinkingMs,
			OutputTokens: int64(reply.Usage.OutputTokens),
			DurationMs:   elapsed.Milliseconds(),
			Trace:        savedTrace,
			Replay:       reply.Replay,
		}); err != nil {
			s.log.Error("could not persist a scheduled reply", "task", task.ID, "error", err)
		}
	}

	s.recordRun(ctx, task, statusFor(genErr), errorText(genErr))
	return genErr
}

// recordRun writes down how a task went, so the page can say.
func (s *Server) recordRun(ctx context.Context, task store.ScheduledTask, status, message string) {
	if err := s.store.MarkScheduledTask(
		context.WithoutCancel(ctx), task.ID, time.Now().UnixMilli(), status, message,
	); err != nil {
		s.log.Error("could not record a scheduled run", "task", task.ID, "error", err)
	}
}

func statusFor(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
