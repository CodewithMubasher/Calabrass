// Package agent is the harness: the workspace the model may touch, the tools it
// may use, and the prompt that explains both. It knows nothing about HTTP or
// the database.
package agent

import (
	"errors"
	"fmt"
	"io/fs"
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
//
// Symbolic links are followed before the containment check, including for a
// file that does not exist yet: the deepest part of the path that does exist is
// resolved, and the missing tail is put back after it. Without that, writing
// "link/new.txt" through a link that points outside the project would pass a
// purely lexical check.
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

	real, err := evalExisting(candidate)
	if err != nil {
		return "", fmt.Errorf("%s cannot be used: %w", path, err)
	}
	if !w.contains(real) {
		return "", fmt.Errorf("%s is outside the workspace (%s)", path, w.root)
	}
	return real, nil
}

// evalExisting resolves symbolic links in p even when its last parts do not
// exist yet. It fails closed: a link that points nowhere, or any lookup error
// other than "not found", is an error, never a reason to trust the spelling.
func evalExisting(p string) (string, error) {
	tail := ""
	cur := p
	for {
		real, err := filepath.EvalSymlinks(cur)
		if err == nil {
			if tail == "" {
				return real, nil
			}
			return filepath.Join(real, tail), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		// EvalSymlinks also reports "not found" for a dangling link. Writing
		// through one would create its target, wherever that is.
		if _, lerr := os.Lstat(cur); lerr == nil {
			return "", errors.New("it is a symbolic link that points nowhere")
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", err
		}
		tail = filepath.Join(filepath.Base(cur), tail)
		cur = parent
	}
}

// ResolveForWrite is Resolve for a path a tool is about to change. On top of
// the containment check it refuses anything inside .git: a file there (config,
// hooks) is run by git itself the next time anyone uses the repository, so
// editing it is running code. Git changes go through git, via run_command,
// where they are classified and asked about like any other command.
func (w *Workspace) ResolveForWrite(path string) (string, error) {
	abs, err := w.Resolve(path)
	if err != nil {
		return "", err
	}
	if w.insideGitDir(path) || hasGitComponent(w.Rel(abs)) {
		return "", fmt.Errorf("%s is inside .git, which the file tools may not change; use git commands through run_command instead", path)
	}
	return abs, nil
}

// insideGitDir looks at the path as the model spelled it, relative to the root.
func (w *Workspace) insideGitDir(path string) bool {
	path = strings.TrimSpace(path)
	if filepath.IsAbs(path) {
		rel, err := filepath.Rel(w.root, filepath.Clean(path))
		if err != nil {
			return false
		}
		path = rel
	}
	return hasGitComponent(path)
}

// hasGitComponent reports whether any part of a relative path names a git
// directory, under every spelling the file system may treat as the same name.
func hasGitComponent(rel string) bool {
	parts := strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' })
	for _, part := range parts {
		if isGitComponent(part) {
			return true
		}
	}
	return false
}

// isGitComponent is deliberately wider than ".git": Windows ignores trailing
// dots and spaces and has 8.3 short names (GIT~1) and alternate data streams
// (.git::$INDEX_ALLOCATION), and macOS and Windows compare names without case.
func isGitComponent(name string) bool {
	name = strings.ToLower(strings.TrimRight(name, ". "))
	return name == ".git" || strings.HasPrefix(name, ".git:") || strings.HasPrefix(name, "git~")
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
