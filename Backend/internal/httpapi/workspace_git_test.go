package httpapi

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitAvailable is the whole test's precondition: the reader shells out to git,
// so a machine without it has nothing to test rather than a failing test.
func gitAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// A folder with no repository of its own reports the one it sits in — the case
// the workspaces folder is in, since it lives under the project's own checkout.
func TestReadGitInfoInsideParentRepository(t *testing.T) {
	gitAvailable(t)

	parent := t.TempDir()
	git(t, parent, "init")
	child := filepath.Join(parent, "workspace")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}

	info := readGitInfo(context.Background(), child)
	if !info.IsRepo {
		t.Fatal("a folder inside a repository should report one")
	}
}

// A folder outside any repository reports none, which is what the card reads as
// "not a git repository yet".
func TestReadGitInfoNoRepository(t *testing.T) {
	gitAvailable(t)

	info := readGitInfo(context.Background(), t.TempDir())
	if info.IsRepo {
		t.Fatal("a folder outside any repository should report none")
	}
	if info.LatestCommit != nil || info.TotalCommits != 0 || info.RemoteURL != "" {
		t.Errorf("empty report expected, got %+v", info)
	}
}

// A repository with a commit reports its count, its tip and its remote.
func TestReadGitInfoRepository(t *testing.T) {
	gitAvailable(t)

	dir := t.TempDir()
	git(t, dir, "init")
	git(t, dir, "config", "user.email", "test@example.com")
	git(t, dir, "config", "user.name", "Test")
	git(t, dir, "remote", "add", "origin", "https://example.com/repo.git")

	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "first commit")

	info := readGitInfo(context.Background(), dir)
	if !info.IsRepo {
		t.Fatal("a repository should report itself")
	}
	if info.TotalCommits != 1 {
		t.Errorf("TotalCommits = %d, want 1", info.TotalCommits)
	}
	if info.RemoteURL != "https://example.com/repo.git" {
		t.Errorf("RemoteURL = %q", info.RemoteURL)
	}
	if info.LatestCommit == nil {
		t.Fatal("LatestCommit is nil after a commit")
	}
	if info.LatestCommit.Message != "first commit" {
		t.Errorf("LatestCommit.Message = %q", info.LatestCommit.Message)
	}
	if len(info.LatestCommit.Hash) < 7 {
		t.Errorf("LatestCommit.Hash = %q, want a short hash", info.LatestCommit.Hash)
	}
	if info.LatestCommit.Date == "" {
		t.Error("LatestCommit.Date is empty")
	}
}

// An empty repository — initialised, no commits — is still a repository, and
// reports no tip rather than failing.
func TestReadGitInfoEmptyRepository(t *testing.T) {
	gitAvailable(t)

	dir := t.TempDir()
	git(t, dir, "init")

	info := readGitInfo(context.Background(), dir)
	if !info.IsRepo {
		t.Fatal("an initialised repository should report itself")
	}
	if info.TotalCommits != 0 {
		t.Errorf("TotalCommits = %d, want 0", info.TotalCommits)
	}
	if info.LatestCommit != nil {
		t.Errorf("LatestCommit = %+v, want nil", info.LatestCommit)
	}
}
