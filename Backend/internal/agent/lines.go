package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/zendev-sh/goai"
)

// --- the shape of a file on disk ---------------------------------------------

// fileBody is a file taken apart into the pieces an edit has to put back: its
// lines without their line endings, the ending they used, and the two bytes of
// business that a naive split-and-join quietly destroys — a byte-order mark in
// front, and whether the file ended with a line ending at all.
//
// Keeping the endings out of the lines is what makes line-addressed editing work
// at all: read_file shows a CRLF file with its \r stripped, so text copied from a
// read can only be matched, or written back, against lines that have had the same
// treatment.
type fileBody struct {
	lines   []string
	eol     string
	bom     bool
	newline bool
}

func splitFile(text string) fileBody {
	body := fileBody{eol: "\n"}

	if strings.HasPrefix(text, "\ufeff") {
		body.bom = true
		text = strings.TrimPrefix(text, "\ufeff")
	}

	// Whichever ending the file uses most is the one it is written back with, so
	// adding a line cannot turn a CRLF file into a mixed one.
	crlf := strings.Count(text, "\r\n")
	if lf := strings.Count(text, "\n") - crlf; crlf > lf {
		body.eol = "\r\n"
	}

	body.newline = strings.HasSuffix(text, "\n")
	body.lines = splitLines(text)
	return body
}

// join is the file as bytes again. A file that ended without a line ending keeps
// ending without one: adding a trailing newline is a change nobody asked for.
func (b fileBody) join() string {
	text := strings.Join(b.lines, b.eol)
	if b.newline && len(b.lines) > 0 {
		text += b.eol
	}
	if b.bom {
		text = "\ufeff" + text
	}
	return text
}

// --- edit_lines --------------------------------------------------------------

// maxLineEdits caps how many changes one call may carry, so a runaway model
// cannot build a list longer than the file it is editing.
const maxLineEdits = 200

// lineEdit is one change: the lines to replace, and what goes there instead.
//
// Every field is required by the schema the SDK generates, so end is always
// sent: there is no "left out" to interpret, and the rules can be read straight
// off the numbers. Text comes through the same tag parser that splits on commas,
// hence the semicolons in the descriptions.
type lineEdit struct {
	Start  int    `json:"start" jsonschema:"description=First line the edit touches; use the numbers read_file printed; one past the last line appends"`
	End    int    `json:"end,omitempty" jsonschema:"description=Last line the edit touches; repeat start to change one line; an end lower than start inserts the text before start instead"`
	Text   string `json:"text" jsonschema:"description=The lines to put in their place; empty deletes the lines the edit covers"`
	Expect string `json:"expect,omitempty" jsonschema:"description=The beginning of the text on line start; the edit is refused when that line has changed since you read it"`
}

type editLinesArgs struct {
	Path  string     `json:"path" jsonschema:"description=Path of the file to change; relative to the project root"`
	Edits []lineEdit `json:"edits" jsonschema:"description=Every change to make in one call; they all describe the file as you read it and are written together"`
}

func editLinesTool(ws *Workspace) goai.Tool {
	return goai.NewTool("edit_lines",
		"Change an existing file line by line, using the numbers read_file prints. "+
			"Each edit replaces lines start..end with text: repeat start as the end to change one line, and make the end lower than the start to insert before it. "+
			"Every edit in the call describes the file as you read it and they are written together, so they cannot disturb each other's numbers. "+
			"Pass expect, the beginning of the text on line start, and an edit that would land somewhere else is refused instead. "+
			"Read the file first. Changes state; do not run two edits to the same file in parallel.",
		func(ctx context.Context, args editLinesArgs) (string, error) {
			return ws.editLines(args)
		})
}

// editLines replaces whole line ranges in one pass and one write.
//
// Line numbers are only worth trusting against the file the model actually read,
// so each expect is checked against the file on disk before anything is applied:
// every edit in a call describes the same original file, and a call that fails
// changes nothing.
func (w *Workspace) editLines(args editLinesArgs) (string, error) {
	abs, err := w.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	switch {
	case len(args.Edits) == 0:
		return "", errors.New("no edits to make")
	case len(args.Edits) > maxLineEdits:
		return "", fmt.Errorf("%d edits in one call, over the limit of %d", len(args.Edits), maxLineEdits)
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

	original := string(data)
	body := splitFile(original)
	total := len(body.lines)

	// Sorted so the file can be walked once, with each edit's lines named the way
	// the model named them: what moves underneath is tracked separately.
	edits := make([]lineEdit, len(args.Edits))
	copy(edits, args.Edits)
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].Start < edits[j].Start })

	type step struct {
		edit   lineEdit
		from   int // first line replaced, 0-based
		to     int // one past the last line replaced
		insert []string
	}

	plan := make([]step, 0, len(edits))
	lastTo := 0
	for _, edit := range edits {
		from, to, err := lineRange(edit, total)
		if err != nil {
			return "", err
		}
		if from < lastTo {
			return "", fmt.Errorf("line %d is covered by two edits; give each line to one of them", from+1)
		}
		if from < total {
			if err := checkLineAnchor(edit, body.lines[from]); err != nil {
				return "", err
			}
		}
		lastTo = to
		plan = append(plan, step{edit: edit, from: from, to: to, insert: textLines(edit.Text)})
	}

	lines := body.lines
	notes := make([]string, 0, len(plan))
	shift := 0
	for _, step := range plan {
		at := step.from + shift
		lines = splice(lines, at, at+(step.to-step.from), step.insert)
		notes = append(notes, describeChange(step.edit, step.to-step.from, len(step.insert)))
		shift += len(step.insert) - (step.to - step.from)
	}
	body.lines = lines

	updated := body.join()
	switch {
	case updated == original:
		return "", errors.New("the edits leave the file unchanged")
	case len(updated) > writeFileMaxBytes:
		return "", fmt.Errorf("the result would be %s, over the %d KB limit", humanSize(int64(len(updated))), writeFileMaxBytes>>10)
	}
	if err := os.WriteFile(abs, []byte(updated), 0o644); err != nil {
		return "", err
	}

	return fmt.Sprintf("edited %s: %d edit(s), now %d lines\n- %s",
		w.Rel(abs), len(plan), len(lines), strings.Join(notes, "\n- ")), nil
}

// lineRange turns the numbers the model gave into a half-open range of the
// file's own lines, [from, to). An insert is the empty range before a line.
func lineRange(edit lineEdit, total int) (from, to int, err error) {
	switch {
	case edit.Start < 1:
		return 0, 0, fmt.Errorf("start must be 1 or more, not %d; to add lines above the first one use start 1 with end 0", edit.Start)
	case edit.Start > total+1:
		return 0, 0, fmt.Errorf("start is line %d, but the file has %d lines", edit.Start, total)
	}

	if edit.End < edit.Start {
		// Nothing is replaced: the text goes in before the line. For any line
		// but the first, one less than start is the spelling, so a 0 is a field
		// that never arrived rather than an insert.
		if edit.End == 0 && edit.Start > 1 {
			return 0, 0, fmt.Errorf("end is 0 with start %d; use end %d to insert before that line, or end %d to change it",
				edit.Start, edit.Start-1, edit.Start)
		}
		return edit.Start - 1, edit.Start - 1, nil
	}
	if edit.End > total {
		return 0, 0, fmt.Errorf("end is line %d, but the file has %d lines", edit.End, total)
	}

	return edit.Start - 1, edit.End, nil
}

// checkLineAnchor refuses an edit whose anchor line is not the one the model
// read. It compares the beginning of the line, trimmed: a long line is shown
// truncated, and trailing space is not worth failing an edit over.
func checkLineAnchor(edit lineEdit, actual string) error {
	want := strings.TrimSpace(stripLineNumber(edit.Expect))
	if want == "" {
		return nil
	}
	if strings.HasPrefix(strings.TrimSpace(actual), want) {
		return nil
	}
	return fmt.Errorf("line %d is %q, not %q; read the file again before changing it",
		edit.Start, truncateLine(strings.TrimSpace(actual), 80), want)
}

// stripLineNumber drops the "   12| " prefix when the model copies a line out of
// read_file along with its number. It is an easy thing to do and not worth
// refusing an edit over.
func stripLineNumber(text string) string {
	trimmed := strings.TrimLeft(text, " \t")

	digits := 0
	for digits < len(trimmed) && trimmed[digits] >= '0' && trimmed[digits] <= '9' {
		digits++
	}
	if digits == 0 || digits >= len(trimmed) || trimmed[digits] != '|' {
		return text
	}
	return strings.TrimLeft(trimmed[digits+1:], " \t")
}

// textLines is the replacement as lines. Empty text replaces nothing, which is
// how a range is deleted.
func textLines(text string) []string {
	if text == "" {
		return nil
	}
	return splitLines(text)
}

// splice replaces lines[from:to] with insert.
func splice(lines []string, from, to int, insert []string) []string {
	out := make([]string, 0, len(lines)-(to-from)+len(insert))
	out = append(out, lines[:from]...)
	out = append(out, insert...)
	return append(out, lines[to:]...)
}

// describeChange is one line of the report: what happened to which lines, and
// which way everything below them moved.
func describeChange(edit lineEdit, replaced, inserted int) string {
	var note string
	switch {
	case replaced == 0:
		note = fmt.Sprintf("inserted %d line(s) before line %d", inserted, edit.Start)
	case inserted == 0:
		note = fmt.Sprintf("deleted lines %d-%d (%d lines)", edit.Start, edit.Start+replaced-1, replaced)
	default:
		note = fmt.Sprintf("lines %d-%d (%d lines) -> %d lines", edit.Start, edit.Start+replaced-1, replaced, inserted)
	}

	if delta := inserted - replaced; delta != 0 {
		note += fmt.Sprintf("; lines below shift by %+d", delta)
	}
	return note
}
