package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultWorkspaceName is the folder a chat works in when it has not chosen a
// workspace of its own.
const DefaultWorkspaceName = "default"

// Library is the folder that holds every workspace: <project>/workspace unless
// it is configured elsewhere. A workspace is one directory inside it, and the
// harness is confined to whichever one it was opened on — nothing in the
// library lets a tool reach a sibling, or the project around it.
type Library struct {
	root string
}

// NewLibrary pins the workspaces folder, creating it when it is missing.
func NewLibrary(dir string) (*Library, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("agent: workspaces folder is empty")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("agent: resolve workspaces folder %q: %w", dir, err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("agent: create workspaces folder %s: %w", abs, err)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	return &Library{root: abs}, nil
}

// Root is the folder every workspace lives in.
func (l *Library) Root() string { return l.root }

// Path is where one workspace sits on disk. The folder name is checked first,
// so a name that arrived from the database can never point outside the library.
func (l *Library) Path(dir string) (string, error) {
	name, err := workspaceDir(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(l.root, name), nil
}

// Ensure creates the workspace folder when it is missing, and opens it.
func (l *Library) Ensure(dir string) (*Workspace, error) {
	path, err := l.Path(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, fmt.Errorf("agent: create workspace %s: %w", path, err)
	}
	return NewWorkspace(path)
}

// Exists reports whether the workspace folder is on disk.
func (l *Library) Exists(dir string) bool {
	path, err := l.Path(dir)
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// Unique picks a folder name for a new workspace that nothing is using, adding
// a number when the name is taken.
func (l *Library) Unique(name string) string {
	base := Slug(name)
	candidate := base
	for n := 2; n <= 99; n++ {
		if _, err := os.Stat(filepath.Join(l.root, candidate)); err != nil {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, n)
	}
	return candidate
}

// workspaceDir checks a stored folder name: exactly one path segment, and one
// that cannot climb out of the library.
func workspaceDir(dir string) (string, error) {
	name := strings.TrimSpace(dir)
	if name == "" {
		return "", errors.New("agent: workspace folder is empty")
	}
	if name == "." || name == ".." || name != filepath.Base(name) {
		return "", fmt.Errorf("agent: %q is not a workspace folder", dir)
	}
	if filepath.IsAbs(name) || strings.ContainsAny(name, `/\:`) {
		return "", fmt.Errorf("agent: %q is not a workspace folder", dir)
	}
	return name, nil
}

// slugPattern matches everything a folder name should not keep. Letters and
// digits from any script survive, so a name is still recognisable on disk.
var slugPattern = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// Slug turns a workspace name into a folder name.
func Slug(name string) string {
	slug := strings.Trim(slugPattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-"), "-")
	if slug == "" {
		return "workspace"
	}
	return slug
}
