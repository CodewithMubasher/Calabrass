package connectors

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSplitCommandKeepsQuotedRunsTogether(t *testing.T) {
	fields, err := splitCommand(`"C:\Program Files\a thing\x.exe" --flag "two words" plain`)
	if err != nil {
		t.Fatalf("splitCommand: %v", err)
	}

	want := []string{`C:\Program Files\a thing\x.exe`, "--flag", "two words", "plain"}
	if len(fields) != len(want) {
		t.Fatalf("got %d fields (%q), want %d", len(fields), fields, len(want))
	}
	for i := range want {
		if fields[i] != want[i] {
			t.Errorf("field %d = %q, want %q", i, fields[i], want[i])
		}
	}
}

func TestSplitCommandRefusesAnUnclosedQuote(t *testing.T) {
	if _, err := splitCommand(`"C:\Program Files\x.exe`); err == nil {
		t.Error("a command that ends inside a quote should be refused, not guessed at")
	}
}

// TestResolveTakesAWholePathWithSpaces: this project lives under a folder with a
// space in its name, so a connector pointing at a binary here must survive
// being written the obvious way.
func TestResolveTakesAWholePathWithSpaces(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "a folder with spaces", "win-control-mcp.exe")
	writeFile(t, binary)

	program, args, err := New(nil, root, nil).resolve(binary)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if program != binary {
		t.Errorf("program = %q, want the path as written", program)
	}
	if len(args) != 0 {
		t.Errorf("args = %q, want none", args)
	}
}

func TestResolveFindsACommandInsideTheCheckout(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the checkout layout and the backslash path it resolves are Windows-shaped")
	}
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "MCP", "win-control", "win-control-mcp.exe"))

	program, _, err := New(nil, root, nil).resolve(`MCP\win-control\win-control-mcp.exe`)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !filepath.IsAbs(program) || !strings.HasSuffix(program, "win-control-mcp.exe") {
		t.Errorf("program = %q, want the absolute path to the binary", program)
	}
}

func TestResolveSplitsArguments(t *testing.T) {
	root := t.TempDir()

	// A command that is not a file here is split, and the program is left for the
	// operating system to find — that is how "npx" and "go" keep working.
	program, args, err := New(nil, root, nil).resolve(`node "C:\tools\server.js" --port 8080`)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if program != "node" {
		t.Errorf("program = %q, want node", program)
	}
	if len(args) != 3 || args[0] != `C:\tools\server.js` || args[1] != "--port" || args[2] != "8080" {
		t.Errorf("args = %q, want the script and its flags", args)
	}
}

func TestResolveRefusesAnEmptyCommand(t *testing.T) {
	for _, command := range []string{"", "   "} {
		if _, _, err := New(nil, "", nil).resolve(command); err == nil {
			t.Errorf("%q should have been refused", command)
		}
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("not really a program"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
