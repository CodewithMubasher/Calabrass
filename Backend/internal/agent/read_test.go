package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A single read must not be able to fill the context on its own, and when it is
// stopped it has to say how to carry on.

func readOn(t *testing.T, ws *Workspace, path string) string {
	t.Helper()

	raw, err := json.Marshal(readFileArgs{Path: path})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	said, err := readFileTool(ws).Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("read_file %s: %v", path, err)
	}
	return said
}

// tail is the end of a string, for showing what a capped read finished with.
func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// readWorkspace is a project holding one file of the text it is given.
func readWorkspace(t *testing.T, name string, body []byte) *Workspace {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, name), body, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}

	ws, err := NewWorkspace(root)
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}
	return ws
}

func TestReadFileReturnsASmallFileWhole(t *testing.T) {
	ws := readWorkspace(t, "small.txt", []byte("first\nsecond\nthird\n"))

	said := readOn(t, ws, "small.txt")

	if !strings.Contains(said, "3 lines total") {
		t.Errorf("got %q, want the file's size", said)
	}
	if !strings.Contains(said, "| first") || !strings.Contains(said, "| third") {
		t.Errorf("got %q, want every line", said)
	}
	if strings.Contains(said, "stopped at") {
		t.Errorf("a small file should not be cut short: %q", said)
	}
}

// TestReadFileStopsBeforeItFillsTheContext: long lines are the way one read turns
// into fifty thousand tokens. The cap is counted in bytes, so the line length
// does not matter.
func TestReadFileStopsBeforeItFillsTheContext(t *testing.T) {
	var body strings.Builder
	for i := 0; i < 2000; i++ {
		body.WriteString(strings.Repeat("x", 200))
		body.WriteString("\n")
	}
	ws := readWorkspace(t, "big.txt", []byte(body.String()))

	said := readOn(t, ws, "big.txt")

	if len(said) > readFileMaxBytes+2048 {
		t.Errorf("one read returned %d bytes, over the %d KB cap", len(said), readFileMaxBytes>>10)
	}
	if !strings.Contains(said, "continue with offset=") {
		t.Errorf("a read that stops early has to say how to continue: %q", tail(said, 160))
	}
}

// TestReadFileSaysWhatItIsShowing: the model has to be able to tell a window from
// a whole file, because the line numbers it edits with depend on it.
func TestReadFileSaysWhatItIsShowing(t *testing.T) {
	var body strings.Builder
	for i := 0; i < 900; i++ {
		body.WriteString("a line\n")
	}
	ws := readWorkspace(t, "long.txt", []byte(body.String()))

	said := readOn(t, ws, "long.txt")

	if !strings.Contains(said, "900 lines total") {
		t.Errorf("got %q, want the file's size", head(said, 80))
	}
	if !strings.Contains(said, "showing 1-400") {
		t.Errorf("got %q, want the window it returned", head(said, 80))
	}
}
