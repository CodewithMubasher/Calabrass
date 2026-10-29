package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	goai "github.com/zendev-sh/goai"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/ai"
	"github.com/CodewithMubasher/Calabrass/backend/internal/crew"
	"github.com/CodewithMubasher/Calabrass/backend/internal/permission"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// The crew's work lives on disk, one file per step: the Orchestrator writes a
// todo before it hands work out, and the worker writes a report when it is done.
// The rail watches that folder the way the artifacts rail watches the workspace,
// which is what makes the panel open by itself when there is something to see.

// crewPollInterval is how often the crew folder is looked at. It matches the
// artifacts rail: a file a worker has just written is on screen by the time its
// reply lands.
const crewPollInterval = 400 * time.Millisecond

// handleCrewStream follows one workspace's .crew folder and re-sends its
// listing whenever anything in it changes. The first snapshot goes out with the
// connection, so opening the panel has no loading state.
func (s *Server) handleCrewStream(w http.ResponseWriter, r *http.Request) {
	chatID := strings.TrimSpace(r.URL.Query().Get("chatId"))
	ws, _, err := s.artifactsWorkspace(r.Context(), currentUser(r.Context()).ID, chatID)
	if err != nil {
		writeError(w, artifactStatus(err), err)
		return
	}

	stream, err := newSSEWriter(w)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	ticker := time.NewTicker(crewPollInterval)
	defer ticker.Stop()

	sent := false
	var last string
	for {
		if files, signature := scanCrew(ws.Root(), chatID); !sent || signature != last {
			sent, last = true, signature
			if err := stream.event("crew", artifactSnapshot{Files: files}); err != nil {
				return
			}
		}

		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

// scanCrew lists one chat's JSON crew files, newest first, and returns a
// signature of everything the listing shows. It reads only .crew/chats/<chat>,
// so a panel never shows another conversation's todo or report. Paths stay
// relative to the workspace root, because that is what the artifact detail
// reads them by.
func scanCrew(root, chatID string) ([]artifactPayload, string) {
	crewRoot := agent.CrewRoot(root, chatID)
	files := make([]artifactPayload, 0, 16)
	var signature strings.Builder

	_ = filepath.WalkDir(crewRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			// A folder that cannot be read is not a reason to fail the listing.
			return nil
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		name := entry.Name()
		files = append(files, artifactPayload{
			Path:      rel,
			Name:      name,
			Extension: strings.TrimPrefix(strings.ToUpper(filepath.Ext(name)), "."),
			Size:      info.Size(),
			Modified:  info.ModTime().UnixMilli(),
		})
		fmt.Fprintf(&signature, "%s\x00%d\x00%d\n", rel, info.Size(), info.ModTime().UnixMilli())

		if len(files) >= artifactListMax {
			return filepath.SkipAll
		}
		return nil
	})

	sort.Slice(files, func(i, j int) bool {
		if files[i].Modified != files[j].Modified {
			return files[i].Modified > files[j].Modified
		}
		return files[i].Path < files[j].Path
	})
	return files, signature.String()
}

// crewDelegateRequest is the body of POST /api/crew/delegate. The Orchestrator's
// tool calls delegateTask directly, so this endpoint exists for anything else
// that wants to start the same run.
type crewDelegateRequest struct {
	ChatID   string `json:"chatId"`
	WorkerID string `json:"workerId"`
	Task     string `json:"task"`
	Context  string `json:"context"`
}

// handleCrewDelegate hands one task to one worker and answers with its report.
func (s *Server) handleCrewDelegate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())

	var req crewDelegateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	chatID := strings.TrimSpace(req.ChatID)
	if chatID == "" {
		writeError(w, http.StatusBadRequest, errors.New("chatId is required"))
		return
	}

	chat, err := s.store.GetChat(r.Context(), chatID, user.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, errors.New("chat not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if chat.Mode != store.ModeCrew {
		writeError(w, http.StatusBadRequest, errors.New("delegation is only available in crew mode"))
		return
	}

	workspace := s.workspaceForUser(user.ID, chat.Chat)
	if workspace == nil {
		writeError(w, http.StatusServiceUnavailable, errArtifactsUnavailable)
		return
	}

	// Nobody is streaming this request, but a person started it: what a worker
	// needs approval for is listed for them at /api/approvals.
	delegateCtx := permission.WithSession(r.Context(), s.interactiveSession(user.ID, chatID, workspace.Root(), nil))
	report, err := s.delegateTask(delegateCtx, user.ID, chatID, workspace.Root(), agent.DelegateTaskArgs{
		WorkerID: req.WorkerID,
		Task:     req.Task,
		Context:  req.Context,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": report})
}

// crewLock returns the mutex for one chat's crew work, creating it on first
// use. The Orchestrator is told to hand out one task at a time, but a model can
// still ask for two in one step; this is what makes that impossible to run in
// parallel.
func (s *Server) crewLock(chatID string) *sync.Mutex {
	s.crewMu.Lock()
	defer s.crewMu.Unlock()

	if s.crewLocks == nil {
		s.crewLocks = make(map[string]*sync.Mutex)
	}
	lock, ok := s.crewLocks[chatID]
	if !ok {
		lock = &sync.Mutex{}
		s.crewLocks[chatID] = lock
	}
	return lock
}

// workerByLooseName finds a worker by its name, or by its id without the
// built-in prefix. A model often writes "Frontend Developer" or
// "frontend-developer" when the exact value is "builtin-frontend-developer";
// that is a naming difference, not a reason to stop the delegation.
func (s *Server) workerByLooseName(ctx context.Context, userID, want string) (string, bool) {
	want = strings.Trim(strings.ToLower(strings.TrimSpace(want)), `.,;:"'`)
	if want == "" {
		return "", false
	}
	wantSlug := strings.ReplaceAll(want, " ", "-")

	workers, err := s.workerSummaries(ctx, userID)
	if err != nil {
		return "", false
	}
	for _, worker := range workers {
		id := strings.ToLower(strings.TrimSpace(worker.ID))
		name := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(worker.Name)), " ", "-")
		if id == want || name == wantSlug || name == want ||
			strings.TrimPrefix(id, crew.BuiltinIDPrefix) == wantSlug {
			return worker.ID, true
		}
	}
	return "", false
}

// delegateTask writes the todo, runs the worker, and makes sure a report is on
// disk either way. A worker that failed is still a finished delegation: the
// report says so and the Orchestrator is told, rather than the whole turn
// collapsing into an error the user cannot read.
func (s *Server) delegateTask(ctx context.Context, userID, chatID, workspaceRoot string, args agent.DelegateTaskArgs) (string, error) {
	workerID := strings.TrimSpace(args.WorkerID)
	if workerID == "" {
		workers, err := s.workerSummaries(ctx, userID)
		if err != nil {
			return "", err
		}
		switch len(workers) {
		case 0:
			return "", errors.New("a workerId is required, but there are no workers available")
		case 1:
			// With one worker there is nothing to choose. A model that drops
			// the id still gets the work done instead of stopping on a naming
			// detail.
			workerID = workers[0].ID
		default:
			return "", fmt.Errorf("a workerId is required; choose one of: %s", agent.WorkerChoices(workers))
		}
	}
	if strings.TrimSpace(args.Task) == "" {
		return "", errors.New("a task is required")
	}

	worker, _, err := s.resolveAgent(ctx, userID, workerID)
	if errors.Is(err, store.ErrNotFound) {
		// A model may write the worker's name, or its id without the built-in
		// prefix, instead of the exact id. Resolve those before failing.
		if canonical, ok := s.workerByLooseName(ctx, userID, workerID); ok {
			workerID = canonical
			worker, _, err = s.resolveAgent(ctx, userID, workerID)
		}
	}
	if errors.Is(err, store.ErrNotFound) {
		workers, _ := s.workerSummaries(ctx, userID)
		if choices := agent.WorkerChoices(workers); choices != "" {
			return "", fmt.Errorf("there is no worker with id %q; choose one of: %s", workerID, choices)
		}
		return "", fmt.Errorf("there is no worker with id %q", workerID)
	}
	if err != nil {
		return "", err
	}
	if crew.IsOrchestrator(worker.ID) {
		return "", errors.New("the Orchestrator cannot delegate to itself")
	}

	// One worker at a time per chat, even if the model asked for two.
	lock := s.crewLock(chatID)
	lock.Lock()
	defer lock.Unlock()

	task := strings.TrimSpace(args.Task)
	taskContext := strings.TrimSpace(args.Context)
	acceptance := trimNonEmpty(args.Acceptance)

	// The journal is the durable memory of this chat's crew work. Before a new
	// delegation is written, check whether the same work is already done or
	// has already failed in exactly the same way. This is what stops an
	// orchestrator from re-delegating a completed task because the report fell
	// out of its context.
	journalPath := agent.CrewJournalPath(workspaceRoot, chatID)
	journal := agent.ReadCrewJournal(journalPath, 200)
	if completed, failed := agent.CrewDuplicates(journal, worker.ID, task, taskContext); completed != nil {
		reason := fmt.Sprintf(
			"this task is already recorded as completed by %s at %s: %s. Do not delegate it again; use the existing report and move on to the next piece of the goal.",
			worker.Name, completed.Time, truncate(completed.Summary, 300))
		_ = agent.AppendCrewJournal(journalPath, agent.CrewJournalEntry{
			Kind:     agent.JournalGuard,
			WorkerID: worker.ID,
			Task:     task,
			Reason:   reason,
		})
		return "", errors.New(reason)
	} else if failed != nil {
		detail := failed.Error
		if strings.TrimSpace(detail) == "" {
			detail = failed.Summary
		}
		reason := fmt.Sprintf(
			"this exact task already failed for %s at %s: %s. Change the task or give the worker new context that addresses the failure; do not repeat it unchanged.",
			worker.Name, failed.Time, truncate(detail, 300))
		_ = agent.AppendCrewJournal(journalPath, agent.CrewJournalEntry{
			Kind:     agent.JournalGuard,
			WorkerID: worker.ID,
			Task:     task,
			Context:  taskContext,
			Reason:   reason,
		})
		return "", errors.New(reason)
	}

	orchestratorDir, workersDir := agent.CrewDirs(workspaceRoot, chatID)
	reportPath := agent.DelegateTaskReportPath(workersDir, worker.ID)
	if err := os.MkdirAll(orchestratorDir, 0o755); err != nil {
		return "", fmt.Errorf("create the crew folder: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return "", fmt.Errorf("create the worker folder: %w", err)
	}

	todo := agent.DelegateTaskTodo{
		WorkerID:   worker.ID,
		WorkerName: worker.Name,
		Task:       task,
		Context:    taskContext,
		Acceptance: acceptance,
		Workspace:  workspaceRoot,
		CreatedAt:  agent.NowRFC3339(),
		TodoPath:   agent.DelegateTaskTodoRel(chatID, worker.ID),
		ReportPath: agent.DelegateTaskReportRel(chatID, worker.ID),
	}
	todoBytes, err := agent.MarshalIndentJSON(todo)
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(agent.DelegateTaskTodoPath(orchestratorDir, worker.ID), todoBytes); err != nil {
		return "", fmt.Errorf("write the todo: %w", err)
	}
	// The todo is journaled before the worker starts, so an interrupted run
	// leaves evidence that this delegation was begun and never reported.
	s.appendJournal(journalPath, agent.CrewJournalEntry{
		Kind:       agent.JournalTodo,
		WorkerID:   worker.ID,
		WorkerName: worker.Name,
		Task:       todo.Task,
		Context:    todo.Context,
		Acceptance: todo.Acceptance,
	})

	report := s.runWorker(ctx, userID, chatID, workspaceRoot, worker, todo)
	report.FinishedAt = agent.NowRFC3339()

	reportBytes, err := agent.MarshalIndentJSON(report)
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(reportPath, reportBytes); err != nil {
		return "", fmt.Errorf("write the report: %w", err)
	}
	s.appendJournal(journalPath, agent.CrewJournalEntry{
		Kind:         agent.JournalReport,
		WorkerID:     report.WorkerID,
		WorkerName:   report.WorkerName,
		Task:         todo.Task,
		Context:      todo.Context,
		Acceptance:   todo.Acceptance,
		Status:       report.Status,
		Summary:      report.Summary,
		FilesChanged: report.FilesChanged,
		Checks:       report.Checks,
		Unresolved:   report.Unresolved,
		Error:        report.Error,
	})

	return string(reportBytes), nil
}

// appendJournal records crew state without letting a journal write failure
// discard the work that was actually done.
func (s *Server) appendJournal(path string, entry agent.CrewJournalEntry) {
	if err := agent.AppendCrewJournal(path, entry); err != nil {
		s.log.Warn("could not append the crew journal", "path", path, "error", err)
	}
}

func trimNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// runWorker is one worker run: its prompt, its tools, its skills, and the
// report it is required to end with. It never returns an error for a model or
// tool failure — those become a failed report, because the Orchestrator needs to
// read what went wrong rather than lose the turn.
func (s *Server) runWorker(ctx context.Context, userID, chatID, workspaceRoot string, worker store.AgentRecord, todo agent.DelegateTaskTodo) agent.DelegateTaskReport {
	report := agent.DelegateTaskReport{
		WorkerID:   worker.ID,
		WorkerName: worker.Name,
		Status:     agent.ReportFailed,
		Summary:    "The worker did not finish.",
		StartedAt:  agent.NowRFC3339(),
	}

	fail := func(reason string) agent.DelegateTaskReport {
		report.Status = agent.ReportFailed
		report.Error = reason
		if strings.TrimSpace(report.Summary) == "" {
			report.Summary = "The worker did not finish."
		}
		return report
	}

	workspace, err := agent.NewWorkspace(workspaceRoot)
	if err != nil {
		return fail("The workspace could not be opened: " + err.Error())
	}

	providerID, modelID := s.resolveModelFor(ctx, userID, worker.Provider, worker.Model)
	altProviderID, altModelID := s.resolveModelFor(ctx, userID, worker.AltProvider, worker.AltModel)
	if providerID == "" || modelID == "" {
		providerID, modelID = altProviderID, altModelID
		altProviderID, altModelID = "", ""
	}
	if providerID == "" || modelID == "" {
		return fail("No model is available for this worker. Pick one in the agent's settings.")
	}
	fallbackAvailable := (altProviderID != "" && altModelID != "") &&
		(altProviderID != providerID || altModelID != modelID)

	setup := s.setupTurnFor(ctx, userID, workspace, chatID, worker.ID)

	// A worker stops after two failures rather than retrying the same thing.
	// The prompt says so and the loop guard enforces it: the third call is
	// refused before it runs, which ends the run with a reportable error.
	if setup.runState != nil {
		setup.runState.SetMaxConsecutiveFail(2)
	}

	// A worker does not ask the user questions attached to a turn and does not
	// schedule its own follow-up. It reads, changes the project, and reports.
	tools := make([]goai.Tool, 0, len(setup.tools)+2)
	for _, tool := range setup.tools {
		switch tool.Name {
		case "ask_questions", "schedule_task", "delegate_task":
			continue
		}
		tools = append(tools, tool)
	}
	tools = append(tools, agent.ReadSkillTool())

	var reported *agent.DelegateTaskReport
	tools = append(tools, agent.ReportTaskTool(func(args agent.ReportTaskArgs) (string, error) {
		if reported != nil {
			return "", errors.New("the report was already sent; stop and end the task")
		}
		rep := agent.DelegateTaskReport{
			WorkerID:     worker.ID,
			WorkerName:   worker.Name,
			Status:       args.Status,
			Summary:      args.Summary,
			FilesChanged: args.FilesChanged,
			Checks:       args.Checks,
			Unresolved:   args.Unresolved,
			Error:        args.Error,
			StartedAt:    report.StartedAt,
		}
		if err := agent.ValidateReport(&rep); err != nil {
			return "", err
		}
		reported = &rep
		return "Report received. Stop now.", nil
	}))

	system := s.workerSystemPrompt(ctx, chatID, worker, todo, workspace, tools)

	run := func(provider, model string) (ai.Reply, error) {
		return s.registryFor(userID).Stream(ctx, ai.StreamRequest{
			ProviderID:  provider,
			Model:       model,
			System:      system,
			Messages:    []ai.Message{{Role: ai.RoleUser, Content: todo.Task}},
			Tools:       tools,
			ToolOrigins: setup.origins,
			MaxSteps:    agent.MaxSteps(agent.ModeCrew),
		}, ai.Handlers{
			OnText:      func(string) error { return nil },
			OnReasoning: func(string) error { return nil },
		})
	}

	reply, genErr := run(providerID, modelID)
	s.recordUsageFor(ctx, userID, providerID, modelID, reply.Usage)

	// A provider failure before any tool has run can safely be retried on the
	// worker's configured fallback: no side effect exists to duplicate and the
	// task state has not moved. A failure after a tool ran is not retried on
	// another model, because the fallback would see half a workspace.
	if genErr != nil && fallbackAvailable && ai.Retryable(genErr) && !traceHasToolCalls(reply.Trace) {
		if ctx.Err() == nil {
			fallbackReply, fallbackErr := run(altProviderID, altModelID)
			s.recordUsageFor(ctx, userID, altProviderID, altModelID, fallbackReply.Usage)
			if fallbackErr == nil || len(fallbackReply.Trace.Steps) > 0 {
				s.log.Warn("worker moved to its fallback model",
					"worker", worker.ID, "provider", altProviderID, "model", altModelID, "error", genErr)
				reply, genErr = fallbackReply, fallbackErr
				providerID, modelID = altProviderID, altModelID
			}
		}
	}

	// A report that arrived is the answer, even if the model then kept talking
	// and hit the step limit: the interface was satisfied before the trouble
	// started, and losing it would turn a finished task into a failure.
	if reported != nil {
		return *reported
	}

	// The worker stopped without a report. That is a failed run, not a success
	// with an empty report. When tools did run, say which files they may have
	// touched and mark the report partial, so the orchestrator knows the
	// workspace is not necessarily unchanged.
	files, mutated := traceChangedFiles(reply.Trace)
	partial := mutated
	if len(files) > 0 {
		report.FilesChanged = files
	}

	var reason string
	switch {
	case genErr != nil:
		reason = "The worker run failed: " + genErr.Error()
		if len(files) > 0 {
			reason += " Files may have been changed before the failure: " + strings.Join(files, ", ")
		}
	case reply.StepsExhausted:
		reason = fmt.Sprintf("The worker used its whole step budget (%d steps) without reporting.", reply.Steps)
	default:
		reason = "The worker stopped without calling report_task."
	}

	if partial {
		report.Partial = true
		report.Summary = "The worker stopped before reporting; partial changes may be present."
		report.Unresolved = append(report.Unresolved,
			"The worker did not report. Verify the workspace state before continuing.")
		if len(files) > 0 {
			report.Unresolved = append(report.Unresolved,
				"Files the worker may have changed: "+strings.Join(files, ", "))
		}
	}

	said := strings.TrimSpace(reply.Text)
	if answer := strings.TrimSpace(reply.Trace.AnswerText()); answer != "" {
		said = answer
	}
	if said != "" {
		reason += " It said: " + truncate(said, 500)
	}
	return fail(reason)
}

// traceHasToolCalls reports whether a run got as far as executing a tool. It is
// the boundary for a safe model fallback: before the first tool, the fallback
// starts from the same workspace and the same task.
func traceHasToolCalls(trace ai.Trace) bool {
	for _, step := range trace.Steps {
		for _, part := range step.Parts {
			if part.Kind == ai.PartTool {
				return true
			}
		}
	}
	return false
}

// traceChangedFiles reads the paths out of the write tools a worker called and
// reports whether any call could have changed state at all. It is a
// conservative record for a partial report, not a substitute for the worker's
// own list: a failed call is included, because it may still have changed the
// file, and a command or connector tool counts as mutating because the harness
// cannot see its effects.
func traceChangedFiles(trace ai.Trace) ([]string, bool) {
	seen := make(map[string]bool)
	files := make([]string, 0, 8)
	mutated := false
	for _, step := range trace.Steps {
		for _, part := range step.Parts {
			if part.Kind != ai.PartTool {
				continue
			}
			switch part.Name {
			case "read_file", "list_dir", "glob", "grep", "read_skill":
				continue
			}
			mutated = true
			switch part.Name {
			case "write_file", "edit_file", "edit_lines", "create_dir":
			default:
				continue
			}
			var args struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(part.Input), &args); err != nil {
				continue
			}
			path := strings.TrimSpace(args.Path)
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			files = append(files, path)
			if len(files) >= 50 {
				return files, mutated
			}
		}
	}
	return files, mutated
}

// workerSystemPrompt composes what a worker reads before it starts: the persona
// the account saved, the harness rules for a long run, then the task from the
// Orchestrator, the skills it must read, and the rule about stopping after two
// failures. The task and the rules come last so they are the freshest thing the
// model read when it takes its first step.
func (s *Server) workerSystemPrompt(ctx context.Context, chatID string, worker store.AgentRecord, todo agent.DelegateTaskTodo, workspace *agent.Workspace, tools []goai.Tool) string {
	persona := strings.TrimSpace(worker.SystemPrompt)
	if persona == "" {
		persona = strings.TrimSpace(worker.Prompt)
	}
	// A persona that already names the tools gets the list filled in. One that
	// does not is still told what it has, because no worker can choose a tool it
	// was never shown.
	personaNamesTools := strings.Contains(persona, "{{tools_section}}")

	persona = agent.RenderTemplate(persona, agent.PromptVars{
		Workspace: todo.Workspace,
		Date:      time.Now().Format("Monday, 2 January 2006, 15:04"),
		Tools:     toolSummaries(tools),
	})

	projectMap := agent.ScanProjectMap(ctx, workspace.Root()).ProjectMapString()
	// A worker gets the harness without the tools it does not have: no
	// ask_questions and no schedule_task. Building it here rather than editing
	// the shared prompt afterwards keeps the two from drifting apart.
	harness := agent.WorkerPrompt(workspace, persona, projectMap)

	var b strings.Builder
	b.WriteString(harness)
	b.WriteString("\n\n---\n\n")

	if !personaNamesTools {
		b.WriteString(agent.RenderTemplate("{{tools_section}}", agent.PromptVars{Tools: toolSummaries(tools)}))
		b.WriteString("\n\n")
	}

	// Prior work in this conversation is evidence the worker should not have to
	// rediscover. Its own current task is left out: it is about to read it.
	if state := agent.CrewTaskStateFor(todo.Workspace, chatID, worker.ID, todo.Task); state != "" {
		b.WriteString(state)
		b.WriteString("\n")
	}

	b.WriteString("Your task, from the Orchestrator. It is also saved at ")
	b.WriteString(todo.TodoPath)
	b.WriteString(" in the workspace.\n\n")
	b.WriteString("Task:\n")
	b.WriteString(todo.Task)
	if todo.Context != "" {
		b.WriteString("\n\nContext:\n")
		b.WriteString(todo.Context)
	}
	if len(todo.Acceptance) > 0 {
		b.WriteString("\n\nAcceptance criteria (the task is not complete until these are true):\n")
		for _, item := range todo.Acceptance {
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n\n")
	b.WriteString(agent.SkillInstruction(worker.Skills))

	b.WriteString("\n\nErrors:\n")
	b.WriteString("- If an action fails, read the error and change the call. Do not repeat the same failing call.\n")
	b.WriteString("- If the same step fails twice, stop trying it. Report what failed, with the real error, and end the task.\n")
	b.WriteString("- Do not ask the user anything. You have no way to reach them; report a blocker instead.\n")

	b.WriteString("\nFinishing:\n")
	b.WriteString("- When the task is done, and before you stop, you MUST call report_task exactly once.\n")
	b.WriteString("- status is completed, failed, or blocked. summary is one line saying what was done.\n")
	b.WriteString("- filesChanged lists every file you created or changed, relative to the project root.\n")
	b.WriteString("- checks lists only what you actually verified, for example typecheck, lint, build, or a manual check. If a check could not be run, say that instead of implying it passed.\n")
	b.WriteString("- unresolved lists what remains unfinished, what is uncertain, or what the next worker needs to know. Leave it out only when nothing remains.\n")
	b.WriteString("- If you did not finish, set failed or blocked and fill in error with the real reason.\n")
	b.WriteString("- The backend writes your report to ")
	b.WriteString(todo.ReportPath)
	b.WriteString("; your text answer is not the report.\n")

	return b.String()
}

// withoutScheduleTask is kept for callers outside this file; the worker prompt
// itself is now built without the scheduler line rather than edited afterwards.
func withoutScheduleTask(prompt string) string {
	lines := strings.Split(prompt, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.Contains(line, "schedule_task") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// truncate cuts a string to at most limit runes, adding an ellipsis when it did.
func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

// writeFileAtomic writes a file beside its destination and renames it into
// place, so a reader never sees a half-written todo or report.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
