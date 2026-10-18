package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/protocol"
	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/workbook"
)

// The tools that change how a workbook looks rather than what it holds.

func (s *session) formatRange() protocol.Tool {
	return protocol.Tool{
		Name: "format_range",
		Description: "Style a range in one call: bold or italic text, a font and size, a " +
			"text or fill colour, a number format, alignment, wrapping, rotation and a border. " +
			"Set number_format to a preset — currency, percent, date, integer, decimal — or " +
			"write an Excel format code. Use this for a title row, a total row, or the money " +
			"column of a table.",
		Schema: protocol.Object(map[string]any{
			"path":  protocol.String("The workbook to change."),
			"sheet": protocol.String("The sheet holding the range. Leave out for the active sheet."),
			"range": protocol.String("The range to style, such as A1:D1."),
			"style": protocol.Style("What the range should look like. Set only what should change."),
		}, "path", "range", "style"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			named, err := protocol.RequiredString(args, "range", 200)
			if err != nil {
				return "", err
			}
			rawStyle, ok := args["style"].(map[string]any)
			if !ok {
				return "", fmt.Errorf("style is required: an object naming at least one thing to change")
			}
			spec, err := workbook.StyleFrom(rawStyle)
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
				id, err := book.File().NewStyle(spec)
				if err != nil {
					return "", fmt.Errorf("build the style: %w", err)
				}
				if err := book.File().SetCellStyle(sheet, start, end, id); err != nil {
					return "", fmt.Errorf("style %s!%s:%s: %w", sheet, start, end, err)
				}
				return fmt.Sprintf("Styled %s!%s:%s.", sheet, start, end), nil
			})
		},
	}
}

func (s *session) setColumnWidth() protocol.Tool {
	return protocol.Tool{
		Name: "set_column_width",
		Description: "Set how wide one or more columns are, in Excel's own width units — " +
			"about one character per unit. Give a single column (\"C\") or a span " +
			"(\"C:E\"). A width of 0 hides the columns. write_table and write_range can size " +
			"columns to their contents instead, which is usually what you want.",
		Schema: protocol.Object(map[string]any{
			"path":    protocol.String("The workbook to change."),
			"sheet":   protocol.String("The sheet to change. Leave out for the active sheet."),
			"columns": protocol.String("The column or span of columns, such as C or B:D."),
			"width":   protocol.Number("The width in Excel's units, from 0 to 255."),
		}, "path", "columns", "width"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			columns, err := protocol.RequiredString(args, "columns", 40)
			if err != nil {
				return "", err
			}
			width, err := protocol.OptionalFloat(args, "width", -1)
			if err != nil {
				return "", err
			}
			if width < 0 || width > 255 {
				return "", fmt.Errorf("width must be between 0 and 255")
			}
			start, end, err := columnSpan(columns)
			if err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}
				if err := book.File().SetColWidth(sheet, start, end, width); err != nil {
					return "", fmt.Errorf("set the width of %s: %w", columns, err)
				}
				return fmt.Sprintf("Set %s!%s to width %g.", sheet, columns, width), nil
			})
		},
	}
}

func (s *session) setRowHeight() protocol.Tool {
	return protocol.Tool{
		Name: "set_row_height",
		Description: "Set how tall rows are, in points — 15 is Excel's usual height. Give " +
			"one row (\"5\") or a span (\"5:10\"). A height of 0 hides the rows. Taller rows " +
			"are what a wrapped paragraph or a title needs.",
		Schema: protocol.Object(map[string]any{
			"path":   protocol.String("The workbook to change."),
			"sheet":  protocol.String("The sheet to change. Leave out for the active sheet."),
			"rows":   protocol.String("The row or span of rows, such as 5 or 5:10."),
			"height": protocol.Number("The height in points, from 0 to 400."),
		}, "path", "rows", "height"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			rows, err := protocol.RequiredString(args, "rows", 40)
			if err != nil {
				return "", err
			}
			height, err := protocol.OptionalFloat(args, "height", -1)
			if err != nil {
				return "", err
			}
			if height < 0 || height > 400 {
				return "", fmt.Errorf("height must be between 0 and 400 points")
			}
			first, last, err := rowSpan(rows)
			if err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}
				for row := first; row <= last; row++ {
					if err := book.File().SetRowHeight(sheet, row, height); err != nil {
						return "", fmt.Errorf("set the height of row %d: %w", row, err)
					}
				}
				return fmt.Sprintf("Set rows %s of %s to height %g.", rows, sheet, height), nil
			})
		},
	}
}

func (s *session) mergeCells() protocol.Tool {
	return protocol.Tool{
		Name: "merge_cells",
		Description: "Join a range into one cell for a title or a heading spanning columns. " +
			"Only the top-left cell's value survives, so write the text there first. The text " +
			"is centred across the merged range unless center is set false. Set unmerge true " +
			"to take a merge apart again.",
		Schema: protocol.Object(map[string]any{
			"path":    protocol.String("The workbook to change."),
			"sheet":   protocol.String("The sheet holding the range. Leave out for the active sheet."),
			"range":   protocol.String("The range to merge, such as A1:D1."),
			"unmerge": protocol.Boolean("Take the merge apart instead of making it."),
			"center":  protocol.Boolean("Centre the text across the merged cells. On unless set false."),
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
			unmerge, err := protocol.OptionalBool(args, "unmerge", false)
			if err != nil {
				return "", err
			}
			center, err := protocol.OptionalBool(args, "center", true)
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
				file := book.File()

				if unmerge {
					if err := file.UnmergeCell(sheet, start, end); err != nil {
						return "", fmt.Errorf("unmerge %s!%s:%s: %w", sheet, start, end, err)
					}
					return fmt.Sprintf("Unmerged %s!%s:%s.", sheet, start, end), nil
				}

				if start == end {
					return "", fmt.Errorf("a merge needs more than one cell: give a range such as A1:D1")
				}
				if err := file.MergeCell(sheet, start, end); err != nil {
					return "", fmt.Errorf("merge %s!%s:%s: %w", sheet, start, end, err)
				}
				if center {
					style, err := file.NewStyle(&excelize.Style{
						Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
					})
					if err != nil {
						return "", err
					}
					if err := file.SetCellStyle(sheet, start, end, style); err != nil {
						return "", err
					}
				}
				return fmt.Sprintf("Merged %s!%s:%s.", sheet, start, end), nil
			})
		},
	}
}

func (s *session) freezePanes() protocol.Tool {
	return protocol.Tool{
		Name: "freeze_panes",
		Description: "Keep the rows above and the columns left of a cell in view while the " +
			"rest scrolls — the usual choice is A2, which holds the header row still. Use " +
			"unfreeze true to let everything scroll again.",
		Schema: protocol.Object(map[string]any{
			"path":     protocol.String("The workbook to change."),
			"sheet":    protocol.String("The sheet to change. Leave out for the active sheet."),
			"cell":     protocol.String("The first cell that scrolls, such as A2 to hold row 1 still, or B2 to hold row 1 and column A."),
			"unfreeze": protocol.Boolean("Remove any frozen rows and columns instead."),
		}, "path"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			unfreeze, err := protocol.OptionalBool(args, "unfreeze", false)
			if err != nil {
				return "", err
			}
			cell, err := protocol.OptionalString(args, "cell", "A2", 20)
			if err != nil {
				return "", err
			}
			normalized, err := workbook.NormalizeCell(cell)
			if err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}

				if unfreeze {
					if err := book.File().SetPanes(sheet, &excelize.Panes{}); err != nil {
						return "", fmt.Errorf("unfreeze %s: %w", sheet, err)
					}
					return fmt.Sprintf("Nothing is frozen on %s any more.", sheet), nil
				}

				x, y, err := excelize.CellNameToCoordinates(normalized)
				if err != nil {
					return "", err
				}
				xSplit, ySplit := x-1, y-1
				if xSplit == 0 && ySplit == 0 {
					return "", fmt.Errorf("cell %s freezes nothing: give a cell such as A2 or B2", normalized)
				}
				active := "bottomLeft"
				switch {
				case xSplit > 0 && ySplit > 0:
					active = "bottomRight"
				case xSplit > 0:
					active = "topRight"
				}

				panes := &excelize.Panes{
					Freeze:      true,
					XSplit:      xSplit,
					YSplit:      ySplit,
					TopLeftCell: normalized,
					ActivePane:  active,
					Selection:   []excelize.Selection{{SQRef: normalized, ActiveCell: normalized, Pane: active}},
				}
				if err := book.File().SetPanes(sheet, panes); err != nil {
					return "", fmt.Errorf("freeze %s on %s: %w", normalized, sheet, err)
				}
				return fmt.Sprintf("Frozen above and left of %s!%s.", sheet, normalized), nil
			})
		},
	}
}

func (s *session) setAutoFilter() protocol.Tool {
	return protocol.Tool{
		Name: "set_auto_filter",
		Description: "Put a filter button on the first row of a range, so the table can be " +
			"sorted and filtered by whoever opens the workbook. Give the range without its " +
			"header — the header row is the first row of what you give, so A1:D50 for a " +
			"header in row 1. A real table made by write_table already has this.",
		Schema: protocol.Object(map[string]any{
			"path":  protocol.String("The workbook to change."),
			"sheet": protocol.String("The sheet holding the range. Leave out for the active sheet."),
			"range": protocol.String("The range to filter, header row first, such as A1:D50."),
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
			start, end, err := workbook.ParseRange(named)
			if err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}
				if err := book.File().AutoFilter(sheet, start+":"+end, nil); err != nil {
					return "", fmt.Errorf("filter %s!%s:%s: %w", sheet, start, end, err)
				}
				return fmt.Sprintf("Filtered %s!%s:%s on its first row.", sheet, start, end), nil
			})
		},
	}
}

func (s *session) conditionalFormat() protocol.Tool {
	return protocol.Tool{
		Name: "conditional_format",
		Description: "Colour cells by what they hold: numbers above or below a value, a " +
			"value between two others, text that contains a word, or a colour scale and a " +
			"data bar across the range. The rule lives with the workbook, so it follows the " +
			"data when it changes. This is what makes a report read at a glance.",
		Schema: protocol.Object(map[string]any{
			"path":         protocol.String("The workbook to change."),
			"sheet":        protocol.String("The sheet holding the range. Leave out for the active sheet."),
			"range":        protocol.String("The range the rule applies to, such as C2:C100."),
			"rule":         protocol.Choice("What the rule looks for.", "greater_than", "greater_than_or_equal", "less_than", "less_than_or_equal", "equal", "not_equal", "between", "not_between", "contains_text", "not_contains_text", "begins_with", "ends_with", "color_scale", "data_bar"),
			"value":        protocol.String("What to compare against: a number, a cell such as $B$1, or the text to look for."),
			"value2":       protocol.String("The upper value when the rule is between or not_between."),
			"fill_color":   protocol.String("Background to put on a matching cell, as a hex value."),
			"font_color":   protocol.String("Text colour to put on a matching cell, as a hex value."),
			"bold":         protocol.Boolean("Show matching cells in bold."),
			"mid_color":    protocol.String("For color_scale: the middle colour, making it a three-colour scale."),
			"bar_color":    protocol.String("For data_bar: the colour of the bar."),
			"stop_if_true": protocol.Boolean("Stop later rules from applying to a matching cell."),
		}, "path", "range", "rule"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			named, err := protocol.RequiredString(args, "range", 200)
			if err != nil {
				return "", err
			}
			rule, err := protocol.RequiredChoice(args, "rule",
				"greater_than", "greater_than_or_equal", "less_than", "less_than_or_equal",
				"equal", "not_equal", "between", "not_between",
				"contains_text", "not_contains_text", "begins_with", "ends_with",
				"color_scale", "data_bar")
			if err != nil {
				return "", err
			}
			value, err := protocol.OptionalString(args, "value", "", 200)
			if err != nil {
				return "", err
			}
			value2, err := protocol.OptionalString(args, "value2", "", 200)
			if err != nil {
				return "", err
			}
			fill, err := protocol.OptionalString(args, "fill_color", "", 20)
			if err != nil {
				return "", err
			}
			font, err := protocol.OptionalString(args, "font_color", "", 20)
			if err != nil {
				return "", err
			}
			bold, err := protocol.OptionalBool(args, "bold", false)
			if err != nil {
				return "", err
			}
			midColor, err := protocol.OptionalString(args, "mid_color", "", 20)
			if err != nil {
				return "", err
			}
			barColor, err := protocol.OptionalString(args, "bar_color", "", 20)
			if err != nil {
				return "", err
			}
			stopIfTrue, err := protocol.OptionalBool(args, "stop_if_true", false)
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
				options, err := conditionalRule(book.File(), rule, value, value2, fill, font, bold, midColor, barColor, stopIfTrue)
				if err != nil {
					return "", err
				}
				if err := book.File().SetConditionalFormat(sheet, start+":"+end, []excelize.ConditionalFormatOptions{options}); err != nil {
					return "", fmt.Errorf("add the rule to %s!%s:%s: %w", sheet, start, end, err)
				}
				return fmt.Sprintf("The rule %q now colours matching cells in %s!%s:%s.", rule, sheet, start, end), nil
			})
		},
	}
}

func (s *session) dataValidation() protocol.Tool {
	return protocol.Tool{
		Name: "data_validation",
		Description: "Limit what a cell will accept: a dropdown list of choices, a whole " +
			"number or decimal in a range, a date, or text of a certain length. A list of " +
			"values gives the workbook a dropdown, which is the simplest way to keep a " +
			"column consistent. Excel shows the message when someone types something the " +
			"rule refuses.",
		Schema: protocol.Object(map[string]any{
			"path":          protocol.String("The workbook to change."),
			"sheet":         protocol.String("The sheet holding the range. Leave out for the active sheet."),
			"range":         protocol.String("The cells the rule applies to, such as D2:D200."),
			"kind":          protocol.Choice("What the cell will accept.", "list", "whole", "decimal", "date", "time", "text_length", "custom"),
			"values":        protocol.Array("For a list: the choices, in the order they should appear."),
			"formula1":      protocol.String("The limit, instead of values: a number, another cell such as $B$1, or a range holding a list."),
			"formula2":      protocol.String("The upper limit for a between rule."),
			"operator":      protocol.Choice("How the limit is read.", "between", "not_between", "equal", "not_equal", "greater_than", "less_than", "greater_than_or_equal", "less_than_or_equal"),
			"allow_blank":   protocol.Boolean("Allow the cell to be left empty. On unless set false."),
			"title":         protocol.String("The heading of the message Excel shows for a refused value."),
			"message":       protocol.String("The text of that message."),
			"input_title":   protocol.String("The heading of the note shown when the cell is selected."),
			"input_message": protocol.String("The text of that note."),
		}, "path", "range", "kind"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			named, err := protocol.RequiredString(args, "range", 200)
			if err != nil {
				return "", err
			}
			kind, err := protocol.RequiredChoice(args, "kind", "list", "whole", "decimal", "date", "time", "text_length", "custom")
			if err != nil {
				return "", err
			}
			values, err := protocol.StringList(args, "values", 200)
			if err != nil {
				return "", err
			}
			formula1, err := protocol.OptionalString(args, "formula1", "", 300)
			if err != nil {
				return "", err
			}
			formula2, err := protocol.OptionalString(args, "formula2", "", 300)
			if err != nil {
				return "", err
			}
			operator, err := protocol.OptionalChoice(args, "operator", "between",
				"between", "not_between", "equal", "not_equal",
				"greater_than", "less_than", "greater_than_or_equal", "less_than_or_equal")
			if err != nil {
				return "", err
			}
			allowBlank, err := protocol.OptionalBool(args, "allow_blank", true)
			if err != nil {
				return "", err
			}
			title, err := protocol.OptionalString(args, "title", "", 100)
			if err != nil {
				return "", err
			}
			message, err := protocol.OptionalString(args, "message", "", 300)
			if err != nil {
				return "", err
			}
			inputTitle, err := protocol.OptionalString(args, "input_title", "", 100)
			if err != nil {
				return "", err
			}
			inputMessage, err := protocol.OptionalString(args, "input_message", "", 300)
			if err != nil {
				return "", err
			}
			start, end, err := workbook.ParseRange(named)
			if err != nil {
				return "", err
			}

			rule, err := validationRule(kind, values, formula1, formula2, operator, allowBlank, title, message, inputTitle, inputMessage)
			if err != nil {
				return "", err
			}
			rule.Sqref = start + ":" + end

			return s.edit(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}
				if err := book.File().AddDataValidation(sheet, rule); err != nil {
					return "", fmt.Errorf("add the rule to %s!%s:%s: %w", sheet, start, end, err)
				}
				return fmt.Sprintf("Cells %s!%s:%s now accept %s.", sheet, start, end, describeRule(kind, values)), nil
			})
		},
	}
}

// columnSpan reads a column or a span of columns.
func columnSpan(columns string) (string, string, error) {
	parts := strings.SplitN(columns, ":", 2)
	start := strings.ToUpper(strings.TrimSpace(parts[0]))
	if _, err := workbook.ColumnNumber(start); err != nil {
		return "", "", err
	}
	if len(parts) == 1 {
		return start, start, nil
	}
	end := strings.ToUpper(strings.TrimSpace(parts[1]))
	if _, err := workbook.ColumnNumber(end); err != nil {
		return "", "", err
	}
	return start, end, nil
}

// rowSpan reads a row or a span of rows.
func rowSpan(rows string) (int, int, error) {
	parts := strings.SplitN(rows, ":", 2)
	first, err := rowNumberValue(parts[0])
	if err != nil {
		return 0, 0, err
	}
	if len(parts) == 1 {
		return first, first, nil
	}
	last, err := rowNumberValue(parts[1])
	if err != nil {
		return 0, 0, err
	}
	if last < first {
		first, last = last, first
	}
	return first, last, nil
}

func rowNumberValue(text string) (int, error) {
	var number int
	if _, err := fmt.Sscanf(strings.TrimSpace(text), "%d", &number); err != nil || number < 1 || number > 1048576 {
		return 0, fmt.Errorf("%q is not a row number: give one like 5 or 5:10", text)
	}
	return number, nil
}
