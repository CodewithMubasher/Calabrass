package tools

import (
	"context"
	"fmt"

	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/protocol"
	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/workbook"
)

// The tools that make room in a sheet, or take it away. They are four small
// tools rather than one with an action, because "insert a row" and "delete a
// row" are asked for as different things and a model should not have to
// remember which word goes with which.

func (s *session) insertRows() protocol.Tool {
	return protocol.Tool{
		Name: "insert_rows",
		Description: "Push rows down to make room, leaving everything from the row below " +
			"onwards untouched. Nothing moves on other sheets. Formulas pointing at the " +
			"cells below are not rewritten, so check them afterwards if the sheet is full " +
			"of references.",
		Schema: protocol.Object(map[string]any{
			"path":  protocol.String("The workbook to change."),
			"sheet": protocol.String("The sheet to change. Leave out for the active sheet."),
			"row":   protocol.Integer("Insert before this row number.", 1, 1048576),
			"count": protocol.Integer("How many rows to insert. One unless set.", 1, 1000),
		}, "path", "row"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			row, err := protocol.RequiredInt(args, "row", 1, 1048576)
			if err != nil {
				return "", err
			}
			count, err := protocol.OptionalInt(args, "count", 1, 1, 1000)
			if err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}
				if err := book.File().InsertRows(sheet, row, count); err != nil {
					return "", fmt.Errorf("insert rows into %s: %w", sheet, err)
				}
				return fmt.Sprintf("Inserted %d row%s into %s before row %d.", count, plural(count), sheet, row), nil
			})
		},
	}
}

func (s *session) deleteRows() protocol.Tool {
	return protocol.Tool{
		Name: "delete_rows",
		Description: "Remove rows and pull the rows below up to fill the gap, so the rest " +
			"of the sheet moves. Use this to take out blank spacer rows or a bad import; use " +
			"clear_range to empty rows without moving anything.",
		Schema: protocol.Object(map[string]any{
			"path":  protocol.String("The workbook to change."),
			"sheet": protocol.String("The sheet to change. Leave out for the active sheet."),
			"row":   protocol.Integer("The first row to remove.", 1, 1048576),
			"count": protocol.Integer("How many rows to remove. One unless set.", 1, 1000),
		}, "path", "row"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			row, err := protocol.RequiredInt(args, "row", 1, 1048576)
			if err != nil {
				return "", err
			}
			count, err := protocol.OptionalInt(args, "count", 1, 1, 1000)
			if err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}
				for i := 0; i < count; i++ {
					if err := book.File().RemoveRow(sheet, row); err != nil {
						return "", fmt.Errorf("remove row %d from %s: %w", row, sheet, err)
					}
				}
				book.Recount(sheet)
				return fmt.Sprintf("Removed %d row%s from %s, starting at row %d.", count, plural(count), sheet, row), nil
			})
		},
	}
}

func (s *session) insertColumns() protocol.Tool {
	return protocol.Tool{
		Name: "insert_columns",
		Description: "Push columns right to make room, so a column can be added in the " +
			"middle of a table without rebuilding it.",
		Schema: protocol.Object(map[string]any{
			"path":   protocol.String("The workbook to change."),
			"sheet":  protocol.String("The sheet to change. Leave out for the active sheet."),
			"column": protocol.String("Insert before this column letter, such as C."),
			"count":  protocol.Integer("How many columns to insert. One unless set.", 1, 100),
		}, "path", "column"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			column, err := protocol.RequiredString(args, "column", 10)
			if err != nil {
				return "", err
			}
			if _, err := workbook.ColumnNumber(column); err != nil {
				return "", err
			}
			count, err := protocol.OptionalInt(args, "count", 1, 1, 100)
			if err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}
				if err := book.File().InsertCols(sheet, column, count); err != nil {
					return "", fmt.Errorf("insert columns into %s: %w", sheet, err)
				}
				return fmt.Sprintf("Inserted %d column%s into %s before column %s.", count, plural(count), sheet, column), nil
			})
		},
	}
}

func (s *session) deleteColumns() protocol.Tool {
	return protocol.Tool{
		Name: "delete_columns",
		Description: "Remove columns and pull the columns to the right leftwards to fill " +
			"the gap. Deleting a column changes the letters of every column after it, so read " +
			"the sheet again before writing to it by letter.",
		Schema: protocol.Object(map[string]any{
			"path":   protocol.String("The workbook to change."),
			"sheet":  protocol.String("The sheet to change. Leave out for the active sheet."),
			"column": protocol.String("The first column to remove, such as C."),
			"count":  protocol.Integer("How many columns to remove. One unless set.", 1, 100),
		}, "path", "column"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			column, err := protocol.RequiredString(args, "column", 10)
			if err != nil {
				return "", err
			}
			if _, err := workbook.ColumnNumber(column); err != nil {
				return "", err
			}
			count, err := protocol.OptionalInt(args, "count", 1, 1, 100)
			if err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}
				for i := 0; i < count; i++ {
					if err := book.File().RemoveCol(sheet, column); err != nil {
						return "", fmt.Errorf("remove column %s from %s: %w", column, sheet, err)
					}
				}
				book.Recount(sheet)
				return fmt.Sprintf("Removed %d column%s from %s, starting at %s.", count, plural(count), sheet, column), nil
			})
		},
	}
}
