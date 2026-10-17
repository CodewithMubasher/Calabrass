package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// edit_lines is the tool that has to be simpler than quoting text: the numbers
// come from read_file, and the file's own line endings are not the model's
// problem. What is tested here is exactly that.

func editWorkspace(t *testing.T, name, content string) *Workspace {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}

	ws, err := NewWorkspace(root)
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}
	return ws
}

func editLinesOn(t *testing.T, ws *Workspace, path string, edits ...lineEdit) (string, error) {
	t.Helper()

	raw, err := json.Marshal(editLinesArgs{Path: path, Edits: edits})
	if err != nil {
		t.Fatalf("marshal edits: %v", err)
	}
	return editLinesTool(ws).Execute(context.Background(), raw)
}

func fileText(t *testing.T, ws *Workspace, path string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(ws.Root(), filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestEditLinesReplacesARange(t *testing.T) {
	ws := editWorkspace(t, "app.txt", "one\ntwo\nthree\nfour\nfive\n")

	said, err := editLinesOn(t, ws, "app.txt", lineEdit{Start: 2, End: 4, Text: "TWO\nTHREE"})
	if err != nil {
		t.Fatalf("edit_lines: %v", err)
	}

	if got, want := fileText(t, ws, "app.txt"), "one\nTWO\nTHREE\nfive\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !strings.Contains(said, "lines 2-4 (3 lines) -> 2 lines") {
		t.Errorf("report is %q, want it to say what moved", said)
	}
	if !strings.Contains(said, "shift by -1") {
		t.Errorf("report is %q, want it to say which way the lines below moved", said)
	}
}

func TestEditLinesReplacesOneLine(t *testing.T) {
	ws := editWorkspace(t, "app.txt", "one\ntwo\nthree\n")

	if _, err := editLinesOn(t, ws, "app.txt", lineEdit{Start: 2, End: 2, Text: "TWO"}); err != nil {
		t.Fatalf("edit_lines: %v", err)
	}
	if got, want := fileText(t, ws, "app.txt"), "one\nTWO\nthree\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestEditLinesAppliesEveryEditToTheFileAsItWasRead is the point of taking them
// as a list: the numbers all come from one read, so a second edit does not have
// to allow for what the first one did.
func TestEditLinesAppliesEveryEditToTheFileAsItWasRead(t *testing.T) {
	ws := editWorkspace(t, "app.txt", "a\nb\nc\nd\ne\n")

	_, err := editLinesOn(t, ws, "app.txt",
		lineEdit{Start: 1, End: 1, Text: "A1\nA2"},
		lineEdit{Start: 3, End: 4, Text: "c and d"},
		lineEdit{Start: 5, End: 5, Text: "E"},
	)
	if err != nil {
		t.Fatalf("edit_lines: %v", err)
	}

	if got, want := fileText(t, ws, "app.txt"), "A1\nA2\nb\nc and d\nE\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEditLinesInsertsAndAppends(t *testing.T) {
	ws := editWorkspace(t, "app.txt", "b\nc\n")

	// Start 1 with end 0 is above the first line; an end below start is a pure
	// insert before that line; one past the last line appends.
	_, err := editLinesOn(t, ws, "app.txt",
		lineEdit{Start: 1, End: 0, Text: "first"},
		lineEdit{Start: 2, End: 1, Text: "b-extra"},
		lineEdit{Start: 3, End: 2, Text: "last"},
	)
	if err != nil {
		t.Fatalf("edit_lines: %v", err)
	}

	if got, want := fileText(t, ws, "app.txt"), "first\nb\nb-extra\nc\nlast\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEditLinesDeletesWithEmptyText(t *testing.T) {
	ws := editWorkspace(t, "app.txt", "a\nb\nc\nd\n")

	said, err := editLinesOn(t, ws, "app.txt", lineEdit{Start: 2, End: 3})
	if err != nil {
		t.Fatalf("edit_lines: %v", err)
	}

	if got, want := fileText(t, ws, "app.txt"), "a\nd\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !strings.Contains(said, "deleted lines 2-3") {
		t.Errorf("report is %q, want it to say what was deleted", said)
	}
}

// TestEditLinesWritesWithTheFilesOwnLineEndings is the bug this design exists to
// avoid: read_file shows a CRLF file without its \r, so text copied from a read
// can only be matched against lines that were treated the same way.
func TestEditLinesWritesWithTheFilesOwnLineEndings(t *testing.T) {
	ws := editWorkspace(t, "app.txt", "one\r\ntwo\r\nthree\r\n")

	if _, err := editLinesOn(t, ws, "app.txt", lineEdit{Start: 2, End: 2, Text: "TWO"}); err != nil {
		t.Fatalf("edit_lines: %v", err)
	}
	if got, want := fileText(t, ws, "app.txt"), "one\r\nTWO\r\nthree\r\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEditLinesDoesNotInventATrailingNewline(t *testing.T) {
	ws := editWorkspace(t, "app.txt", "one\ntwo")

	if _, err := editLinesOn(t, ws, "app.txt", lineEdit{Start: 2, End: 2, Text: "TWO"}); err != nil {
		t.Fatalf("edit_lines: %v", err)
	}
	if got, want := fileText(t, ws, "app.txt"), "one\nTWO"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEditLinesKeepsAByteOrderMarkOffTheFirstLine(t *testing.T) {
	ws := editWorkspace(t, "app.txt", "\ufeffone\ntwo\n")

	if _, err := editLinesOn(t, ws, "app.txt", lineEdit{Start: 1, End: 1, Expect: "one", Text: "ONE"}); err != nil {
		t.Fatalf("edit_lines: %v", err)
	}
	if got, want := fileText(t, ws, "app.txt"), "\ufeffONE\ntwo\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEditLinesRefusesAnEditWhoseAnchorLineHasMoved(t *testing.T) {
	ws := editWorkspace(t, "app.txt", "one\ntwo\nthree\n")

	_, err := editLinesOn(t, ws, "app.txt",
		lineEdit{Start: 2, End: 2, Text: "TWO", Expect: "the line I thought was here"})
	if err == nil {
		t.Fatal("an edit whose anchor line has changed should be refused")
	}
	if !strings.Contains(err.Error(), "read the file again") {
		t.Errorf("error is %v, want it to say what to do next", err)
	}
	if got, want := fileText(t, ws, "app.txt"), "one\ntwo\nthree\n"; got != want {
		t.Errorf("a refused call changed the file: %q", got)
	}
}

func TestEditLinesAcceptsAnAnchorCopiedWithItsLineNumber(t *testing.T) {
	ws := editWorkspace(t, "app.txt", "one\ntwo\nthree\n")

	// The file shows line 2 as "    2| two", and pasting that is easy to do.
	if _, err := editLinesOn(t, ws, "app.txt", lineEdit{Start: 2, End: 2, Text: "TWO", Expect: "    2| two"}); err != nil {
		t.Fatalf("a line copied with its number should still anchor: %v", err)
	}
	if got, want := fileText(t, ws, "app.txt"), "one\nTWO\nthree\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEditLinesRefusesWhatItCannotDo(t *testing.T) {
	ws := editWorkspace(t, "app.txt", "one\ntwo\nthree\n")

	cases := []struct {
		name  string
		edits []lineEdit
	}{
		{"a start past the end", []lineEdit{{Start: 9, End: 9, Text: "x"}}},
		{"an end past the end", []lineEdit{{Start: 1, End: 9, Text: "x"}}},
		{"a start of zero", []lineEdit{{Start: 0, End: 0, Text: "x"}}},
		{"an end that never arrived", []lineEdit{{Start: 3, End: 0, Text: "x"}}},
		{"changes that change nothing", []lineEdit{{Start: 2, End: 2, Text: "two"}}},
		{"a line covered twice", []lineEdit{
			{Start: 1, End: 2, Text: "x"},
			{Start: 2, End: 3, Text: "y"},
		}},
		{"a file that does not exist", []lineEdit{{Start: 1, End: 1, Text: "x"}}},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			path := "app.txt"
			if test.name == "a file that does not exist" {
				path = "missing.txt"
			}
			if _, err := editLinesOn(t, ws, path, test.edits...); err == nil {
				t.Errorf("%v should have been refused", test.edits)
			}
		})
	}

	if got, want := fileText(t, ws, "app.txt"), "one\ntwo\nthree\n"; got != want {
		t.Errorf("a refused call changed the file: %q", got)
	}
}

// TestSplittingAFileAndJoiningItBackChangesNothing is what makes the edits above
// trustworthy: everything the lines are not is remembered separately.
func TestSplittingAFileAndJoiningItBackChangesNothing(t *testing.T) {
	for _, text := range []string{
		"",
		"\n",
		"a",
		"a\n",
		"a\nb",
		"a\nb\n",
		"a\r\nb\r\n",
		"\ufeffa\n",
		"a\n\n\nb\n",
		"only\rcarriage\n",
	} {
		if got := splitFile(text).join(); got != text {
			t.Errorf("splitFile(%q).join() = %q, want it unchanged", text, got)
		}
	}
}

// TestAMixedFileIsWrittenBackWithTheEndingItUsesMost: a file that disagrees with
// itself has to end up as something, and the majority is the least surprising.
func TestAMixedFileIsWrittenBackWithTheEndingItUsesMost(t *testing.T) {
	if got, want := splitFile("a\r\nb\r\nc\n").join(), "a\r\nb\r\nc\r\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
