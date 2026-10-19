package httpapi

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitAvailable is the whole suite's precondition: these tests shell out to git,
// so a machine without it has nothing to test rather than failing tests.
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

// sourceRepo builds a repository with one commit, to clone from.
func sourceRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init")
	git(t, dir, "config", "user.email", "test@example.com")
	git(t, dir, "config", "user.name", "Test")
	git(t, dir, "remote", "add", "origin", "https://github.com/example/repo.git")
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "first commit")
	return dir
}

// A workspace with nothing cloned into it reports no repository — the state the
// card draws its clone form in.
func TestReadGitInfoEmptyWorkspace(t *testing.T) {
	gitAvailable(t)

	info := readGitInfo(context.Background(), t.TempDir())
	if info.IsRepo {
		t.Fatal("an empty workspace should report no repository")
	}
	if info.TotalCommits != 0 || info.LatestCommit != nil || info.RemoteURL != "" {
		t.Errorf("empty report expected, got %+v", info)
	}
}

// The workspaces folder lives under the project folder, which is itself a
// repository. A workspace in it must still report nothing rather than borrow the
// project's commits.
func TestReadGitInfoInsideParentRepository(t *testing.T) {
	gitAvailable(t)

	parent := t.TempDir()
	git(t, parent, "init")
	child := filepath.Join(parent, "workspace", "my-workspace")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}

	if info := readGitInfo(context.Background(), child); info.IsRepo {
		t.Errorf("a folder inside another repository should report none, got %+v", info)
	}
}

// Cloning puts the repository in a folder of its own, and the card reads it from
// there: the remote it was cloned from, its commits, and its tip.
func TestCloneIntoWorkspace(t *testing.T) {
	gitAvailable(t)

	workspace := t.TempDir()
	// A file already in the workspace does not matter: the clone gets its own
	// folder beside it.
	if err := os.WriteFile(filepath.Join(workspace, "notes.md"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cloneInto(context.Background(), workspace, sourceRepo(t)); err != nil {
		t.Fatalf("clone: %v", err)
	}

	info := readGitInfo(context.Background(), workspace)
	if !info.IsRepo {
		t.Fatal("the cloned repository should be found")
	}
	if info.TotalCommits != 1 {
		t.Errorf("TotalCommits = %d, want 1", info.TotalCommits)
	}
	// Cloning from a folder records that folder as origin, not the source
	// repository's own origin.
	if info.RemoteURL == "" {
		t.Error("RemoteURL is empty after a clone")
	}
	if info.Visibility != "" {
		t.Errorf("Visibility = %q, want empty for a remote that is not GitHub", info.Visibility)
	}
	if info.LatestCommit == nil {
		t.Fatal("LatestCommit is nil after a clone")
	}
	if info.LatestCommit.Message != "first commit" {
		t.Errorf("LatestCommit.Message = %q", info.LatestCommit.Message)
	}
	if info.LatestCommit.Date == "" {
		t.Error("LatestCommit.Date is empty")
	}
}

// A workspace that is itself a repository — one that was initialised in place
// rather than cloned into — is read directly, rather than looked past.
func TestReadGitInfoWorkspaceRoot(t *testing.T) {
	gitAvailable(t)

	dir := sourceRepo(t)
	info := readGitInfo(context.Background(), dir)
	if !info.IsRepo {
		t.Fatal("a workspace that is a repository should report itself")
	}
	if info.TotalCommits != 1 {
		t.Errorf("TotalCommits = %d, want 1", info.TotalCommits)
	}
	if info.LatestCommit == nil || info.LatestCommit.Message != "first commit" {
		t.Errorf("LatestCommit = %+v", info.LatestCommit)
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

// A clone that cannot work reports what git said, so the card can show the
// reason rather than a bare failure.
func TestCloneIntoWorkspaceBadRemote(t *testing.T) {
	gitAvailable(t)

	workspace := t.TempDir()
	// A relative name that is not there: an absolute Windows path would be read
	// by git as a scp-like address and answered with an empty repository.
	err := cloneInto(context.Background(), workspace, "missing-repo")
	if err == nil {
		t.Fatal("cloning a missing repository should fail")
	}
	if readGitInfo(context.Background(), workspace).IsRepo {
		t.Error("a failed clone should leave no repository behind")
	}
}

// The address forms git accepts all name the same repository.
func TestGithubSlug(t *testing.T) {
	cases := []struct {
		remote string
		owner  string
		name   string
		ok     bool
	}{
		{"https://github.com/CodewithMubasher/Calabrass.git", "CodewithMubasher", "Calabrass", true},
		{"https://github.com/CodewithMubasher/Calabrass", "CodewithMubasher", "Calabrass", true},
		{"git@github.com:CodewithMubasher/Calabrass.git", "CodewithMubasher", "Calabrass", true},
		{"ssh://git@github.com/CodewithMubasher/Calabrass.git", "CodewithMubasher", "Calabrass", true},
		{"https://gitlab.com/CodewithMubasher/Calabrass.git", "", "", false},
		{"https://github.com/CodewithMubasher", "", "", false},
		{"https://github.com/a/b/c", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		owner, name, ok := githubSlug(c.remote)
		if ok != c.ok || owner != c.owner || name != c.name {
			t.Errorf("githubSlug(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.remote, owner, name, ok, c.owner, c.name, c.ok)
		}
	}
}
