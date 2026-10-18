package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/protocol"
	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/workbook"
)

// The tools that read a workbook: what is in a range, and where a word is.

func (s *session) readRange() protocol.Tool {
	return protocol.Tool{
		Name: "read_range",
		Description: "Read a range of cells as a table of rows, with an optional map of the " +
			"formulas in it. Leave the range out to read everything the sheet has, up to " +
			"20000 cells. Values come back the way Excel shows them (a percent as 12.50%, a " +
			"date as 2026-01-15); pass raw true for the numbers underneath, which is what you " +
			"want before doing arithmetic on them. A formula's result is worked out where this " +
			"server can, since a workbook made here has never been opened by Excel.",
		Schema: protocol.Object(map[string]any{
			"path":     protocol.String("The workbook to read."),
			"sheet":    protocol.String("The sheet to read. Leave out for the active sheet."),
			"range":    protocol.String("The range to read, such as A1:C10 or B2. Leave out for everything the sheet has."),
			"raw":      protocol.Boolean("Return the stored number rather than the displayed text."),
			"formulas": protocol.Boolean("Also return the formulas in the range, by cell."),
			"trim":     protocol.Boolean("Drop blank cells from the end of each row. On unless set false."),
		}, "path"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			raw, err := protocol.OptionalBool(args, "raw", false)
			if err != nil {
				return "", err
			}
			wantFormulas, err := protocol.OptionalBool(args, "formulas", false)
			if err != nil {
				return "", err
			}
			trim, err := protocol.OptionalBool(args, "trim", true)
			if err != nil {
				return "", err
			}
			named, err := protocol.OptionalString(args, "range", "", 200)
			if err != nil {
				return "", err
			}

			return s.read(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}

				start, end := "", ""
				if named == "" {
					used, hasData, err := book.UsedRange(sheet)
					if err != nil {
						return "", err
					}
					if !hasData {
						return fmt.Sprintf("The sheet %q is empty.", sheet), nil
					}
					if start, end, err = workbook.ParseRange(used); err != nil {
						return "", err
					}
				} else if start, end, err = workbook.ParseRange(named); err != nil {
					return "", err
				}

				block, err := book.ReadBlock(sheet, start, end, raw, wantFormulas)
				if err != nil {
					return "", err
				}
				if trim {
					block = workbook.TrimBlock(block)
				}

				answer := map[string]any{
					"sheet": block.Sheet,
					"range": block.Range,
					"rows":  block.Values(),
				}
				if wantFormulas {
					if formulas := block.Formulas(); len(formulas) > 0 {
						answer["formulas"] = formulas
					}
				}
				if block.Truncated {
					answer["truncated"] = true
					answer["note"] = fmt.Sprintf("Only the first %d cells were read: ask for a smaller range, or read it in parts.", maxReadCellsForTools)
				}

				encoded, err := json.MarshalIndent(answer, "", "  ")
				if err != nil {
					return "", err
				}
				return string(encoded), nil
			})
		},
	}
}

// maxReadCellsForTools mirrors the limit the workbook package applies to one
// read; it is repeated in the answer so the model knows why a read stopped.
const maxReadCellsForTools = 20000

func (s *session) searchCells() protocol.Tool {
	return protocol.Tool{
		Name: "search_cells",
		Description: "Find cells holding a piece of text, across one sheet or the whole " +
			"workbook, and answer with the sheet, the cell and its value. The search is " +
			"case-insensitive and matches part of a cell unless whole_cell is set. Use it " +
			"to find where a name, an item or a total lives before reading around it.",
		Schema: protocol.Object(map[string]any{
			"path":        protocol.String("The workbook to search."),
			"query":       protocol.String("The text to look for."),
			"sheet":       protocol.String("The sheet to search. Leave out to search every sheet."),
			"match_case":  protocol.Boolean("Match capital and small letters exactly."),
			"whole_cell":  protocol.Boolean("Only match a cell whose whole value is the query."),
			"max_results": protocol.Integer("Stop after this many matches.", 1, 500),
		}, "path", "query"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			query, err := protocol.RequiredString(args, "query", 500)
			if err != nil {
				return "", err
			}
			matchCase, err := protocol.OptionalBool(args, "match_case", false)
			if err != nil {
				return "", err
			}
			whole, err := protocol.OptionalBool(args, "whole_cell", false)
			if err != nil {
				return "", err
			}
			max, err := protocol.OptionalInt(args, "max_results", 50, 1, 500)
			if err != nil {
				return "", err
			}
			named, err := protocol.OptionalString(args, "sheet", "", 100)
			if err != nil {
				return "", err
			}

			return s.read(path, func(book *workbook.Book) (string, error) {
				sheets := book.File().GetSheetList()
				if named != "" {
					sheet, err := book.Sheet(named)
					if err != nil {
						return "", err
					}
					sheets = []string{sheet}
				}

				needle := query
				if !matchCase {
					needle = strings.ToLower(needle)
				}

				type hit struct {
					Sheet string `json:"sheet"`
					Cell  string `json:"cell"`
					Value string `json:"value"`
				}
				var (
					hits      []hit
					truncated bool
				)

				for _, sheet := range sheets {
					if len(hits) >= max {
						break
					}
					used, hasData, err := book.UsedRange(sheet)
					if err != nil || !hasData {
						continue
					}
					start, end, err := workbook.ParseRange(used)
					if err != nil {
						continue
					}
					block, err := book.ReadBlock(sheet, start, end, false, false)
					if err != nil {
						return "", err
					}
					if block.Truncated {
						truncated = true
					}
					for _, row := range block.Rows {
						for _, cell := range row {
							if cell.Value == "" || !matches(cell.Value, needle, matchCase, whole) {
								continue
							}
							hits = append(hits, hit{Sheet: sheet, Cell: cell.Ref, Value: cell.Value})
							if len(hits) >= max {
								truncated = true
								break
							}
						}
						if len(hits) >= max {
							break
						}
					}
				}

				if len(hits) == 0 {
					return fmt.Sprintf("Nothing in %s holds %q.", book.Name(), query), nil
				}
				answer := map[string]any{
					"count": len(hits),
					"hits":  hits,
				}
				if truncated {
					answer["truncated"] = true
					answer["note"] = "There may be more matches; narrow the search or raise max_results."
				}
				encoded, err := json.MarshalIndent(answer, "", "  ")
				if err != nil {
					return "", err
				}
				return string(encoded), nil
			})
		},
	}
}

func matches(value, needle string, matchCase, whole bool) bool {
	haystack := value
	if !matchCase {
		haystack = strings.ToLower(haystack)
	}
	if whole {
		return haystack == needle
	}
	return strings.Contains(haystack, needle)
}
