package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/proc"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// workspaceGitInfo is what the Git & GitHub card shows: the repository the
// workspace holds, where it points, what has landed in it, and whether it is
// visible to anyone else. Every field is read from the folder itself, so the
// card describes the repository that is really there.
type workspaceGitInfo struct {
	// IsRepo is false until a repository has been cloned in.
	IsRepo bool `json:"isRepo"`
	// RemoteURL is the origin's URL, empty when no remote is set.
	RemoteURL string `json:"remoteUrl"`
	// TotalCommits counts the commits on HEAD; zero on a repository with none.
	TotalCommits int `json:"totalCommits"`
	// LatestCommit is the tip of HEAD, nil while there are no commits.
	LatestCommit *workspaceCommit `json:"latestCommit"`
	// Visibility is "public" or "private" when it is known, and empty when it is
	// not — a remote that is not on GitHub, or a GitHub that did not answer.
	Visibility string `json:"visibility"`
}

// workspaceCommit is one commit, as the card reads it.
type workspaceCommit struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
	// Date is the commit time, RFC 3339, as git prints it.
	Date string `json:"date"`
}

const (
	// gitTimeout bounds a single read. A repository on a network share should not
	// be able to hold the card open.
	gitTimeout = 5 * time.Second
	// cloneTimeout bounds a clone. A large repository over a slow link is still a
	// clone the user asked for, so the budget is minutes rather than seconds.
	cloneTimeout = 5 * time.Minute
	// visibilityTimeout bounds the GitHub lookup. The card is worth showing
	// without the tag, so a slow network never holds it up.
	visibilityTimeout = 3 * time.Second
)

// handleWorkspaceGit answers with the repository the workspace holds.
func (s *Server) handleWorkspaceGit(w http.ResponseWriter, r *http.Request) {
	path, ok := s.workspacePath(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, readGitInfo(r.Context(), path))
}

// handleCloneWorkspaceRepo clones a repository into the workspace folder, where
// git gives it a folder of its own, and answers with the state that follows. The
// reply is a fresh read rather than the request echoed back, so the card shows
// what actually landed on disk.
func (s *Server) handleCloneWorkspaceRepo(w http.ResponseWriter, r *http.Request) {
	path, ok := s.workspacePath(w, r)
	if !ok {
		return
	}

	var body struct {
		RemoteURL string `json:"remoteUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("the request body is not valid JSON"))
		return
	}
	remote := strings.TrimSpace(body.RemoteURL)
	if remote == "" {
		writeError(w, http.StatusBadRequest, errors.New("enter a repository URL"))
		return
	}

	if err := cloneInto(r.Context(), path, remote); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, readGitInfo(r.Context(), path))
}

// workspacePath resolves the folder behind the workspace in the URL. It writes
// the failure response itself, so the caller only has to stop when it reports
// false.
func (s *Server) workspacePath(w http.ResponseWriter, r *http.Request) (string, bool) {
	if s.workspacesUnavailable(w) {
		return "", false
	}
	user := currentUser(r.Context())
	record, err := s.store.GetWorkspace(r.Context(), r.PathValue("workspaceID"), user.ID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
		return "", false
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
		return "", false
	}

	lib := s.libraryFor(user.ID)
	if lib == nil {
		writeError(w, http.StatusServiceUnavailable,
			errors.New("the workspaces folder is unavailable; check the server log"))
		return "", false
	}
	path, err := lib.Path(record.Dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return "", false
	}
	return path, true
}

// readGitInfo reads the state of the repository a workspace holds. A workspace
// with nothing cloned into it reports as much rather than an error: that is the
// state the card draws its clone form in.
func readGitInfo(ctx context.Context, dir string) workspaceGitInfo {
	repo, ok := repoDir(dir)
	if !ok {
		return workspaceGitInfo{}
	}

	gitCtx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	info := workspaceGitInfo{IsRepo: true}
	info.RemoteURL = gitOrigin(gitCtx, repo)

	if out, err := gitOutput(gitCtx, repo, "rev-list", "--count", "HEAD"); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(out)); err == nil {
			info.TotalCommits = n
		}
	}

	// %x09 is a tab, which cannot occur in a subject line's first line but does
	// separate the fields cleanly from a message that contains spaces.
	if out, err := gitOutput(gitCtx, repo, "log", "-1", "--pretty=format:%h%x09%s%x09%cI"); err == nil {
		if fields := strings.SplitN(out, "\t", 3); len(fields) == 3 {
			info.LatestCommit = &workspaceCommit{
				Hash:    fields[0],
				Message: fields[1],
				Date:    fields[2],
			}
		}
	}

	info.Visibility = gitVisibility(ctx, info.RemoteURL)
	return info
}

// repoDir names the repository a workspace holds: the folder itself when it is
// one, otherwise the repository cloned into it. A clone lands in a folder of its
// own — the workspace is the container — so a connected workspace keeps its
// repository one level down. When several are there, the most recently written
// wins, which is the one the user just cloned.
func repoDir(dir string) (string, bool) {
	if isRepoRoot(dir) {
		return dir, true
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}

	best := ""
	var bestAt time.Time
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		child := filepath.Join(dir, entry.Name())
		if !isRepoRoot(child) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestAt) {
			best, bestAt = child, info.ModTime()
		}
	}
	if best == "" {
		return "", false
	}
	return best, true
}

// isRepoRoot reports whether a folder is the top of a repository of its own. The
// check is a repository's own `.git`, which is also the thing a plain folder
// inside somebody else's repository does not have — the workspaces folder lives
// under the project folder, and every workspace in it would otherwise read as
// "inside a repository" and show the project's commits.
func isRepoRoot(dir string) bool {
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
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
		if remote, err := gitOutput(ctx, dir, "remote", "get-url", name); err == nil {
			return strings.TrimSpace(remote)
		}
	}
	return ""
}

// cloneInto clones remote into dir. git names the folder after the repository,
// so whatever else the workspace holds is left alone.
func cloneInto(ctx context.Context, dir, remote string) error {
	ctx, cancel := context.WithTimeout(ctx, cloneTimeout)
	defer cancel()

	// The URL goes after `--` so a remote that begins with a dash is read as an
	// address rather than as another option.
	cmd := exec.CommandContext(ctx, "git", "clone", "--", remote)
	cmd.Dir = dir
	proc.Hide(cmd)

	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if message := strings.TrimSpace(string(out)); message != "" {
		return errors.New(message)
	}
	return err
}

// gitOutput runs one git command inside the folder. A failure — git missing, a
// timeout, a non-zero exit — comes back as an error the caller reads as "no
// answer", never as a panic.
func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	proc.Hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// gitVisibility asks GitHub whether a repository is public. An anonymous request
// cannot see a private repository, so a repository GitHub does not know — one
// that is nonetheless cloned here, which is what proves it exists — is private.
// Anything else, including a GitHub that did not answer, is left unknown.
func gitVisibility(ctx context.Context, remote string) string {
	owner, name, ok := githubSlug(remote)
	if !ok {
		return ""
	}

	if cached, ok := visibilityOf(owner, name); ok {
		return cached
	}

	visibility := fetchVisibility(ctx, owner, name)
	if visibility != "" {
		rememberVisibility(owner, name, visibility)
	}
	return visibility
}

// visibilityCache remembers what GitHub said, so reopening a workspace does not
// spend another request on it: anonymous GitHub requests are limited per hour,
// and the answer rarely changes.
var (
	visibilityMu    sync.Mutex
	visibilityCache = map[string]string{}
)

func visibilityOf(owner, name string) (string, bool) {
	visibilityMu.Lock()
	defer visibilityMu.Unlock()
	visibility, ok := visibilityCache[owner+"/"+name]
	return visibility, ok
}

func rememberVisibility(owner, name, visibility string) {
	visibilityMu.Lock()
	defer visibilityMu.Unlock()
	visibilityCache[owner+"/"+name] = visibility
}

func fetchVisibility(ctx context.Context, owner, name string) string {
	ctx, cancel := context.WithTimeout(ctx, visibilityTimeout)
	defer cancel()

	endpoint := "https://api.github.com/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	// GitHub refuses a request that does not say who it is from.
	req.Header.Set("User-Agent", "Calabrass")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		var body struct {
			Private bool `json:"private"`
		}
		if err := json.NewDecoder(io.LimitReader(res.Body, 1<<16)).Decode(&body); err != nil {
			return ""
		}
		if body.Private {
			return "private"
		}
		return "public"
	case http.StatusNotFound:
		// Not visible to an anonymous request, and the clone says it is there.
		return "private"
	default:
		return ""
	}
}

// githubSlug reads the owner and repository out of a remote, for the address
// forms git accepts: https, ssh, and the scp-like git@github.com:owner/repo.
func githubSlug(remote string) (string, string, bool) {
	remote = strings.TrimSpace(remote)

	var path string
	if rest, ok := strings.CutPrefix(remote, "git@"); ok {
		host, after, ok := strings.Cut(rest, ":")
		if !ok || !strings.EqualFold(host, "github.com") {
			return "", "", false
		}
		path = after
	} else {
		parsed, err := url.Parse(remote)
		if err != nil || !strings.EqualFold(parsed.Host, "github.com") {
			return "", "", false
		}
		path = strings.TrimPrefix(parsed.Path, "/")
	}

	path = strings.TrimSuffix(path, ".git")
	owner, name, ok := strings.Cut(path, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", "", false
	}
	return owner, name, true
}

// handleWorkspaceCommand runs a shell command in the workspace folder.
// The command runs in the background with no terminal window.
func (s *Server) handleWorkspaceCommand(w http.ResponseWriter, r *http.Request) {
	path, ok := s.workspacePath(w, r)
	if !ok {
		return
	}

	var body struct {
		Command string `json:"command"`
	}
	// A command is a line of text. Anything much bigger is not one.
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("the request body is not valid JSON"))
		return
	}
	command := strings.TrimSpace(body.Command)
	if command == "" {
		writeError(w, http.StatusBadRequest, errors.New("enter a command"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	name, args := userShell(command)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = path
	proc.Hide(cmd)

	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"output":    string(out),
		"exitCode":  exitCode,
	})
}

// userShell is how a command the person typed into the workspace's command box
// is run on this machine: the Windows command interpreter there, the system
// shell everywhere else. It used to be cmd.exe unconditionally, which does not
// exist on macOS or Linux, where the command line ships too.
//
// This is the person's own command, typed by them, so it is not put through the
// agent's approval step; the agent's run_command is.
func userShell(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", command}
	}
	return "/bin/sh", []string{"-c", command}
}
