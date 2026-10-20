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

// runCommandOn runs one command through the tool and returns the parsed result.
func runCommandOn(t *testing.T, ws *Workspace, args map[string]any) shellToolResult {
	t.Helper()

	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	said, err := shellTool(ws).Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("run_command %s: %v", raw, err)
	}

	var result shellToolResult
	if err := json.Unmarshal([]byte(said), &result); err != nil {
		t.Fatalf("unmarshal result %q: %v", said, err)
	}
	return result
}

func TestRunCommandStartsInTheProjectDirectory(t *testing.T) {
	ws := shellWorkspace(t)

	result := runCommandOn(t, ws, map[string]any{
		"command": shellCommand("Get-Content marker.txt", "cat marker.txt"),
	})

	if !strings.Contains(result.Output, "the-project-marker") {
		t.Errorf("got %q, want the file the command read by a relative path", result.Output)
	}
	if result.ExitCode != 0 {
		t.Errorf("got exit code %d, want 0", result.ExitCode)
	}
	if result.Cwd != ws.Root() {
		t.Errorf("got cwd %q, want workspace root %q", result.Cwd, ws.Root())
	}
}

func TestRunCommandReportsAFailureWithoutFailingTheCall(t *testing.T) {
	ws := shellWorkspace(t)

	result := runCommandOn(t, ws, map[string]any{
		"command": shellCommand(`Write-Output "before"; exit 3`, "echo before; exit 3"),
	})

	if !strings.Contains(result.Output, "before") {
		t.Errorf("got %q, want what the command printed before it failed", result.Output)
	}
	if result.ExitCode != 3 {
		t.Errorf("got exit code %d, want 3", result.ExitCode)
	}
}

func TestRunCommandStopsACommandThatWillNotEnd(t *testing.T) {
	ws := shellWorkspace(t)

	started := time.Now()
	result := runCommandOn(t, ws, map[string]any{
		"command":         shellCommand("Start-Sleep -Seconds 30", "sleep 30"),
		"timeout_seconds": 1,
	})

	if result.ExitCode != -1 {
		t.Errorf("got exit code %d, want -1 (timeout)", result.ExitCode)
	}
	if elapsed := time.Since(started); elapsed > 15*time.Second {
		t.Errorf("took %s, want the timeout to end the command", elapsed)
	}
}

func TestRunCommandNeedsACommand(t *testing.T) {
	ws := shellWorkspace(t)

	_, err := shellTool(ws).Execute(context.Background(), json.RawMessage(`{"command":"   "}`))
	if err == nil {
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
