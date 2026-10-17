package httpapi

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The rail shows the workspace as it is on disk, so the listing has to agree
// with the folder: relative slash-separated paths, extensions the viewer can
// read, and dependencies and build output left out.
func TestScanArtifactsListsTheWorkspace(t *testing.T) {
	root := t.TempDir()
	write := func(rel string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("plan.md")
	write("src/main.go")
	write("notes/todo.txt")
	write(".env")
	write("LICENCE") // no extension at all
	write("node_modules/left-pad/index.js")
	write("dist/bundle.js")
	write(".git/HEAD")

	files, signature := scanArtifacts(root)
	if signature == "" {
		t.Fatal("signature is empty, so a watcher could never see the folder change")
	}

	seen := map[string]string{} // path -> extension
	for _, file := range files {
		seen[file.Path] = file.Extension
	}

	for _, want := range []string{"plan.md", "src/main.go", "notes/todo.txt", ".env", "LICENCE"} {
		if _, ok := seen[want]; !ok {
			t.Errorf("%s is missing from the listing: %v", want, seen)
		}
	}
	for _, unwanted := range []string{
		"node_modules/left-pad/index.js", "dist/bundle.js", ".git/HEAD",
	} {
		if _, ok := seen[unwanted]; ok {
			t.Errorf("%s should not be listed as an artifact", unwanted)
		}
	}

	if got := seen["plan.md"]; got != "MD" {
		t.Errorf("plan.md extension = %q, want %q", got, "MD")
	}
	if got := seen["LICENCE"]; got != "" {
		t.Errorf("an extensionless file reported extension %q, want none", got)
	}
}

// The listing leads with the newest file, so whatever the agent has just
// written is the first thing on screen.
func TestScanArtifactsLeadsWithTheNewestFile(t *testing.T) {
	root := t.TempDir()
	older := filepath.Join(root, "older.md")
	newer := filepath.Join(root, "newer.md")

	for _, path := range []string{older, newer} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(older, past, past); err != nil {
		t.Fatal(err)
	}

	files, _ := scanArtifacts(root)
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	if files[0].Path != "newer.md" {
		t.Errorf("first file = %q, want the newest (%q)", files[0].Path, "newer.md")
	}
}

// A watcher compares signatures instead of listings, so the signature has to
// change when a file does and stay put when nothing has.
func TestScanArtifactsSignatureTracksTheFolder(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "plan.md")
	if err := os.WriteFile(path, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, before := scanArtifacts(root)
	if _, again := scanArtifacts(root); again != before {
		t.Error("signature changed with nothing to show for it")
	}

	if err := os.WriteFile(path, []byte("second, longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, after := scanArtifacts(root); after == before {
		t.Error("signature did not change after the file was written again")
	}
}

// Download-all names the archive after the workspace it holds.
func TestArtifactAttachmentNamesTheFile(t *testing.T) {
	got := artifactAttachment("plan.md")
	if got != `attachment; filename=plan.md` {
		t.Errorf("attachment = %q, want a plain filename", got)
	}
}
