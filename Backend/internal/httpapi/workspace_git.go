package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// workspaceGitInfo is what the Git & GitHub card shows: whether the folder is a
// repository at all, where it points, and what has landed in it. Every field is
// read from the workspace folder itself, so the card describes the repository
// the folder really is rather than a setting remembered elsewhere.
type workspaceGitInfo struct {
	// IsRepo is false when the folder is not a git repository.
	IsRepo bool `json:"isRepo"`
	// RemoteURL is the origin's URL, empty when no remote is set.
	RemoteURL string `json:"remoteUrl"`
	// TotalCommits counts the commits on HEAD; zero on a repository with none.
	TotalCommits int `json:"totalCommits"`
	// LatestCommit is the tip of HEAD, nil while there are no commits.
	LatestCommit *workspaceCommit `json:"latestCommit"`
}

// workspaceCommit is one commit, as the card reads it.
type workspaceCommit struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
	// Date is the commit time, RFC 3339, as git prints it.
	Date string `json:"date"`
}

// gitTimeout bounds a single read. A repository on a network share should not be
// able to hold the card open.
const gitTimeout = 5 * time.Second

// handleWorkspaceGit answers with the workspace folder's own git state.
func (s *Server) handleWorkspaceGit(w http.ResponseWriter, r *http.Request) {
	if s.workspacesUnavailable(w) {
		return
	}
	user := currentUser(r.Context())
	record, err := s.store.GetWorkspace(r.Context(), r.PathValue("workspaceID"), user.ID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	lib := s.libraryFor(user.ID)
	if lib == nil {
		writeError(w, http.StatusServiceUnavailable,
			errors.New("the workspaces folder is unavailable; check the server log"))
		return
	}
	path, err := lib.Path(record.Dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusOK, readGitInfo(r.Context(), path))
}

// readGitInfo reads the folder's repository state. Anything with no repository —
// or a git that is missing or slow — answers with an empty report rather than an
// error: the card has a state for "no repository yet", and a folder outside
// version control is that state, not a failure.
//
// The repository is resolved with `--show-toplevel` rather than
// `--is-inside-work-tree`, both because the former names the repository and
// because a workspace usually sits *inside* one already: the workspaces folder
// lives under the project folder, which is itself a repository. The remote the
// card shows is what says which repository that is.
func readGitInfo(ctx context.Context, dir string) workspaceGitInfo {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	top, err := gitOutput(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return workspaceGitInfo{}
	}

	// Every read is taken from the repository root, so a folder deep inside it
	// reports the same repository rather than a path that happens to work.
	repo := strings.TrimSpace(top)
	if repo == "" {
		return workspaceGitInfo{}
	}

	info := workspaceGitInfo{IsRepo: true}
	info.RemoteURL = gitOrigin(ctx, repo)

	if out, err := gitOutput(ctx, repo, "rev-list", "--count", "HEAD"); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(out)); err == nil {
			info.TotalCommits = n
		}
	}

	// %x09 is a tab, which cannot occur in a subject line's first line but does
	// separate the fields cleanly from a message that contains spaces.
	if out, err := gitOutput(ctx, repo, "log", "-1", "--pretty=format:%h%x09%s%x09%cI"); err == nil {
		if fields := strings.SplitN(out, "\t", 3); len(fields) == 3 {
			info.LatestCommit = &workspaceCommit{
				Hash:    fields[0],
				Message: fields[1],
				Date:    fields[2],
			}
		}
	}

	return info
}

// gitOrigin names the remote the card shows: origin when it is set, otherwise
// the first remote there is, so a repository cloned under another name still
// shows where it points.
func gitOrigin(ctx context.Context, dir string) string {
	if out, err := gitOutput(ctx, dir, "remote", "get-url", "origin"); err == nil {
		return strings.TrimSpace(out)
	}
	out, err := gitOutput(ctx, dir, "remote")
	if err != nil {
		return ""
	}
	for _, name := range strings.Fields(out) {
		if url, err := gitOutput(ctx, dir, "remote", "get-url", name); err == nil {
			return strings.TrimSpace(url)
		}
	}
	return ""
}

// gitOutput runs one git command inside the folder. A failure — git missing, a
// timeout, a non-zero exit — comes back as an error the caller reads as "no
// answer", never as a panic.
func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
