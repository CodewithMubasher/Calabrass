// Package agent is the harness: the workspace the model may touch, the tools it
// may use, and the prompt that explains both. It knows nothing about HTTP or
// the database.
package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Workspace is the directory tree the harness is allowed to touch. Every path a
// tool is handed goes through it, so a model cannot read or write outside the
// project the user opened.
type Workspace struct {
	root string
}

// NewWorkspace pins the harness to one directory, which must exist.
func NewWorkspace(root string) (*Workspace, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("agent: workspace root is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("agent: resolve workspace %q: %w", root, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("agent: workspace %s: %w", abs, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("agent: workspace %s is not a directory", abs)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	return &Workspace{root: abs}, nil
}

// Root is the absolute workspace directory.
func (w *Workspace) Root() string { return w.root }

// Resolve turns a model-supplied path into an absolute path inside the
// workspace. An empty path, or ".", means the workspace root itself. Paths that
// climb out of the workspace are refused.
func (w *Workspace) Resolve(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "." {
		return w.root, nil
	}

	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(w.root, candidate)
	}
	candidate = filepath.Clean(candidate)

	// Follow links so a symlink cannot be used to leave the workspace. The
	// lookup fails for a path that does not exist yet (writes and reads of a
	// missing file alike), which is fine: the lexical check still applies.
	if real, err := filepath.EvalSymlinks(candidate); err == nil {
		candidate = real
	}

	if !w.contains(candidate) {
		return "", fmt.Errorf("%s is outside the workspace (%s)", path, w.root)
	}
	return candidate, nil
}

// Rel renders an absolute path the way the model should see it.
func (w *Workspace) Rel(abs string) string {
	rel, err := filepath.Rel(w.root, abs)
	if err != nil || rel == "." {
		return "."
	}
	return filepath.ToSlash(rel)
}

func (w *Workspace) contains(path string) bool {
	root, candidate := w.root, path
	if runtime.GOOS == "windows" {
		root, candidate = strings.ToLower(root), strings.ToLower(candidate)
	}
	if candidate == root {
		return true
	}
	return strings.HasPrefix(candidate, root+string(os.PathSeparator))
}

// DiscoverRoot walks up from dir looking for the top of a project (a .git
// directory), so the harness defaults to the repository the backend lives in.
// It falls back to dir itself when nothing is found.
func DiscoverRoot(dir string) string {
	if strings.TrimSpace(dir) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "."
		}
		dir = wd
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	for current := abs; ; {
		if info, err := os.Stat(filepath.Join(current, ".git")); err == nil && info.IsDir() {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs
		}
		current = parent
	}
}
