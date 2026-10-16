package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// The library is what keeps every workspace inside one folder: a stored folder
// name must not be able to point at the project, at a sibling workspace, or
// anywhere else on disk.
func TestLibraryConfinesWorkspaces(t *testing.T) {
	root := t.TempDir()
	library, err := NewLibrary(root)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"default", "my-notes", ".hidden", "notes"} {
		path, err := library.Path(name)
		if err != nil {
			t.Errorf("Path(%q) = %v, want a path inside", name, err)
			continue
		}
		if filepath.Dir(path) != library.Root() {
			t.Errorf("Path(%q) = %q, want a direct child of %q", name, path, library.Root())
		}
	}

	outside := []string{"", "  ", ".", "..", "../escape", "a/b", `a\b`, "notes/.."}
	if os.PathSeparator == '\\' {
		outside = append(outside, `C:\Windows`, `C:relative`)
	} else {
		outside = append(outside, "/etc")
	}
	for _, name := range outside {
		if path, err := library.Path(name); err == nil {
			t.Errorf("Path(%q) = %q, want a refusal", name, path)
		}
	}
}

// A workspace the library hands out is still a workspace: the tools inside it
// cannot reach the library, a sibling, or the project around it.
func TestLibraryWorkspacesStayInside(t *testing.T) {
	root := t.TempDir()
	library, err := NewLibrary(root)
	if err != nil {
		t.Fatal(err)
	}

	notes, err := library.Ensure("notes")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.Ensure("default"); err != nil {
		t.Fatal(err)
	}

	// Ensure makes the folder when it is missing.
	info, err := os.Stat(notes.Root())
	if err != nil || !info.IsDir() {
		t.Fatalf("workspace folder = %v, %v", info, err)
	}
	if !library.Exists("notes") {
		t.Error("Exists(notes) = false, want true")
	}
	if library.Exists("nope") {
		t.Error("Exists(nope) = true, want false")
	}

	if _, err := notes.writeFile("draft.md", "hello\n"); err != nil {
		t.Fatalf("writeFile = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "notes", "draft.md")); err != nil {
		t.Errorf("file is not in the workspace folder: %v", err)
	}

	// A sibling workspace, its files, and the project are all out of reach.
	for _, path := range []string{"..", "../default", "../default/secret.txt", "../../go.mod", "../notes/../default"} {
		if abs, err := notes.Resolve(path); err == nil {
			t.Errorf("Resolve(%q) = %q, want a refusal", path, abs)
		}
	}
}

func TestLibraryUniqueNaming(t *testing.T) {
	root := t.TempDir()
	library, err := NewLibrary(root)
	if err != nil {
		t.Fatal(err)
	}

	if got := library.Unique("Landing Page"); got != "landing-page" {
		t.Errorf("Unique = %q, want landing-page", got)
	}
	if _, err := library.Ensure("landing-page"); err != nil {
		t.Fatal(err)
	}
	if got := library.Unique("Landing Page"); got != "landing-page-2" {
		t.Errorf("Unique when the first name is taken = %q, want landing-page-2", got)
	}
	if _, err := library.Ensure("landing-page-2"); err != nil {
		t.Fatal(err)
	}
	if got := library.Unique("Landing Page"); got != "landing-page-3" {
		t.Errorf("Unique when two names are taken = %q, want landing-page-3", got)
	}
	// A name with nothing usable in it still gets a folder.
	if got := library.Unique("!!!"); got != "workspace" {
		t.Errorf("Unique(!!!) = %q, want workspace", got)
	}
}

func TestSlug(t *testing.T) {
	cases := []struct{ name, want string }{
		{"Notes", "notes"},
		{"Landing  Page", "landing-page"},
		{"my_project/v2", "my-project-v2"},
		{"  spaced  ", "spaced"},
		{"!!!", "workspace"},
		{"", "workspace"},
		{"مرحبا بالعالم", "مرحبا-بالعالم"},
	}
	for _, c := range cases {
		if got := Slug(c.name); got != c.want {
			t.Errorf("Slug(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// A folder name that reached the database some other way is still checked.
func TestWorkspaceDir(t *testing.T) {
	for _, name := range []string{"default", "notes-2", ".hidden", "مرحبا"} {
		if _, err := workspaceDir(name); err != nil {
			t.Errorf("workspaceDir(%q) = %v, want ok", name, err)
		}
	}
	for _, name := range []string{"", " ", ".", "..", "../x", "a/b", `a\b`, "C:temp", "/tmp"} {
		if got, err := workspaceDir(name); err == nil {
			t.Errorf("workspaceDir(%q) = %q, want a refusal", name, got)
		}
	}
}
