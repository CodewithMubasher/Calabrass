package httpapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
)

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
