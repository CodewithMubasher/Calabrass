// Package workbook is the part of this server that touches the disk: where a
// path is allowed to point, how a workbook is opened, changed and written back,
// and the small conversions between what a model says ("A1:C4", "#,##0.00")
// and what the spreadsheet library wants.
//
// Every call opens the file, makes its change and saves it again. That is the
// simplest promise to keep — the file on disk is the truth at all times, so a
// file made in one call is there for the next one — and it is why the protocol
// serves one message at a time.
package workbook

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/xuri/excelize/v2"
)

const (
	// Extension is the workbook format this server writes. Only .xlsx is
	// supported: the one thing a spreadsheet must not do is open a file, change
	// it and quietly drop something the format carries that the library does
	// not — which is what happens to the macros in an .xlsm.
	Extension = ".xlsx"

	// RootEnv overrides the folder relative paths are taken from.
	RootEnv = "CALABRASS_EXCEL_ROOT"
	// AllowOutsideEnv lifts the rule that a path must stay inside that folder.
	AllowOutsideEnv = "CALABRASS_EXCEL_ALLOW_OUTSIDE"

	// maxPath is as long as a path this server will act on may be.
	maxPath = 400
)

// Root is the folder every relative path is taken from, and the boundary an
// absolute path has to stay inside.
type Root struct {
	dir          string
	allowOutside bool
}

// NewRoot picks the folder. The order is: the -root flag, CALABRASS_EXCEL_ROOT,
// then Documents\Calabrass under the user's home — a spreadsheet is something
// the person asked for, not part of the project, so it does not land in the
// checkout by default.
func NewRoot(flag string) (*Root, error) {
	dir := strings.TrimSpace(flag)
	if dir == "" {
		dir = strings.TrimSpace(os.Getenv(RootEnv))
	}
	if dir == "" {
		dir = defaultRoot()
	}
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("no folder to work in, and the current one could not be read: %w", err)
		}
		dir = cwd
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", dir, err)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	return &Root{
		dir:          abs,
		allowOutside: strings.TrimSpace(os.Getenv(AllowOutsideEnv)) != "",
	}, nil
}

// Dir is the folder relative paths are resolved against, as the model is told it.
func (r *Root) Dir() string { return r.dir }

// AllowOutside reports whether absolute paths beyond the root are accepted.
func (r *Root) AllowOutside() bool { return r.allowOutside }

func defaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	if info, err := os.Stat(filepath.Join(home, "Documents")); err == nil && info.IsDir() {
		return filepath.Join(home, "Documents", "Calabrass")
	}
	return filepath.Join(home, "Calabrass")
}

// ResolveWorkbook turns a model's path into the file to act on: `.xlsx` is added
// when no extension was given, the result is made absolute, and it is held
// inside the root unless this server was told otherwise.
func (r *Root) ResolveWorkbook(path string) (string, error) {
	return r.resolve(path, Extension, []string{Extension})
}

// ResolveFile is ResolveWorkbook for a file that is not a workbook, such as the
// CSV that export_csv writes.
func (r *Root) ResolveFile(path, extension string, allowed ...string) (string, error) {
	return r.resolve(path, extension, allowed)
}

func (r *Root) resolve(path, extension string, allowed []string) (string, error) {
	name := strings.TrimSpace(path)
	if name == "" {
		return "", errors.New("a file path is required")
	}
	if len(name) > maxPath {
		return "", fmt.Errorf("the path is longer than %d characters", maxPath)
	}
	if strings.ContainsAny(name, "\x00\r\n") {
		return "", errors.New("the path must not contain control characters")
	}

	// The extension decides the format, so it is checked rather than left to the
	// library: writing an .xlsx into a name that says .csv is a file nobody can
	// open.
	ext := strings.ToLower(filepath.Ext(name))
	switch {
	case ext == "":
		name += extension
	case contains(allowed, ext):
	default:
		return "", fmt.Errorf("%q is not a file this server writes: the extension must be one of %s",
			filepath.Base(name), strings.Join(allowed, ", "))
	}

	abs := name
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(r.dir, abs)
	}
	abs = filepath.Clean(abs)

	if !r.allowOutside && !inside(r.dir, abs) {
		return "", fmt.Errorf("%q is outside the folder this server works in (%s): "+
			"give a path inside it, or set %s", filepath.Base(abs), r.dir, AllowOutsideEnv)
	}
	return abs, nil
}

// inside reports whether a path is at or under a folder.
func inside(folder, path string) bool {
	rel, err := filepath.Rel(folder, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

// Relative renders a path the way the model should read it: relative to the
// root when it is inside it, absolute otherwise.
func (r *Root) Relative(path string) string {
	rel, err := filepath.Rel(r.dir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.ToSlash(rel)
}

// Listing is one file a directory holds.
type Listing struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime string `json:"modified"`
}

// List returns the spreadsheets under a directory, newest first, walking
// subdirectories when asked.
func (r *Root) List(dir string, recursive bool) ([]Listing, error) {
	start := r.dir
	if strings.TrimSpace(dir) != "" {
		resolved, err := r.resolveDir(dir)
		if err != nil {
			return nil, err
		}
		start = resolved
	}

	var found []Listing
	walk := func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if path != start && (!recursive || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), Extension) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		found = append(found, Listing{
			Path:    r.Relative(path),
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04"),
		})
		return nil
	}
	if err := filepath.WalkDir(start, walk); err != nil {
		return nil, fmt.Errorf("read %s: %w", start, err)
	}
	return found, nil
}

func (r *Root) resolveDir(dir string) (string, error) {
	name := strings.TrimSpace(dir)
	if name == "" {
		return r.dir, nil
	}
	abs := name
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(r.dir, abs)
	}
	abs = filepath.Clean(abs)
	if !r.allowOutside && !inside(r.dir, abs) {
		return "", fmt.Errorf("%q is outside the folder this server works in (%s)", filepath.Base(abs), r.dir)
	}
	return abs, nil
}

// Book is an open workbook.
type Book struct {
	file *excelize.File
	path string
}

// Open reads a workbook that is already there.
func Open(path string) (*Book, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("there is no workbook at %s: create it with create_workbook, or list_workbooks to find it", path)
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a folder, not a workbook", path)
	}
	if !strings.EqualFold(filepath.Ext(path), Extension) {
		return nil, fmt.Errorf("%s is not a %s workbook", filepath.Base(path), Extension)
	}

	file, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s could not be opened as a workbook: %w", filepath.Base(path), err)
	}
	return &Book{file: file, path: path}, nil
}

// Create makes a new workbook with the given sheets, in order. It refuses to
// stand on an existing file unless it was asked to overwrite, because losing a
// workbook to a mistyped name is not a mistake this server should be able to
// make.
func Create(path string, sheets []string, overwrite bool) (*Book, error) {
	if _, err := os.Stat(path); err == nil && !overwrite {
		return nil, fmt.Errorf("%s already exists: pass overwrite true to replace it, "+
			"or use another name", filepath.Base(path))
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create the folder for %s: %w", filepath.Base(path), err)
	}

	names := make([]string, 0, len(sheets))
	for _, name := range sheets {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		if err := ValidSheetName(trimmed); err != nil {
			return nil, err
		}
		names = append(names, trimmed)
	}

	file := excelize.NewFile()
	// NewFile always starts with a sheet of its own; it is renamed into the
	// first sheet asked for, or kept when nothing was asked for.
	const fresh = "Sheet1"
	if len(names) == 0 {
		names = []string{fresh}
	}
	if err := file.SetSheetName(fresh, names[0]); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("name the first sheet %q: %w", names[0], err)
	}
	for _, name := range names[1:] {
		if _, err := file.NewSheet(name); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("add the sheet %q: %w", name, err)
		}
	}

	book := &Book{file: file, path: path}
	return book, nil
}

// ValidSheetName checks what a sheet may be called: a name Excel accepts, which
// is short, not empty and free of the characters a formula would read as syntax.
func ValidSheetName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("a sheet name must not be empty")
	}
	if len([]rune(trimmed)) > 31 {
		return fmt.Errorf("the sheet name %q is too long: Excel allows 31 characters", trimmed)
	}
	if trimmed != name {
		return fmt.Errorf("the sheet name %q must not start or end with a space", name)
	}
	if strings.ContainsAny(trimmed, `:\/?*[]`) {
		return fmt.Errorf(`the sheet name %q must not contain any of : \ / ? * [ ]`, trimmed)
	}
	return nil
}

// File is the workbook as the library holds it.
func (b *Book) File() *excelize.File { return b.file }

// Path is where the workbook is on disk.
func (b *Book) Path() string { return b.path }

// Name is the file name without its folder.
func (b *Book) Name() string { return filepath.Base(b.path) }

// Relative is how the model should read the path.
func (b *Book) Relative(root *Root) string { return root.Relative(b.path) }

// Save writes the workbook back to the file it was opened from. It is the moment
// the change becomes real, so a failure here is reported as the tool's failure.
func (b *Book) Save() error {
	if err := b.file.SaveAs(b.path); err != nil {
		return fmt.Errorf("%s could not be saved: %w", b.Name(), err)
	}
	return nil
}

// Close releases the workbook.
func (b *Book) Close() error {
	if b.file == nil {
		return nil
	}
	return b.file.Close()
}

// Guard serialises calls against each other. One workbook is opened, changed
// and saved per call, and two calls racing on the same file would save over each
// other's changes.
type Guard struct{ mu sync.Mutex }

// Do runs one call with nothing else touching a workbook.
func (g *Guard) Do(work func() (string, error)) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return work()
}
