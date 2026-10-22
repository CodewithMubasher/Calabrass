package httpapi

import (
	"context"
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

	report, err := s.delegateTask(r.Context(), user.ID, chatID, workspace.Root(), agent.DelegateTaskArgs{
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
		Task:       strings.TrimSpace(args.Task),
		Context:    strings.TrimSpace(args.Context),
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

	report := s.runWorker(ctx, userID, chatID, workspaceRoot, worker, todo)
	report.FinishedAt = agent.NowRFC3339()

	reportBytes, err := agent.MarshalIndentJSON(report)
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(reportPath, reportBytes); err != nil {
		return "", fmt.Errorf("write the report: %w", err)
	}

	return string(reportBytes), nil
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
	if providerID == "" || modelID == "" {
		providerID, modelID = s.resolveModelFor(ctx, userID, worker.AltProvider, worker.AltModel)
	}
	if providerID == "" || modelID == "" {
		return fail("No model is available for this worker. Pick one in the agent's settings.")
	}

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
			Error:        args.Error,
			StartedAt:    report.StartedAt,
		}
		if err := agent.ValidateReport(&rep); err != nil {
			return "", err
		}
		reported = &rep
		return "Report received. Stop now.", nil
	}))

	system := s.workerSystemPrompt(ctx, worker, todo, workspace, tools)

	reply, genErr := s.registryFor(userID).Stream(ctx, ai.StreamRequest{
		ProviderID:  providerID,
		Model:       modelID,
		System:      system,
		Messages:    []ai.Message{{Role: ai.RoleUser, Content: todo.Task}},
		Tools:       tools,
		ToolOrigins: setup.origins,
		MaxSteps:    agent.MaxSteps(agent.ModeCrew),
	}, ai.Handlers{
		OnText:      func(string) error { return nil },
		OnReasoning: func(string) error { return nil },
	})
	s.recordUsageFor(ctx, userID, providerID, modelID, reply.Usage)

	// A report that arrived is the answer, even if the model then kept talking
	// and hit the step limit: the interface was satisfied before the trouble
	// started, and losing it would turn a finished task into a failure.
	if reported != nil {
		return *reported
	}

	if genErr != nil {
		reason := "The worker run failed: " + genErr.Error()
		return fail(reason)
	}

	// The worker stopped without calling report_task. That is a failed run, not
	// a success with an empty report: the interface is how the Orchestrator
	// knows what happened, and there is no interface without it.
	said := strings.TrimSpace(reply.Text)
	if answer := strings.TrimSpace(reply.Trace.AnswerText()); answer != "" {
		said = answer
	}
	reason := "The worker stopped without calling report_task."
	if said != "" {
		reason += " It said: " + truncate(said, 500)
	}
	return fail(reason)
}

// workerSystemPrompt composes what a worker reads before it starts: the persona
// the account saved, the harness rules for a long run, then the task from the
// Orchestrator, the skills it must read, and the rule about stopping after two
// failures. The task and the rules come last so they are the freshest thing the
// model read when it takes its first step.
func (s *Server) workerSystemPrompt(ctx context.Context, worker store.AgentRecord, todo agent.DelegateTaskTodo, workspace *agent.Workspace, tools []goai.Tool) string {
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
	harness := withoutScheduleTask(agent.SystemPrompt(workspace, persona, nil, agent.ModeCrew, projectMap))

	var b strings.Builder
	b.WriteString(harness)
	b.WriteString("\n\n---\n\n")

	if !personaNamesTools {
		b.WriteString(agent.RenderTemplate("{{tools_section}}", agent.PromptVars{Tools: toolSummaries(tools)}))
		b.WriteString("\n\n")
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
	b.WriteString("- checks lists what you verified, for example typecheck, lint, build, or a manual check.\n")
	b.WriteString("- If you did not finish, set failed or blocked and fill in error with the real reason.\n")
	b.WriteString("- The backend writes your report to ")
	b.WriteString(todo.ReportPath)
	b.WriteString("; your text answer is not the report.\n")

	return b.String()
}

// withoutScheduleTask drops the harness's schedule_task advice from a worker's
// prompt. A worker has no scheduler — it finishes one task and reports — and a
// prompt that describes a tool the model does not have invites a call that can
// only fail.
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
