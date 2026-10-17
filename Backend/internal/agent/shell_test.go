package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// The shell tool is the one tool that runs on the user's own machine, so what is
// checked here is what it promises: the command starts in the project directory,
// its output and its exit code both come back, and it cannot run for ever.

// shellCommand is the same request in the two shells this program speaks, so the
// tests say what they mean wherever they run.
func shellCommand(windows, other string) string {
	if runtime.GOOS == "windows" {
		return windows
	}
	return other
}

// shellWorkspace is a project with one file in it, to be read back through a
// relative path.
func shellWorkspace(t *testing.T) *Workspace {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "marker.txt"), []byte("the-project-marker\n"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	ws, err := NewWorkspace(root)
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}
	return ws
}

// runCommandOn runs one command through the tool and returns what it said. A
// command that fails is not a failed call, so only a broken tool fails the test.
func runCommandOn(t *testing.T, ws *Workspace, args map[string]any) string {
	t.Helper()

	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	said, err := shellTool(ws).Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("run_command %s: %v", raw, err)
	}
	return said
}

func TestRunCommandStartsInTheProjectDirectory(t *testing.T) {
	ws := shellWorkspace(t)

	said := runCommandOn(t, ws, map[string]any{
		"command": shellCommand("Get-Content marker.txt", "cat marker.txt"),
	})

	if !strings.Contains(said, "the-project-marker") {
		t.Errorf("got %q, want the file the command read by a relative path", said)
	}
	if !strings.Contains(said, "exit code 0") {
		t.Errorf("got %q, want the exit code to be reported", said)
	}
}

func TestRunCommandReportsAFailureWithoutFailingTheCall(t *testing.T) {
	ws := shellWorkspace(t)

	said := runCommandOn(t, ws, map[string]any{
		"command": shellCommand(`Write-Output "before"; exit 3`, "echo before; exit 3"),
	})

	if !strings.Contains(said, "before") {
		t.Errorf("got %q, want what the command printed before it failed", said)
	}
	if !strings.Contains(said, "exit code 3") {
		t.Errorf("got %q, want the exit code the command ended with", said)
	}
}

func TestRunCommandStopsACommandThatWillNotEnd(t *testing.T) {
	ws := shellWorkspace(t)

	started := time.Now()
	said := runCommandOn(t, ws, map[string]any{
		"command":         shellCommand("Start-Sleep -Seconds 30", "sleep 30"),
		"timeout_seconds": 1,
	})

	if !strings.Contains(said, "killed after 1s") {
		t.Errorf("got %q, want it to say the command was killed", said)
	}
	if elapsed := time.Since(started); elapsed > 15*time.Second {
		t.Errorf("took %s, want the timeout to end the command", elapsed)
	}
}

func TestRunCommandNeedsACommand(t *testing.T) {
	ws := shellWorkspace(t)

	if _, err := shellTool(ws).Execute(context.Background(), json.RawMessage(`{"command":"   "}`)); err == nil {
		t.Error("an empty command should be refused rather than run")
	}
}

// TestLongOutputKeepsItsBeginningAndItsEnd: a build says what it is doing at the
// start and how it went at the end, and both ends are what the model needs.
func TestLongOutputKeepsItsBeginningAndItsEnd(t *testing.T) {
	text := strings.Repeat("a", 5000) + "THE-END"

	got := elideMiddle(text, 300)

	if !strings.HasPrefix(got, "aaa") {
		t.Errorf("the beginning was dropped: %q", got[:40])
	}
	if !strings.HasSuffix(got, "THE-END") {
		t.Errorf("the end was dropped: %q", got[len(got)-40:])
	}
	if !strings.Contains(got, "left out") {
		t.Error("output was dropped without saying so")
	}
	if len(got) > 300+80 {
		t.Errorf("elided output is %d bytes, want it near the cap", len(got))
	}
}

func TestElidedOutputStaysValidUTF8(t *testing.T) {
	text := strings.Repeat("é", 500)

	got := elideMiddle(text, 101)

	if !utf8.ValidString(got) {
		t.Errorf("a cut through a rune produced invalid UTF-8: %q", got)
	}
}
