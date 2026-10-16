package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zendev-sh/goai"
)

// --- create_dir --------------------------------------------------------------

type createDirArgs struct {
	Path string `json:"path" jsonschema:"description=Directory to create, relative to the project root; parent directories are created too"`
}

func createDirTool(ws *Workspace) goai.Tool {
	return goai.NewTool("create_dir",
		"Create a directory in the project, including any missing parents. Does nothing when it already exists.",
		func(ctx context.Context, args createDirArgs) (string, error) {
			return ws.createDir(args.Path)
		})
}

func (w *Workspace) createDir(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "." {
		return "", errors.New("path is required")
	}
	abs, err := w.Resolve(path)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(abs); err == nil {
		if info.IsDir() {
			return fmt.Sprintf("%s already exists", w.Rel(abs)), nil
		}
		return "", fmt.Errorf("%s is a file, not a directory", w.Rel(abs))
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", err
	}
	return fmt.Sprintf("created %s/", w.Rel(abs)), nil
}

// --- write_file --------------------------------------------------------------

type writeFileArgs struct {
	Path    string `json:"path" jsonschema:"description=Path to write, relative to the project root"`
	Content string `json:"content" jsonschema:"description=The complete contents of the file"`
}

func writeFileTool(ws *Workspace) goai.Tool {
	return goai.NewTool("write_file",
		"Write a whole file, creating it and any missing parent directories. Existing contents are replaced, so use edit_file for small changes to a file you have read.",
		func(ctx context.Context, args writeFileArgs) (string, error) {
			return ws.writeFile(args.Path, args.Content)
		})
}

func (w *Workspace) writeFile(path, content string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "." {
		return "", errors.New("path is required")
	}
	if len(content) > writeFileMaxBytes {
		return "", fmt.Errorf("content is %s, over the %d KB limit", humanSize(int64(len(content))), writeFileMaxBytes>>10)
	}
	abs, err := w.Resolve(path)
	if err != nil {
		return "", err
	}

	verb := "created"
	if info, err := os.Stat(abs); err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("%s is a directory, not a file", w.Rel(abs))
		}
		verb = "overwrote"
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s %s (%s, %d lines)", verb, w.Rel(abs), humanSize(int64(len(content))), lineCount(content)), nil
}

// --- edit_file ---------------------------------------------------------------

type editFileArgs struct {
	Path       string `json:"path" jsonschema:"description=Path of the file to change, relative to the project root"`
	OldString  string `json:"oldString" jsonschema:"description=The exact text to replace, including its indentation. Copy it from the file, without any line-number prefix."`
	NewString  string `json:"newString" jsonschema:"description=The text to put in its place"`
	ReplaceAll bool   `json:"replaceAll,omitempty" jsonschema:"description=Replace every occurrence instead of requiring a single unique match"`
}

func editFileTool(ws *Workspace) goai.Tool {
	return goai.NewTool("edit_file",
		"Change part of an existing file by replacing an exact piece of text. The text must match exactly once unless replaceAll is set, so include enough surrounding lines to be unambiguous.",
		func(ctx context.Context, args editFileArgs) (string, error) {
			return ws.editFile(args)
		})
}

func (w *Workspace) editFile(args editFileArgs) (string, error) {
	abs, err := w.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(args.OldString) == "" {
		return "", errors.New("oldString is required")
	}
	if args.OldString == args.NewString {
		return "", errors.New("oldString and newString are identical, so nothing would change")
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no such file: %s; use write_file to create it", w.Rel(abs))
		}
		return "", err
	}
	if looksBinary(data) {
		return "", fmt.Errorf("%s looks like a binary file", w.Rel(abs))
	}
	text := string(data)

	count := strings.Count(text, args.OldString)
	switch {
	case count == 0:
		return "", fmt.Errorf("oldString is not in %s; read the file again and copy the text exactly", w.Rel(abs))
	case count > 1 && !args.ReplaceAll:
		return "", fmt.Errorf("oldString appears %d times in %s; add more surrounding lines or set replaceAll", count, w.Rel(abs))
	}

	replaced := 1
	updated := strings.Replace(text, args.OldString, args.NewString, 1)
	if args.ReplaceAll {
		replaced = count
		updated = strings.ReplaceAll(text, args.OldString, args.NewString)
	}
	if len(updated) > writeFileMaxBytes {
		return "", fmt.Errorf("the result would be %s, over the %d KB limit", humanSize(int64(len(updated))), writeFileMaxBytes>>10)
	}
	if err := os.WriteFile(abs, []byte(updated), 0o644); err != nil {
		return "", err
	}

	return fmt.Sprintf("edited %s: replaced %d occurrence(s) at line %d", w.Rel(abs), replaced, lineOf(text, args.OldString)), nil
}

// lineCount counts the lines in a body of text.
func lineCount(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

// lineOf reports the 1-based line where needle first appears.
func lineOf(haystack, needle string) int {
	at := strings.Index(haystack, needle)
	if at < 0 {
		return 0
	}
	return strings.Count(haystack[:at], "\n") + 1
}
