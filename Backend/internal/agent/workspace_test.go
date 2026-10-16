package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The workspace guard is the one security-relevant part of the harness: a model
// must not be able to read or write outside the project the user opened.
func TestWorkspaceResolveConfines(t *testing.T) {
	root := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "sub", "deep"), 0o755))
	must(os.WriteFile(filepath.Join(root, "notes.txt"), []byte("a\nb\nc\n"), 0o644))
	must(os.WriteFile(filepath.Join(root, "sub", "deep", "x.txt"), []byte("deep\n"), 0o644))

	outside := filepath.Join(filepath.Dir(root), "outside-verify.txt")
	must(os.WriteFile(outside, []byte("secret\n"), 0o644))
	defer func() { _ = os.Remove(outside) }()

	ws, err := NewWorkspace(root)
	must(err)

	// Inside: relative, dotted, and already-absolute paths all land in the
	// workspace. The root itself is "" and ".".
	inside := []string{"", ".", "notes.txt", "sub/deep/x.txt", filepath.Join(root, "notes.txt")}
	for _, path := range inside {
		abs, err := ws.Resolve(path)
		if err != nil {
			t.Errorf("Resolve(%q) = %v, want inside", path, err)
			continue
		}
		if !strings.HasPrefix(abs, ws.Root()) {
			t.Errorf("Resolve(%q) = %q, want under %q", path, abs, ws.Root())
		}
	}

	// Outside: climbing out, by any spelling, is refused.
	outsidePaths := []string{
		"../outside-verify.txt",
		"sub/../../outside-verify.txt",
		"..",
		filepath.Join(root, "..", "outside-verify.txt"),
		outside,
	}
	if os.PathSeparator == '\\' {
		outsidePaths = append(outsidePaths, `C:\Windows\System32\drivers\etc\hosts`)
	} else {
		outsidePaths = append(outsidePaths, "/etc/passwd")
	}
	for _, path := range outsidePaths {
		if abs, err := ws.Resolve(path); err == nil {
			t.Errorf("Resolve(%q) = %q, want refusal", path, abs)
		}
	}
}

func TestReadFileWindowAndGuards(t *testing.T) {
	root := t.TempDir()
	ws, err := NewWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}

	// 500 lines: a default read returns 400 of them and says how many there are.
	var big strings.Builder
	for i := 1; i <= 500; i++ {
		big.WriteString("line\n")
	}
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(big.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := ws.readFile(readFileArgs{Path: "big.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "500 lines total; showing 1-400") {
		t.Errorf("header = %q", head(out, 80))
	}
	if got := strings.Count(out, "| "); got != 400 {
		t.Errorf("returned %d lines, want 400", got)
	}

	// Paging reads the tail.
	tail, err := ws.readFile(readFileArgs{Path: "big.txt", Offset: 495, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tail, "showing 495-500") || strings.Count(tail, "| ") != 6 {
		t.Errorf("page = %q", head(tail, 120))
	}

	// A directory, a missing file, a binary file and an escape are all refused.
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.readFile(readFileArgs{Path: "dir"}); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Errorf("directory err = %v", err)
	}
	if _, err := ws.readFile(readFileArgs{Path: "nope.txt"}); err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Errorf("missing err = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), []byte{1, 0, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.readFile(readFileArgs{Path: "bin.dat"}); err == nil || !strings.Contains(err.Error(), "binary") {
		t.Errorf("binary err = %v", err)
	}
	if _, err := ws.readFile(readFileArgs{Path: "../escape.txt"}); err == nil {
		t.Error("escape should fail")
	}
}

func TestDiscoverRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := DiscoverRoot(deep); got != root {
		t.Errorf("DiscoverRoot = %q, want %q", got, root)
	}
}

// The write tools touch the user's files, so the rules that keep them precise
// matter as much as the writes themselves.
func TestWriteTools(t *testing.T) {
	root := t.TempDir()
	ws, err := NewWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}

	// create_dir makes parents and is happy to be asked twice.
	if _, err := ws.createDir("a/b"); err != nil {
		t.Fatalf("createDir = %v", err)
	}
	if _, err := ws.createDir("a/b"); err != nil {
		t.Fatalf("createDir again = %v", err)
	}
	if _, err := ws.createDir("../escape"); err == nil {
		t.Error("createDir outside the workspace should fail")
	}

	// write_file creates and then replaces.
	if out, err := ws.writeFile("a/b/x.txt", "one\ntwo\nthree\n"); err != nil || !strings.HasPrefix(out, "created") {
		t.Fatalf("writeFile = %q, %v", out, err)
	}
	if out, err := ws.writeFile("a/b/x.txt", "one\ntwo\nthree\n"); err != nil || !strings.HasPrefix(out, "overwrote") {
		t.Fatalf("writeFile again = %q, %v", out, err)
	}
	if _, err := ws.writeFile("../escape.txt", "nope"); err == nil {
		t.Error("writeFile outside the workspace should fail")
	}
	if _, err := ws.writeFile("big.txt", strings.Repeat("x", writeFileMaxBytes+1)); err == nil {
		t.Error("writeFile over the size cap should fail")
	}

	// edit_file replaces exactly one match.
	out, err := ws.editFile(editFileArgs{Path: "a/b/x.txt", OldString: "two", NewString: "TWO"})
	if err != nil {
		t.Fatalf("editFile = %v", err)
	}
	if !strings.Contains(out, "line 2") {
		t.Errorf("editFile = %q, want the line number", out)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "a", "b", "x.txt")); string(data) != "one\nTWO\nthree\n" {
		t.Errorf("content = %q", string(data))
	}

	// A repeated match is refused unless the caller means it.
	if _, err := ws.writeFile("dup.txt", "x\nx\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.editFile(editFileArgs{Path: "dup.txt", OldString: "x", NewString: "y"}); err == nil || !strings.Contains(err.Error(), "2 times") {
		t.Errorf("ambiguous edit = %v, want a refusal mentioning 2 times", err)
	}
	if out, err := ws.editFile(editFileArgs{Path: "dup.txt", OldString: "x", NewString: "y", ReplaceAll: true}); err != nil {
		t.Errorf("replaceAll = %q, %v", out, err)
	}

	// Empty, identical and missing text are all refused.
	if _, err := ws.editFile(editFileArgs{Path: "dup.txt", OldString: "  ", NewString: "y"}); err == nil {
		t.Error("blank oldString should fail")
	}
	if _, err := ws.editFile(editFileArgs{Path: "dup.txt", OldString: "y", NewString: "y"}); err == nil {
		t.Error("identical strings should fail")
	}
	if _, err := ws.editFile(editFileArgs{Path: "dup.txt", OldString: "absent", NewString: "y"}); err == nil || !strings.Contains(err.Error(), "not in") {
		t.Errorf("absent oldString = %v", err)
	}
	if _, err := ws.editFile(editFileArgs{Path: "missing.txt", OldString: "a", NewString: "b"}); err == nil || !strings.Contains(err.Error(), "write_file") {
		t.Errorf("missing file = %v, want a hint to use write_file", err)
	}
}

func TestMatchName(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"*.go", "main.go", true},
		{"*.go", "internal/ai/ai.go", true},
		{"*.go", "internal/ai/ai.ts", false},
		{"src/**/*.tsx", "src/routes/chat.tsx", true},
		{"src/**/*.tsx", "src/chat.tsx", true},
		{"src/**/*.tsx", "src/routes/chat.ts", false},
		{"src/*.ts", "src/routes/chat.ts", false},
		{"**/*_test.go", "internal/agent/tools_test.go", true},
		{"Backend/internal/**", "Backend/internal/ai/ai.go", true},
	}
	for _, c := range cases {
		if got := matchName(c.pattern, c.path); got != c.want {
			t.Errorf("matchName(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func head(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
