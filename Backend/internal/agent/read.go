package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zendev-sh/goai"
)

// --- read_file ---------------------------------------------------------------

// readFileArgs is the wire shape of read_file: the JSON Schema is generated
// from these tags.
type readFileArgs struct {
	Path   string `json:"path" jsonschema:"description=Path to the file, relative to the project root"`
	Offset int    `json:"offset,omitempty" jsonschema:"description=1-based line number to start from; defaults to the first line"`
	Limit  int    `json:"limit,omitempty" jsonschema:"description=How many lines to return; defaults to 400"`
}

func readFileTool(ws *Workspace) goai.Tool {
	return goai.NewTool("read_file",
		"Read a text file from the project. Returns the lines, numbered for later reference. Use offset and limit to page through a long file instead of reading it again from the top. Read-only and safe to run in parallel.",
		func(ctx context.Context, args readFileArgs) (string, error) {
			return ws.readFile(args)
		})
}

// readFile returns numbered lines of one file, capped so a single call cannot
// flood the model's context. The first line carries the file's size and the
// window that was returned, so the model knows whether there is more.
func (w *Workspace) readFile(args readFileArgs) (string, error) {
	abs, err := w.Resolve(args.Path)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no such file: %s", w.Rel(abs))
		}
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory, not a file; use list_dir", w.Rel(abs))
	}

	limit := args.Limit
	if limit <= 0 {
		limit = readFileLines
	}
	from := args.Offset - 1
	if from < 0 {
		from = 0
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	if looksBinary(data) {
		return "", fmt.Errorf("%s looks like a binary file (%d bytes)", w.Rel(abs), len(data))
	}

	lines := splitLines(string(data))
	total := len(lines)
	if from > total {
		from = total
	}
	to := from + limit
	if to > total {
		to = total
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s (%d lines total", w.Rel(abs), total)
	if from > 0 || to < total {
		fmt.Fprintf(&b, "; showing %d-%d", from+1, to)
	}
	b.WriteString(")\n")

	written := 0
	for i := from; i < to; i++ {
		line := truncateLine(lines[i], readFileLineBytes)
		fmt.Fprintf(&b, "%5d| %s\n", i+1, line)
		written += len(line) + 8
		if written >= readFileMaxBytes {
			fmt.Fprintf(&b, "… [stopped at %d KB; continue with offset=%d, or search with grep]\n", readFileMaxBytes>>10, i+2)
			break
		}
	}
	return b.String(), nil
}

// --- list_dir ----------------------------------------------------------------

type listDirArgs struct {
	Path string `json:"path,omitempty" jsonschema:"description=Directory to list, relative to the project root; defaults to the project root"`
}

func listDirTool(ws *Workspace) goai.Tool {
	return goai.NewTool("list_dir",
		"List the entries of a directory in the project: subdirectories first, then files with their sizes. Read-only and safe to run in parallel.",
		func(ctx context.Context, args listDirArgs) (string, error) {
			return ws.listDir(args.Path)
		})
}

func (w *Workspace) listDir(path string) (string, error) {
	abs, err := w.Resolve(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no such directory: %s", w.Rel(abs))
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is a file, not a directory; use read_file", w.Rel(abs))
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", err
	}

	// ReadDir sorts by name, so each group stays alphabetical.
	var dirs, files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			if name == ".git" {
				continue
			}
			dirs = append(dirs, name+"/")
			continue
		}
		size := int64(0)
		if info, err := entry.Info(); err == nil {
			size = info.Size()
		}
		files = append(files, fmt.Sprintf("%-9s %s", humanSize(size), name))
	}

	shown := len(dirs) + len(files)
	truncated := false
	if shown > listDirEntries {
		shown = listDirEntries
		truncated = true
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s/ (%d entries)\n", w.Rel(abs), len(dirs)+len(files))
	for i, line := range append(dirs, files...) {
		if i >= shown {
			break
		}
		fmt.Fprintf(&b, "  %s\n", line)
	}
	if truncated {
		fmt.Fprintf(&b, "… [showing %d of %d entries]\n", shown, len(dirs)+len(files))
	}
	return b.String(), nil
}

// --- glob --------------------------------------------------------------------

type globArgs struct {
	Pattern string `json:"pattern" jsonschema:"description=Glob to match against project paths, e.g. \"src/**/*.tsx\", or \"*.go\" to match by file name at any depth"`
	Limit   int    `json:"limit,omitempty" jsonschema:"description=Maximum number of paths to return; defaults to 200"`
}

func globTool(ws *Workspace) goai.Tool {
	return goai.NewTool("glob",
		"Find files in the project by glob. Returns matching paths relative to the project root, newest matches last. Read-only and safe to run in parallel; prefer a narrow pattern over the whole tree.",
		func(ctx context.Context, args globArgs) (string, error) {
			return ws.glob(args.Pattern, args.Limit)
		})
}

func (w *Workspace) glob(pattern string, limit int) (string, error) {
	pattern = strings.TrimSpace(filepath.ToSlash(pattern))
	if pattern == "" {
		return "", errors.New("pattern is required")
	}
	if limit <= 0 || limit > globMaxPaths {
		limit = globMaxPaths
	}

	// A literal leading directory narrows the walk; patterns alone start at the
	// project root.
	root := w.root
	if head, _, ok := strings.Cut(pattern, "/"); ok && head != "" && !strings.ContainsAny(head, "*?[") {
		root = filepath.Join(w.root, head)
	}

	matches := []string{}
	_ = w.walkFiles(root, func(abs, rel string) bool {
		if matchName(pattern, rel) {
			matches = append(matches, rel)
			if len(matches) >= limit {
				return false
			}
		}
		return true
	})

	var b strings.Builder
	fmt.Fprintf(&b, "%d match(es) for %s\n", len(matches), pattern)
	b.WriteString(strings.Join(matches, "\n"))
	if len(matches) > 0 {
		b.WriteString("\n")
	}
	if len(matches) >= limit {
		fmt.Fprintf(&b, "… [capped at %d paths; narrow the pattern]\n", limit)
	}
	return b.String(), nil
}

// --- grep --------------------------------------------------------------------

type grepArgs struct {
	Pattern    string `json:"pattern" jsonschema:"description=Regular expression to search for"`
	Path       string `json:"path,omitempty" jsonschema:"description=File or directory to search; defaults to the project root"`
	Include    string `json:"include,omitempty" jsonschema:"description=Only search files matching this glob, e.g. \"*.go\""`
	MaxResults int    `json:"maxResults,omitempty" jsonschema:"description=Maximum number of matching lines to return; defaults to 200"`
}

func grepTool(ws *Workspace) goai.Tool {
	return goai.NewTool("grep",
		"Search file contents with a regular expression. Returns file:line: text for each match, so it is the fastest way to find where something is defined or used. Read-only and safe to run in parallel; use include to keep the search narrow.",
		func(ctx context.Context, args grepArgs) (string, error) {
			return ws.grep(args)
		})
}

func (w *Workspace) grep(args grepArgs) (string, error) {
	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return "", fmt.Errorf("bad pattern: %w", err)
	}

	root := w.root
	if path := strings.TrimSpace(args.Path); path != "" {
		if root, err = w.Resolve(path); err != nil {
			return "", err
		}
	}
	max := args.MaxResults
	if max <= 0 || max > grepMaxMatches {
		max = grepMaxMatches
	}

	var (
		b       strings.Builder
		matches int
		scanned int
	)
	_ = w.walkFiles(root, func(abs, rel string) bool {
		if args.Include != "" && !matchName(args.Include, rel) {
			return true
		}
		data, err := os.ReadFile(abs)
		if err != nil || len(data) > grepMaxFileBytes || looksBinary(data) {
			return true
		}
		scanned++
		for i, line := range splitLines(string(data)) {
			if !re.MatchString(line) {
				continue
			}
			matches++
			fmt.Fprintf(&b, "%s:%d: %s\n", rel, i+1, truncateLine(strings.TrimRight(line, " \t"), grepLineBytes))
			if matches >= max || b.Len() >= resultMaxBytes {
				return false
			}
		}
		return true
	})

	var head strings.Builder
	fmt.Fprintf(&head, "%d match(es) for %s in %d file(s)\n", matches, args.Pattern, scanned)
	if matches == 0 {
		head.WriteString("no matches\n")
	}
	head.WriteString(b.String())
	if matches >= max {
		fmt.Fprintf(&head, "… [capped at %d matches; narrow the pattern]\n", max)
	}
	return head.String(), nil
}
