package agent

import (
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/zendev-sh/goai"
)

// DefaultMaxSteps bounds how many model turns one request may take before the
// loop stops. A step is one model call plus whatever tools it asked for.
const DefaultMaxSteps = 12

const (
	// readFileLines is how many lines read_file returns when the model does not
	// ask for a window of its own.
	readFileLines = 400
	// readFileMaxBytes caps one read. It is the guard that keeps a read from
	// filling the context by itself: 32 KB is roughly eight thousand tokens, and
	// a file with very long lines reaches that in a few dozen of them.
	readFileMaxBytes = 32 << 10
	// readFileLineBytes caps a single line, so minified files stay readable.
	readFileLineBytes = 2000
	// binarySniffBytes is how much of a file is checked for NUL bytes before
	// it is treated as text.
	binarySniffBytes = 8000

	// listDirEntries is how many directory entries list_dir returns.
	listDirEntries = 500
	// globMaxPaths is how many paths a glob returns.
	globMaxPaths = 200
	// grepMaxMatches is how many matching lines grep returns.
	grepMaxMatches = 200
	// grepMaxFileBytes skips files too large to search usefully.
	grepMaxFileBytes = 512 << 10
	// grepLineBytes caps one reported line.
	grepLineBytes = 400
	// resultMaxBytes caps the text any listing tool returns.
	resultMaxBytes = 64 << 10

	// writeFileMaxBytes caps what write_file and edit_file may write.
	writeFileMaxBytes = 1 << 20
)

// Tools is every tool the harness can offer. Which of them a request gets is
// decided by the chat's mode: Plan keeps the read-only ones, Build adds the
// rest.
func Tools(ws *Workspace) []goai.Tool {
	if ws == nil {
		return nil
	}
	return append(ReadTools(ws),
		createDirTool(ws),
		writeFileTool(ws),
		editFileTool(ws),
		editLinesTool(ws),
		shellTool(ws),
	)
}

// ReadTools is the read-only subset: safe to run without approval, and the
// whole of Plan mode's file access.
func ReadTools(ws *Workspace) []goai.Tool {
	if ws == nil {
		return nil
	}
	return []goai.Tool{
		readFileTool(ws),
		listDirTool(ws),
		globTool(ws),
		grepTool(ws),
	}
}

// ignoredDirs are directories a coding agent has no business walking: they are
// huge, generated, or both.
var ignoredDirs = map[string]bool{
	".git":         true,
	".hg":          true,
	".svn":         true,
	"node_modules": true,
	"dist":         true,
	"build":        true,
	"out":          true,
	".next":        true,
	".svelte-kit":  true,
	"target":       true,
	"vendor":       true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
	".idea":        true,
	".gradle":      true,
	"coverage":     true,
}

// walkFiles visits every file under root, skipping ignored directories and
// unreadable entries. The visitor receives the absolute and workspace-relative
// paths and returns false to stop the walk.
func (w *Workspace) walkFiles(root string, visit func(abs, rel string) bool) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			// An entry we cannot read is skipped rather than failing the walk.
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if path != root && ignoredDirs[strings.ToLower(entry.Name())] {
				return fs.SkipDir
			}
			return nil
		}
		if !visit(path, w.Rel(path)) {
			return fs.SkipAll
		}
		return nil
	})
}

// matchName matches one workspace-relative path against a glob. A pattern with
// no slash matches the file name at any depth, so "*.go" finds Go files
// anywhere; anything else matches the whole path, where "**" spans directories.
func matchName(pattern, rel string) bool {
	pattern = strings.Trim(filepath.ToSlash(pattern), "/")
	if pattern == "" {
		return false
	}
	if !strings.Contains(pattern, "/") {
		ok, err := filepath.Match(pattern, filepath.Base(rel))
		return err == nil && ok
	}
	return matchParts(strings.Split(pattern, "/"), strings.Split(rel, "/"))
}

func matchParts(pattern, name []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			if len(pattern) == 1 {
				return true
			}
			for i := 0; i <= len(name); i++ {
				if matchParts(pattern[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		ok, err := filepath.Match(pattern[0], name[0])
		if err != nil || !ok {
			return false
		}
		pattern, name = pattern[1:], name[1:]
	}
	return len(name) == 0
}

// splitLines normalises line endings so numbering matches what an editor shows.
func splitLines(text string) []string {
	if text == "" {
		return []string{""}
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	return strings.Split(text, "\n")
}

// truncateLine shortens one line on a rune boundary, so the output stays valid
// UTF-8 even for minified files.
func truncateLine(line string, max int) string {
	if len(line) <= max {
		return line
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(line[cut]) {
		cut--
	}
	return line[:cut] + " … [line truncated]"
}

// looksBinary reports whether the start of a file is not text, judged by the
// NUL bytes that binary formats carry and text files do not.
func looksBinary(data []byte) bool {
	head := data
	if len(head) > binarySniffBytes {
		head = head[:binarySniffBytes]
	}
	return strings.IndexByte(string(head), 0) >= 0
}

// humanSize renders a byte count the way a file listing should.
func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return strconv.FormatFloat(float64(n)/(1<<20), 'g', 3, 64) + " MB"
	case n >= 1<<10:
		return strconv.FormatFloat(float64(n)/(1<<10), 'g', 3, 64) + " KB"
	default:
		return strconv.FormatInt(n, 10) + " B"
	}
}
