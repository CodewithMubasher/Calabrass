package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/protocol"
	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/workbook"
)

// The tools that put values into a workbook.

func (s *session) writeRange() protocol.Tool {
	return protocol.Tool{
		Name: "write_range",
		Description: "Write a block of values into a range, starting at a cell. Values are " +
			"rows of cells; a cell is text, a number, true/false or null. A cell that is " +
			"null is left alone. Text starting with \"=\" is written as a formula, the way " +
			"typing it into a spreadsheet would; start it with an apostrophe to keep an " +
			"\"=\" as text. Existing cells the block covers are overwritten.",
		Schema: protocol.Object(map[string]any{
			"path":    protocol.String("The workbook to write to."),
			"sheet":   protocol.String("The sheet to write to. Leave out for the active sheet."),
			"start":   protocol.String("The top-left cell to write from, such as A1 or C7. A1 unless set."),
			"values":  protocol.Grid("The rows to write: an array of rows, each row an array of cells."),
			"autofit": protocol.Boolean("Widen the columns to fit what was written."),
		}, "path", "values"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			values, err := protocol.Rows(args, "values", maxWriteRows, maxWriteCols)
			if err != nil {
				return "", err
			}
			start, err := protocol.OptionalString(args, "start", "A1", 20)
			if err != nil {
				return "", err
			}
			autofit, err := protocol.OptionalBool(args, "autofit", false)
			if err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}
				first, last, err := book.WriteBlock(sheet, start, values)
				if err != nil {
					return "", err
				}
				if autofit {
					book.Autofit(sheet, first, values)
				}
				return fmt.Sprintf("Wrote %d row%s to %s!%s:%s.", len(values), plural(len(values)), sheet, first, last), nil
			})
		},
	}
}

func (s *session) appendRows() protocol.Tool {
	return protocol.Tool{
		Name: "append_rows",
		Description: "Add rows to the bottom of a sheet's data, keeping whatever is already " +
			"there. This is the tool for adding one more order, one more day of figures, one " +
			"more name to a list that already exists. The rows start in the sheet's first " +
			"data column, so they line up with the columns above them.",
		Schema: protocol.Object(map[string]any{
			"path":   protocol.String("The workbook to add to."),
			"sheet":  protocol.String("The sheet to add to. Leave out for the active sheet."),
			"values": protocol.Grid("The rows to add, one after another."),
		}, "path", "values"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			values, err := protocol.Rows(args, "values", maxWriteRows, maxWriteCols)
			if err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}

				start := "A1"
				used, hasData, err := book.UsedRange(sheet)
				if err != nil {
					return "", err
				}
				if hasData {
					from, to, err := workbook.ParseRange(used)
					if err != nil {
						return "", err
					}
					firstColumn := workbook.ColumnOf(from)
					nextRow := strconv.Itoa(workbook.LastRow(to) + 1)
					start = firstColumn + nextRow
				}

				first, last, err := book.WriteBlock(sheet, start, values)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("Added %d row%s at %s!%s:%s.", len(values), plural(len(values)), sheet, first, last), nil
			})
		},
	}
}

func (s *session) writeTable() protocol.Tool {
	return protocol.Tool{
		Name: "write_table",
		Description: "Write a table of data in one call: a row of column names and the rows " +
			"under it, with the columns sized to fit. By default it becomes a real Excel " +
			"table — a styled block with a filter button on every column and a name you can " +
			"use in formulas — which is what most \"make me a spreadsheet\" requests want. " +
			"Set table false for a plain block with a bold header instead.",
		Schema: protocol.Object(map[string]any{
			"path":    protocol.String("The workbook to write to."),
			"sheet":   protocol.String("The sheet to write to. Leave out for the active sheet."),
			"start":   protocol.String("The top-left cell of the header row, such as A1. A1 unless set."),
			"headers": protocol.Array("The column names, left to right."),
			"rows":    protocol.Grid("The data rows, each row an array of cells."),
			"table":   protocol.Boolean("Make it a real Excel table with a filter and a style. On unless set false."),
			"style":   protocol.Choice("The table style when table is on.", "TableStyleLight1", "TableStyleLight9", "TableStyleLight11", "TableStyleMedium2", "TableStyleMedium4", "TableStyleMedium9", "TableStyleDark1", "TableStyleDark3"),
			"autofit": protocol.Boolean("Widen the columns to fit their contents. On unless set false."),
		}, "path"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			headers, err := protocol.StringList(args, "headers", maxWriteCols)
			if err != nil {
				return "", err
			}
			rows, err := optionalRows(args, "rows")
			if err != nil {
				return "", err
			}
			if len(headers) == 0 && len(rows) == 0 {
				return "", fmt.Errorf("a table needs headers, rows, or both")
			}
			asTable, err := protocol.OptionalBool(args, "table", true)
			if err != nil {
				return "", err
			}
			style, err := protocol.OptionalString(args, "style", "TableStyleMedium2", 60)
			if err != nil {
				return "", err
			}
			autofit, err := protocol.OptionalBool(args, "autofit", true)
			if err != nil {
				return "", err
			}
			start, err := protocol.OptionalString(args, "start", "A1", 20)
			if err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}

				width := len(headers)
				for _, row := range rows {
					if len(row) > width {
						width = len(row)
					}
				}
				if width == 0 {
					return "", fmt.Errorf("the table has no columns")
				}

				// The header row is written as data even when the table is off,
				// because a header a person cannot read is worse than one Excel
				// does not call a table.
				var block [][]any
				if len(headers) > 0 {
					row := make([]any, width)
					for i := range row {
						if i < len(headers) {
							row[i] = headers[i]
						}
					}
					block = append(block, row)
				}
				block = append(block, rows...)

				first, last, err := book.WriteBlock(sheet, start, block)
				if err != nil {
					return "", err
				}

				if asTable {
					name, err := uniqueTableName(book.File())
					if err != nil {
						return "", err
					}
					table := &excelize.Table{
						Range:     first + ":" + last,
						Name:      name,
						StyleName: style,
					}
					if err := book.File().AddTable(sheet, table); err != nil {
						return "", fmt.Errorf("make the table on %s: %w", sheet, err)
					}
				} else if len(headers) > 0 {
					if err := styleHeader(book.File(), sheet, first, workbook.ColumnOf(last)); err != nil {
						return "", err
					}
				}

				if autofit {
					book.Autofit(sheet, first, block)
				}

				shape := fmt.Sprintf("%d row%s and %d column%s", len(rows), plural(len(rows)), width, plural(width))
				if asTable {
					return fmt.Sprintf("Wrote a table of %s over %s!%s:%s.", shape, sheet, first, last), nil
				}
				return fmt.Sprintf("Wrote %s over %s!%s:%s.", shape, sheet, first, last), nil
			})
		},
	}
}

func (s *session) clearRange() protocol.Tool {
	return protocol.Tool{
		Name: "clear_range",
		Description: "Empty a range. Clearing contents leaves the cell's formatting and " +
			"number format in place; clearing all takes the formatting with it. This does " +
			"not remove rows or columns, so nothing below or beside the range moves — use " +
			"delete_rows or delete_columns for that.",
		Schema: protocol.Object(map[string]any{
			"path":  protocol.String("The workbook to change."),
			"sheet": protocol.String("The sheet holding the range. Leave out for the active sheet."),
			"range": protocol.String("The range to empty, such as A2:C10."),
			"what":  protocol.Choice("What to clear: contents keeps the formatting, all takes it too.", "contents", "all"),
		}, "path", "range"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			named, err := protocol.RequiredString(args, "range", 200)
			if err != nil {
				return "", err
			}
			mode, err := protocol.OptionalChoice(args, "what", "contents", "contents", "all")
			if err != nil {
				return "", err
			}
			start, end, err := workbook.ParseRange(named)
			if err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}
				x1, y1, x2, y2, err := workbook.Corners(start, end)
				if err != nil {
					return "", err
				}
				for y := y1; y <= y2; y++ {
					for x := x1; x <= x2; x++ {
						ref, err := excelize.CoordinatesToCellName(x, y)
						if err != nil {
							return "", err
						}
						if err := book.File().SetCellValue(sheet, ref, nil); err != nil {
							return "", fmt.Errorf("empty %s: %w", ref, err)
						}
					}
				}
				if mode == "all" {
					if err := book.File().SetCellStyle(sheet, start, end, 0); err != nil {
						return "", fmt.Errorf("clear the formatting of %s: %w", start+":"+end, err)
					}
				}
				// The sheet's record of its size has to be told, or the next call
				// would append after rows that are now empty.
				book.Recount(sheet)
				return fmt.Sprintf("Emptied %s!%s:%s (%s).", sheet, start, end, mode), nil
			})
		},
	}
}

// optionalRows reads a rows argument that may be left out, unlike Rows which
// insists on it.
func optionalRows(args map[string]any, name string) ([][]any, error) {
	if value, ok := args[name]; !ok || value == nil {
		return nil, nil
	}
	return protocol.Rows(args, name, maxWriteRows, maxWriteCols)
}

// uniqueTableName picks a name no table or defined name in the workbook is
// using, because Excel refuses a workbook where two tables share one.
func uniqueTableName(file *excelize.File) (string, error) {
	used := map[string]bool{}
	for _, sheet := range file.GetSheetList() {
		tables, err := file.GetTables(sheet)
		if err != nil {
			continue
		}
		for _, table := range tables {
			used[strings.ToLower(table.Name)] = true
		}
	}
	for _, defined := range file.GetDefinedName() {
		used[strings.ToLower(defined.Name)] = true
	}
	for i := 1; i <= 100000; i++ {
		candidate := "Table" + strconv.Itoa(i)
		if !used[strings.ToLower(candidate)] {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("this workbook has too many tables to name another")
}

// styleHeader makes a plain header row read as a header: bold on a soft fill.
func styleHeader(file *excelize.File, sheet, first, lastColumn string) error {
	fill := "#DDEBF7"
	style, err := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "1F4E78"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{fill}},
	})
	if err != nil {
		return fmt.Errorf("build the header style: %w", err)
	}
	row := workbook.RowOf(first)
	if err := file.SetCellStyle(sheet, workbook.ColumnOf(first)+row, lastColumn+row, style); err != nil {
		return fmt.Errorf("style the header row: %w", err)
	}
	return nil
}
