package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// symlinkOrSkip makes a link, or skips the test where the system will not
// allow one (Windows without developer mode).
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symbolic links here: %v", err)
	}
}

// A link inside the project that points outside must not be a way out, even for
// a file that does not exist yet. Before, the check fell back to the spelling
// of the path whenever the file was missing, so "link/new.txt" got through.
func TestSymlinkCannotLeadOutOfTheWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	ws, err := NewWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, outside, filepath.Join(root, "link"))

	for _, path := range []string{"link", "link/new.txt", "link/sub/deeper/new.txt"} {
		if abs, err := ws.Resolve(path); err == nil {
			t.Errorf("Resolve(%q) = %q, want refusal", path, abs)
		}
	}
	if _, err := ws.writeFile("link/new.txt", "pwned"); err == nil {
		t.Error("writeFile through an outbound link should fail")
	}
	if _, err := os.Stat(filepath.Join(outside, "new.txt")); err == nil {
		t.Fatal("a file was created outside the workspace")
	}
	if _, err := ws.createDir("link/made"); err == nil {
		t.Error("createDir through an outbound link should fail")
	}
	if _, err := os.Stat(filepath.Join(outside, "made")); err == nil {
		t.Fatal("a directory was created outside the workspace")
	}
}

// A link that points nowhere would create its target if written through.
func TestDanglingSymlinkIsRefused(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	ws, err := NewWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(outside, "not-yet")
	symlinkOrSkip(t, target, filepath.Join(root, "dangle"))

	if _, err := ws.Resolve("dangle"); err == nil {
		t.Error("Resolve of a dangling link should fail")
	}
	if _, err := ws.writeFile("dangle", "pwned"); err == nil {
		t.Error("writeFile through a dangling link should fail")
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("the link's target was created")
	}
}

// A link that stays inside the project is fine, new files included.
func TestSymlinkInsideTheWorkspaceStillWorks(t *testing.T) {
	root := t.TempDir()
	ws, err := NewWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, filepath.Join(root, "real"), filepath.Join(root, "alias"))

	if _, err := ws.writeFile("alias/new.txt", "ok"); err != nil {
		t.Fatalf("writeFile through an inbound link: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "real", "new.txt")); err != nil {
		t.Fatalf("the file should have landed in real/: %v", err)
	}
}

// run_command's cwd goes through the same check.
func TestShellCwdCannotFollowALinkOut(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	ws, err := NewWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, outside, filepath.Join(root, "link"))

	if _, err := ws.runCommand(context.Background(), runCommandArgs{Command: "echo hi", Cwd: "link"}); err == nil {
		t.Fatal("a cwd that is a link to outside the project should be refused")
	}
	if _, err := ws.runCommand(context.Background(), runCommandArgs{Command: "echo hi", Cwd: ".."}); err == nil {
		t.Fatal("a cwd above the project should be refused")
	}
}

// Files inside .git are run by git itself, so editing one is running code.
func TestFileToolsRefuseTheGitDirectory(t *testing.T) {
	root := t.TempDir()
	ws, err := NewWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, ".git", "config")
	if err := os.WriteFile(config, []byte("[core]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	paths := []string{
		".git/config",
		".git/hooks/pre-commit",
		".GIT/config",
		".git./config",
		"sub/.git/config",
		"sub/../.git/config",
		filepath.Join(root, ".git", "config"),
	}
	for _, path := range paths {
		if _, err := ws.writeFile(path, "[core]\n\tfsmonitor = evil\n"); err == nil || !strings.Contains(err.Error(), ".git") {
			t.Errorf("writeFile(%q) = %v, want a refusal that names .git", path, err)
		}
		if _, err := ws.editFile(editFileArgs{Path: path, OldString: "[core]", NewString: "[x]"}); err == nil || !strings.Contains(err.Error(), ".git") {
			t.Errorf("editFile(%q) = %v, want a refusal that names .git", path, err)
		}
		if _, err := ws.editLines(editLinesArgs{Path: path}); err == nil || !strings.Contains(err.Error(), ".git") {
			t.Errorf("editLines(%q) = %v, want a refusal that names .git", path, err)
		}
		if _, err := ws.createDir(path + "-dir"); err == nil || !strings.Contains(err.Error(), ".git") {
			t.Errorf("createDir(%q) = %v, want a refusal that names .git", path+"-dir", err)
		}
	}
	if data, _ := os.ReadFile(config); string(data) != "[core]\n" {
		t.Fatalf(".git/config was changed: %q", data)
	}

	// Look-alikes are ordinary files.
	for _, path := range []string{".gitignore", "docs/git-notes.md", ".github/CODEOWNERS", "digit/x.txt"} {
		if _, err := ws.writeFile(path, "fine\n"); err != nil {
			t.Errorf("writeFile(%q) = %v, want it allowed", path, err)
		}
	}

	// Reading is unaffected: the model may look at git's files.
	if _, err := ws.Resolve(".git/config"); err != nil {
		t.Errorf("Resolve(.git/config) for reading = %v", err)
	}
}

func TestIsGitComponent(t *testing.T) {
	for name, want := range map[string]bool{
		".git":                    true,
		".GIT":                    true,
		".Git":                    true,
		".git.":                   true,
		".git ":                   true,
		"GIT~1":                   true,
		".git::$INDEX_ALLOCATION": true,
		".gitignore":              false,
		".github":                 false,
		".gitattributes":          false,
		"git":                     false,
		"digit":                   false,
		"":                        false,
	} {
		if got := isGitComponent(name); got != want {
			t.Errorf("isGitComponent(%q) = %v, want %v", name, got, want)
		}
	}
}

// A repository's own configuration must not decide what the host runs. The
// agent can no longer write .git/config, but a repository the person cloned or
// received can still carry a hostile one.
func TestGitSnapshotIgnoresRepositoryFsmonitor(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the probe is a shell script")
	}
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")

	marker := filepath.Join(t.TempDir(), "ran")
	script := filepath.Join(t.TempDir(), "fsmon.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "config", "core.fsmonitor", script)

	GitSnapshot(context.Background(), dir)

	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the repository's fsmonitor program was run by the host")
	}
}

func TestGitSafetyFlagsAreGlobalOptions(t *testing.T) {
	flags := GitSafetyFlags()
	if len(flags)%2 != 0 {
		t.Fatalf("flags come in -c key=value pairs, got %v", flags)
	}
	joined := strings.Join(flags, " ")
	for _, want := range []string{"core.fsmonitor=false", "core.hooksPath=/dev/null", "protocol.ext.allow=never"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %s in %v", want, flags)
		}
	}
}
