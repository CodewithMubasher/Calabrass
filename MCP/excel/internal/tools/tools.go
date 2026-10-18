// Package tools is what this server offers the model: the ways to make a
// spreadsheet and the ways to change one. Each tool reads as the promise, and
// the workbook package as the mechanism.
package tools

import (
	"fmt"
	"strings"

	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/protocol"
	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/workbook"
)

// maxPath is as long as a path this server will act on may be.
const maxPath = 400

// Limits on a single write. They are the point past which a call is more likely
// a mistake than an intention — a list that arrived as a whole spreadsheet, or a
// row that arrived as a whole table.
const (
	maxWriteRows = 5000
	maxWriteCols = 200
)

// session holds what every tool shares: the folder paths are resolved against,
// and the lock that keeps two calls off the same file.
type session struct {
	root  *workbook.Root
	guard workbook.Guard
}

// All is every tool this server has, in the order they are listed. The order is
// the order someone would work in: make a workbook, look at it, then change it.
func All(root *workbook.Root) []protocol.Tool {
	s := &session{root: root}
	return []protocol.Tool{
		s.createWorkbook(),
		s.listWorkbooks(),
		s.workbookInfo(),
		s.addSheet(),
		s.renameSheet(),
		s.deleteSheet(),
		s.copySheet(),
		s.setSheetVisibility(),
		s.readRange(),
		s.writeRange(),
		s.appendRows(),
		s.writeTable(),
		s.clearRange(),
		s.formatRange(),
		s.setColumnWidth(),
		s.setRowHeight(),
		s.mergeCells(),
		s.freezePanes(),
		s.setAutoFilter(),
		s.conditionalFormat(),
		s.dataValidation(),
		s.insertRows(),
		s.deleteRows(),
		s.insertColumns(),
		s.deleteColumns(),
		s.addChart(),
		s.searchCells(),
		s.exportCSV(),
	}
}

// Instructions is what the client shows the model beside the tools. It is where
// the folder every path lands in is stated, because a model that does not know
// where a file will appear cannot tell the person it is helping.
func Instructions(root *workbook.Root) string {
	note := "These tools make and change Microsoft Excel workbooks (.xlsx) on this computer. " +
		"Every change is written to the file immediately, so the workbook is on disk after each call. " +
		fmt.Sprintf("A relative path is taken from %s; ", root.Dir())
	if root.AllowOutside() {
		note += "an absolute path is used as given. "
	} else {
		note += "an absolute path must stay inside that folder. "
	}
	return note + "Read a sheet with read_range before changing it, and use create_workbook for a " +
		"workbook that does not exist yet."
}

// path reads the workbook path a tool was given and turns it into the file to
// act on.
func (s *session) path(args map[string]any) (string, error) {
	raw, err := protocol.RequiredString(args, "path", maxPath)
	if err != nil {
		return "", err
	}
	return s.root.ResolveWorkbook(raw)
}

// read opens a workbook, reads from it, and closes it.
func (s *session) read(path string, work func(b *workbook.Book) (string, error)) (string, error) {
	return s.guard.Do(func() (string, error) {
		book, err := workbook.Open(path)
		if err != nil {
			return "", err
		}
		defer func() { _ = book.Close() }()
		return work(book)
	})
}

// edit opens a workbook, changes it under the guard, and saves it — which is the
// moment the change becomes real, and the only place a save failure is reported.
func (s *session) edit(path string, work func(b *workbook.Book) (string, error)) (string, error) {
	return s.guard.Do(func() (string, error) {
		book, err := workbook.Open(path)
		if err != nil {
			return "", err
		}
		defer func() { _ = book.Close() }()

		said, err := work(book)
		if err != nil {
			return "", err
		}
		if err := book.Save(); err != nil {
			return "", err
		}
		return said + savedNote(s.root, book), nil
	})
}

// make creates a workbook, fills it, and saves it. Creating is kept apart from
// editing because it is the one call that may stand on a file that is already
// there.
func (s *session) make(path string, sheets []string, overwrite bool, work func(b *workbook.Book) (string, error)) (string, error) {
	return s.guard.Do(func() (string, error) {
		book, err := workbook.Create(path, sheets, overwrite)
		if err != nil {
			return "", err
		}
		defer func() { _ = book.Close() }()

		said, err := work(book)
		if err != nil {
			return "", err
		}
		if err := book.Save(); err != nil {
			return "", err
		}
		return said + savedNote(s.root, book), nil
	})
}

func savedNote(root *workbook.Root, book *workbook.Book) string {
	return "\nSaved: " + root.Relative(book.Path())
}

// sheet reads the sheet a call named, or the workbook's active sheet.
func (s *session) sheet(book *workbook.Book, args map[string]any) (string, error) {
	named, err := protocol.OptionalString(args, "sheet", "", 100)
	if err != nil {
		return "", err
	}
	return book.Sheet(named)
}

// sheetsLine names the sheets a workbook has, for a message that has to say what
// is there.
func sheetsLine(book *workbook.Book) string {
	names := book.File().GetSheetList()
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, fmt.Sprintf("%q", name))
	}
	return strings.Join(quoted, ", ")
}
