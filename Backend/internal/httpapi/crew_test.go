package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/crew"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// A chat in crew mode that names no agent belongs to the Orchestrator. It used
// to be created with no agent at all, so the ordinary coding harness answered
// and did the work itself instead of writing a todo and delegating.
func TestACrewChatWithoutAnAgentIsBoundToTheOrchestrator(t *testing.T) {
	server, token := modeTestServer(t)
	addProvider(t, server, token)

	chat, status := createChat(t, server, token, `{"mode":"crew"}`)
	if status != http.StatusCreated {
		t.Fatalf("creating a crew chat answered %d", status)
	}
	if chat.AgentID != crew.BuiltinOrchestratorID {
		t.Fatalf("the crew chat came back with agent %q, want the Orchestrator", chat.AgentID)
	}
	if chat.Mode != agent.ModeCrew {
		t.Fatalf("the chat came back in mode %q, want crew", chat.Mode)
	}

	// An ordinary chat with no agent stays unbound: the harness is what the
	// person asked for there.
	chat, status = createChat(t, server, token, `{"mode":"chat"}`)
	if status != http.StatusCreated {
		t.Fatalf("creating a chat answered %d", status)
	}
	if chat.AgentID != "" {
		t.Fatalf("the ordinary chat came back with agent %q, want none", chat.AgentID)
	}
}

// A crew chat that was made before the binding still gets the Orchestrator's
// turn: two tools and a plan-and-delegate prompt, never a text editor. This is
// what keeps an older chat from coding the task itself.
func TestACrewTurnWithoutAnAgentIsTheOrchestratorTurn(t *testing.T) {
	ctx := context.Background()

	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ws, err := agent.NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}

	chat, err := st.CreateChat(ctx, "", "", ws.Root(), "", agent.ModeCrew)
	if err != nil {
		t.Fatalf("create chat: %v", err)
	}

	s := &Server{store: st, log: slog.Default()}
	setup := s.setupTurnFor(ctx, "", ws, chat.ID, "")

	names := make([]string, 0, len(setup.tools))
	for _, tool := range setup.tools {
		names = append(names, tool.Name)
	}
	if len(names) != 2 || names[0] != "ask_questions" || names[1] != "delegate_task" {
		t.Fatalf("the crew turn got %v, want only ask_questions and delegate_task", names)
	}
	if strings.Contains(setup.system, "I'm Calabrass") {
		t.Fatalf("the crew turn got the generic assistant prompt:\n%s", setup.system)
	}
	if !strings.Contains(setup.system, "Orchestrator") {
		t.Fatalf("the crew turn does not describe the Orchestrator:\n%s", setup.system)
	}
	// The ids are the one fact the model cannot infer. They must be in the
	// prompt even if a saved persona lost the placeholder.
	if !strings.Contains(setup.system, "id: "+crew.BuiltinFrontendDeveloperID) {
		t.Fatalf("the crew turn does not name the worker id:\n%s", setup.system)
	}
	for _, tool := range setup.tools {
		if tool.Name == "delegate_task" && !strings.Contains(tool.Description, crew.BuiltinFrontendDeveloperID) {
			t.Fatalf("delegate_task does not name the worker id:\n%s", tool.Description)
		}
	}
}

// newCrewTestServer builds the pieces delegateTask needs: a store for the
// agents and a workspace for the .crew files. No provider is registered, so a
// worker run ends with the "no model" report; the todo is still written, which
// is what these tests read.
func newCrewTestServer(t *testing.T) (*Server, *agent.Workspace) {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ws, err := agent.NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}

	return &Server{store: st, log: slog.Default()}, ws
}

// readTodo reads the todo delegateTask wrote for one worker.
func readTodo(t *testing.T, workspaceRoot, workerID string) agent.DelegateTaskTodo {
	t.Helper()

	orchestratorDir, _ := agent.CrewDirs(workspaceRoot)
	data, err := os.ReadFile(agent.DelegateTaskTodoPath(orchestratorDir, workerID))
	if err != nil {
		t.Fatalf("read the todo for %s: %v", workerID, err)
	}

	var todo agent.DelegateTaskTodo
	if err := json.Unmarshal(data, &todo); err != nil {
		t.Fatalf("read the todo JSON: %v", err)
	}
	return todo
}

// A model that drops workerId still delegates when there is one worker: there
// is nothing to choose, so the only worker is used.
func TestDelegateTaskUsesTheOnlyWorkerWhenTheIDIsMissing(t *testing.T) {
	s, ws := newCrewTestServer(t)

	report, err := s.delegateTask(context.Background(), "", "chat-1", ws.Root(), agent.DelegateTaskArgs{
		Task: "Build the Foodie page",
	})
	if err != nil {
		t.Fatalf("delegating without an id: %v", err)
	}
	if !strings.Contains(report, `"workerId": "builtin-frontend-developer"`) {
		t.Fatalf("the report is not for the Frontend Developer:\n%s", report)
	}

	todo := readTodo(t, ws.Root(), crew.BuiltinFrontendDeveloperID)
	if todo.WorkerID != crew.BuiltinFrontendDeveloperID {
		t.Fatalf("the todo went to %q, want the Frontend Developer", todo.WorkerID)
	}
	if todo.Task != "Build the Foodie page" {
		t.Fatalf("the todo task is %q", todo.Task)
	}
}

// The worker's name, or its id without the built-in prefix, resolves to the
// same worker rather than failing on a naming difference.
func TestDelegateTaskResolvesTheWorkerByName(t *testing.T) {
	for _, wanted := range []string{"Frontend Developer", "frontend-developer"} {
		t.Run(wanted, func(t *testing.T) {
			s, ws := newCrewTestServer(t)

			if _, err := s.delegateTask(context.Background(), "", "chat-1", ws.Root(), agent.DelegateTaskArgs{
				WorkerID: wanted,
				Task:     "Build the Foodie page",
			}); err != nil {
				t.Fatalf("delegating to %q: %v", wanted, err)
			}

			todo := readTodo(t, ws.Root(), crew.BuiltinFrontendDeveloperID)
			if todo.WorkerID != crew.BuiltinFrontendDeveloperID {
				t.Fatalf("the todo went to %q", todo.WorkerID)
			}
		})
	}
}

// With more than one worker the id matters, so a call without one is refused
// with the ids to choose from instead of a bare "a workerId is required".
func TestDelegateTaskWithoutAnIDListsTheWorkers(t *testing.T) {
	s, ws := newCrewTestServer(t)

	if _, err := s.store.CreateAgent(context.Background(), store.AgentRecord{
		Name:        "Backend Developer",
		Prompt:      "Build the backend.",
		Description: "Builds APIs.",
		Mode:        agent.ModeCrew,
	}); err != nil {
		t.Fatalf("create the second worker: %v", err)
	}

	_, err := s.delegateTask(context.Background(), "", "chat-1", ws.Root(), agent.DelegateTaskArgs{
		Task: "Build the Foodie page",
	})
	if err == nil {
		t.Fatal("delegating with two workers and no id was accepted")
	}
	for _, want := range []string{"workerId is required", "builtin-frontend-developer", "Frontend Developer"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %q: %v", want, err)
		}
	}
}

// An id that names nobody gets the choices back, so the model can retry in the
// same turn.
func TestDelegateTaskUnknownIDListsTheWorkers(t *testing.T) {
	s, ws := newCrewTestServer(t)

	_, err := s.delegateTask(context.Background(), "", "chat-1", ws.Root(), agent.DelegateTaskArgs{
		WorkerID: "somebody",
		Task:     "Build the Foodie page",
	})
	if err == nil {
		t.Fatal("an unknown worker id was accepted")
	}
	if !strings.Contains(err.Error(), "builtin-frontend-developer") {
		t.Errorf("the error does not list the real worker: %v", err)
	}
}

// Only the crew's JSON work belongs in the rail. The atomic writer leaves a
// .tmp beside each file for an instant, and the folder may hold notes or diffs
// a person put there; neither is a todo or a report.
func TestScanCrewListsOnlyJSONWork(t *testing.T) {
	root := t.TempDir()
	orchestratorDir, workersDir := agent.CrewDirs(root)

	if err := os.MkdirAll(orchestratorDir, 0o755); err != nil {
		t.Fatalf("create orchestrator dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workersDir, "builtin-frontend-developer"), 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}

	files := map[string]string{
		filepath.Join(orchestratorDir, "builtin-frontend-developer.todo.json"):     `{"task":"Build it"}`,
		filepath.Join(workersDir, "builtin-frontend-developer", "report.json"):     `{"status":"completed"}`,
		filepath.Join(orchestratorDir, "builtin-frontend-developer.todo.json.tmp"): "half a write",
		filepath.Join(orchestratorDir, "notes.txt"):                                "not crew work",
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	listed, signature := scanCrew(root)
	if len(listed) != 2 {
		t.Fatalf("scanCrew listed %d files, want the todo and the report: %+v", len(listed), listed)
	}
	if signature == "" {
		t.Fatal("scanCrew returned an empty signature")
	}

	paths := map[string]bool{}
	for _, file := range listed {
		paths[file.Path] = true
		if !strings.HasPrefix(file.Path, agent.CrewDirName+"/") {
			t.Errorf("path %q is not relative to the workspace", file.Path)
		}
	}
	if !paths[".crew/orchestrator/builtin-frontend-developer.todo.json"] {
		t.Errorf("the todo was not listed: %v", paths)
	}
	if !paths[".crew/workers/builtin-frontend-developer/report.json"] {
		t.Errorf("the report was not listed: %v", paths)
	}
}

// A worker has no scheduler, so the harness line that describes one must not
// reach its prompt: a model told about a tool it does not have will call it and
// fail.
func TestWithoutScheduleTaskDropsTheWholeLine(t *testing.T) {
	prompt := strings.Join([]string{
		"Tool use:",
		"- Use schedule_task when the user wants something to happen later.",
		"- Read a file before you change it.",
	}, "\n")

	got := withoutScheduleTask(prompt)
	if strings.Contains(got, "schedule_task") {
		t.Fatalf("the scheduler line survived:\n%s", got)
	}
	if !strings.Contains(got, "Read a file before you change it.") {
		t.Fatalf("the other advice was dropped:\n%s", got)
	}
}
