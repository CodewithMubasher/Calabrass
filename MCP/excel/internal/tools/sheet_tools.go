package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/protocol"
	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/workbook"
)

// The tools about a workbook's sheets.

func (s *session) addSheet() protocol.Tool {
	return protocol.Tool{
		Name: "add_sheet",
		Description: "Add an empty sheet to an existing workbook. The sheet is added at " +
			"the end; a workbook's sheet order is set when it is made, so name the sheets " +
			"in create_workbook when the order matters. Refuses a name the workbook already " +
			"uses, and a name Excel would not accept.",
		Schema: protocol.Object(map[string]any{
			"path": protocol.String("The workbook to add the sheet to."),
			"name": protocol.String("What to call the sheet: up to 31 characters, no : \\ / ? * [ ]."),
		}, "path", "name"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			name, err := protocol.RequiredString(args, "name", 100)
			if err != nil {
				return "", err
			}
			if err := workbook.ValidSheetName(name); err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				if _, exists := book.SheetIndex(name); exists {
					return "", fmt.Errorf("this workbook already has a sheet called %q", name)
				}
				if _, err := book.File().NewSheet(name); err != nil {
					return "", fmt.Errorf("add the sheet %q: %w", name, err)
				}
				return fmt.Sprintf("Added the sheet %q. The workbook now has %s.",
					name, sheetsLine(book)), nil
			})
		},
	}
}

func (s *session) renameSheet() protocol.Tool {
	return protocol.Tool{
		Name: "rename_sheet",
		Description: "Give an existing sheet a new name. Formulas and references inside " +
			"the workbook follow the new name on their own, so there is nothing else to " +
			"update.",
		Schema: protocol.Object(map[string]any{
			"path":     protocol.String("The workbook holding the sheet."),
			"sheet":    protocol.String("The sheet to rename."),
			"new_name": protocol.String("The new name: up to 31 characters, no : \\ / ? * [ ]."),
		}, "path", "sheet", "new_name"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			newName, err := protocol.RequiredString(args, "new_name", 100)
			if err != nil {
				return "", err
			}
			if err := workbook.ValidSheetName(newName); err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}
				if sheet == newName {
					return "", fmt.Errorf("the sheet is already called %q", newName)
				}
				if _, exists := book.SheetIndex(newName); exists {
					return "", fmt.Errorf("this workbook already has a sheet called %q", newName)
				}
				if err := book.File().SetSheetName(sheet, newName); err != nil {
					return "", fmt.Errorf("rename %q: %w", sheet, err)
				}
				return fmt.Sprintf("Renamed %q to %q.", sheet, newName), nil
			})
		},
	}
}

func (s *session) deleteSheet() protocol.Tool {
	return protocol.Tool{
		Name: "delete_sheet",
		Description: "Delete a sheet and everything in it. A workbook must keep at least " +
			"one sheet, so the last one cannot be deleted. This cannot be undone: check " +
			"get_workbook_info first when the workbook is not one you made this turn.",
		Schema: protocol.Object(map[string]any{
			"path":  protocol.String("The workbook holding the sheet."),
			"sheet": protocol.String("The sheet to delete."),
		}, "path", "sheet"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}
				if len(book.File().GetSheetList()) == 1 {
					return "", fmt.Errorf("%q is the workbook's only sheet: a workbook must keep at least one", sheet)
				}
				if err := book.File().DeleteSheet(sheet); err != nil {
					return "", fmt.Errorf("delete %q: %w", sheet, err)
				}
				return fmt.Sprintf("Deleted %q. Left: %s.", sheet, sheetsLine(book)), nil
			})
		},
	}
}

func (s *session) copySheet() protocol.Tool {
	return protocol.Tool{
		Name: "copy_sheet",
		Description: "Copy a sheet inside the same workbook, values, formulas and " +
			"formatting, under a new name. Charts, tables and page settings are not part " +
			"of the copy; add those again with add_chart and write_table.",
		Schema: protocol.Object(map[string]any{
			"path":   protocol.String("The workbook holding the sheet."),
			"source": protocol.String("The sheet to copy."),
			"name":   protocol.String("What to call the copy."),
		}, "path", "source", "name"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			source, err := protocol.RequiredString(args, "source", 100)
			if err != nil {
				return "", err
			}
			name, err := protocol.RequiredString(args, "name", 100)
			if err != nil {
				return "", err
			}
			if err := workbook.ValidSheetName(name); err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				file := book.File()
				if _, exists := book.SheetIndex(source); !exists {
					return "", fmt.Errorf("there is no sheet called %q: this workbook has %s", source, sheetsLine(book))
				}
				if _, exists := book.SheetIndex(name); exists {
					return "", fmt.Errorf("this workbook already has a sheet called %q", name)
				}
				fromIndex, _ := book.SheetIndex(source)
				toIndex, err := file.NewSheet(name)
				if err != nil {
					return "", fmt.Errorf("add the sheet %q: %w", name, err)
				}
				if err := file.CopySheet(fromIndex, toIndex); err != nil {
					return "", fmt.Errorf("copy %q to %q: %w", source, name, err)
				}
				return fmt.Sprintf("Copied %q to %q.", source, name), nil
			})
		},
	}
}

func (s *session) setSheetVisibility() protocol.Tool {
	return protocol.Tool{
		Name: "set_sheet_visibility",
		Description: "Show or hide a sheet. A hidden sheet is out of the way but still " +
			"there, and its cells still answer to formulas; a very hidden sheet cannot be " +
			"brought back from Excel's own menu. A workbook must keep one visible sheet, so " +
			"the last one cannot be hidden.",
		Schema: protocol.Object(map[string]any{
			"path":  protocol.String("The workbook holding the sheet."),
			"sheet": protocol.String("The sheet to show or hide."),
			"state": protocol.Choice("visible shows it, hidden tucks it away, very_hidden makes it hard to find.", "visible", "hidden", "very_hidden"),
		}, "path", "state"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			state, err := protocol.RequiredChoice(args, "state", "visible", "hidden", "very_hidden")
			if err != nil {
				return "", err
			}

			return s.edit(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}
				file := book.File()

				if state != "visible" && visibleSheets(file) <= 1 {
					if visible, err := file.GetSheetVisible(sheet); err == nil && visible {
						return "", fmt.Errorf("%q is the workbook's only visible sheet: one must stay visible", sheet)
					}
				}

				switch state {
				case "visible":
					err = file.SetSheetVisible(sheet, true)
				case "hidden":
					err = file.SetSheetVisible(sheet, false)
				case "very_hidden":
					err = file.SetSheetVisible(sheet, false, true)
				}
				if err != nil {
					return "", fmt.Errorf("set %q to %s: %w", sheet, state, err)
				}

				// Excel will not open a workbook whose active sheet is hidden, so
				// the tab in front moves to one that is showing.
				if index := file.GetActiveSheetIndex(); file.GetSheetName(index) == sheet && state != "visible" {
					for i, name := range file.GetSheetList() {
						if visible, err := file.GetSheetVisible(name); err == nil && visible {
							file.SetActiveSheet(i)
							break
						}
					}
				}

				return fmt.Sprintf("The sheet %q is now %s.", sheet, strings.ReplaceAll(state, "_", " ")), nil
			})
		},
	}
}

func visibleSheets(file interface {
	GetSheetList() []string
	GetSheetVisible(string) (bool, error)
}) int {
	count := 0
	for _, name := range file.GetSheetList() {
		if visible, err := file.GetSheetVisible(name); err == nil && visible {
			count++
		}
	}
	return count
}
