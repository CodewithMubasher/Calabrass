package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestGitSnapshotOutsideARepoIsEmpty(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	if got := GitSnapshot(context.Background(), t.TempDir()); got != "" {
		t.Fatalf("a plain folder got a snapshot: %q", got)
	}
}

func TestGitSnapshotDescribesTheTree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	if got := GitSnapshot(context.Background(), dir); !strings.Contains(got, "no commits yet") {
		t.Fatalf("a repo with no commits: %q", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "first commit")
	got := GitSnapshot(context.Background(), dir)
	if !strings.Contains(got, "on branch main") || !strings.Contains(got, "clean") || !strings.Contains(got, "first commit") {
		t.Fatalf("clean tree: %q", got)
	}

	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("two"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0o644)
	got = GitSnapshot(context.Background(), dir)
	if !strings.Contains(got, "1 changed, 1 untracked") || !strings.Contains(got, "the user's, not yours") {
		t.Fatalf("dirty tree: %q", got)
	}
}
