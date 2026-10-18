package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/protocol"
	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/workbook"
)

// The tools about the workbook as a whole: making one, finding one, and reading
// what is in it.

func (s *session) createWorkbook() protocol.Tool {
	return protocol.Tool{
		Name: "create_workbook",
		Description: "Make a new Excel workbook (.xlsx) and save it. Name the sheets to " +
			"start with, in the order they should appear; a workbook with one sheet " +
			"called \"Sheet1\" is made when none are named. This refuses to replace a " +
			"workbook that is already there unless overwrite is true, so a workbook is " +
			"never lost to a mistyped name. Use write_range or write_table afterwards to " +
			"put values in it.",
		Schema: protocol.Object(map[string]any{
			"path":      protocol.String("Where to put the workbook, ending in .xlsx. A relative path lands in this server's folder."),
			"sheets":    protocol.Array("The sheet names to start with, in order. Excel allows 31 characters and no : \\ / ? * [ ]."),
			"overwrite": protocol.Boolean("Replace a workbook that is already at that path. Leave out to be refused instead."),
			"title":     protocol.String("The workbook's title property, shown by Excel in its file details."),
			"creator":   protocol.String("Who made it, for the file details."),
		}, "path"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			sheets, err := protocol.StringList(args, "sheets", 50)
			if err != nil {
				return "", err
			}
			overwrite, err := protocol.OptionalBool(args, "overwrite", false)
			if err != nil {
				return "", err
			}
			title, err := protocol.OptionalString(args, "title", "", 250)
			if err != nil {
				return "", err
			}
			creator, err := protocol.OptionalString(args, "creator", "", 250)
			if err != nil {
				return "", err
			}

			return s.make(path, sheets, overwrite, func(book *workbook.Book) (string, error) {
				if title != "" || creator != "" {
					props := &excelize.DocProperties{Title: title, Creator: creator, LastModifiedBy: creator}
					if err := book.File().SetDocProps(props); err != nil {
						return "", fmt.Errorf("set the workbook's details: %w", err)
					}
				}
				return fmt.Sprintf("Made %s with the sheet%s %s.", book.Name(),
					plural(len(book.File().GetSheetList())), sheetsLine(book)), nil
			})
		},
	}
}

func (s *session) listWorkbooks() protocol.Tool {
	return protocol.Tool{
		Name: "list_workbooks",
		Description: "List the Excel workbooks in a folder, with their size and when they " +
			"last changed. Use it to find a workbook the user already has before making " +
			"another one, or to see what has been made so far. Only folders inside this " +
			"server's folder can be listed unless it was told to allow more.",
		Schema: protocol.Object(map[string]any{
			"directory": protocol.String("The folder to look in, relative to this server's folder. Leave out for the folder itself."),
			"recursive": protocol.Boolean("Look inside sub-folders too."),
		}),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			directory, err := protocol.OptionalString(args, "directory", "", 300)
			if err != nil {
				return "", err
			}
			recursive, err := protocol.OptionalBool(args, "recursive", false)
			if err != nil {
				return "", err
			}

			found, err := s.root.List(directory, recursive)
			if err != nil {
				return "", err
			}
			if len(found) == 0 {
				return fmt.Sprintf("No workbooks in %s yet.", s.root.Dir()), nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "%d workbook%s in %s:\n", len(found), plural(len(found)), s.root.Dir())
			for _, item := range found {
				fmt.Fprintf(&b, "- %s (%s, changed %s)\n", item.Path, humanBytes(item.Size), item.ModTime)
			}
			return strings.TrimRight(b.String(), "\n"), nil
		},
	}
}

func (s *session) workbookInfo() protocol.Tool {
	return protocol.Tool{
		Name: "get_workbook_info",
		Description: "Describe a workbook before working on it: every sheet, how far its " +
			"data reaches, which sheets are hidden, where tables, merged cells, frozen rows " +
			"and dropdowns are, and the workbook's own properties. This is the cheapest way " +
			"to see the shape of a workbook someone else made.",
		Schema: protocol.Object(map[string]any{
			"path": protocol.String("The workbook to describe."),
		}, "path"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}

			return s.read(path, func(book *workbook.Book) (string, error) {
				info, err := describe(book)
				if err != nil {
					return "", err
				}
				encoded, err := json.MarshalIndent(info, "", "  ")
				if err != nil {
					return "", err
				}
				return string(encoded), nil
			})
		},
	}
}

type sheetReport struct {
	Name         string   `json:"name"`
	Position     int      `json:"position"`
	Visibility   string   `json:"visibility"`
	UsedRange    string   `json:"usedRange,omitempty"`
	HasData      bool     `json:"hasData"`
	Tables       int      `json:"tables,omitempty"`
	MergedCells  []string `json:"mergedCells,omitempty"`
	FreezePanes  string   `json:"freezePanes,omitempty"`
	Validations  int      `json:"dataValidations,omitempty"`
	Conditionals int      `json:"conditionalFormats,omitempty"`
}

type nameReport struct {
	Name     string `json:"name"`
	RefersTo string `json:"refersTo"`
	Scope    string `json:"scope,omitempty"`
}

type propertiesReport struct {
	Title    string `json:"title,omitempty"`
	Creator  string `json:"creator,omitempty"`
	Modified string `json:"modified,omitempty"`
	Subject  string `json:"subject,omitempty"`
}

type workbookReport struct {
	File         string            `json:"path"`
	ActiveSheet  string            `json:"activeSheet"`
	DefaultSheet string            `json:"sheetAChangeActsOn,omitempty"`
	Sheets       []sheetReport     `json:"sheets"`
	DefinedNames []nameReport      `json:"definedNames,omitempty"`
	Properties   *propertiesReport `json:"properties,omitempty"`
}

// describe reads every sheet's shape without reading its cells.
func describe(book *workbook.Book) (workbookReport, error) {
	file := book.File()
	report := workbookReport{
		File:         book.Name(),
		ActiveSheet:  book.DefaultSheet(),
		DefaultSheet: book.DefaultSheet(),
	}

	for position, name := range file.GetSheetList() {
		sheet := sheetReport{Name: name, Position: position + 1, Visibility: "visible"}

		if visible, err := file.GetSheetVisible(name); err == nil && !visible {
			sheet.Visibility = "hidden"
		}

		if used, hasData, err := book.UsedRange(name); err == nil {
			sheet.HasData = hasData
			if hasData {
				sheet.UsedRange = used
			}
		}

		if tables, err := file.GetTables(name); err == nil {
			sheet.Tables = len(tables)
		}
		if merges, err := file.GetMergeCells(name); err == nil {
			for _, merge := range merges {
				sheet.MergedCells = append(sheet.MergedCells, merge.GetStartAxis()+":"+merge.GetEndAxis())
			}
		}
		if panes, err := file.GetPanes(name); err == nil && panes.Freeze {
			sheet.FreezePanes = panes.TopLeftCell
			if sheet.FreezePanes == "" {
				sheet.FreezePanes = "yes"
			}
		}
		if validations, err := file.GetDataValidations(name); err == nil {
			for _, validation := range validations {
				if validation != nil {
					sheet.Validations++
				}
			}
		}
		if formats, err := file.GetConditionalFormats(name); err == nil {
			sheet.Conditionals = len(formats)
		}

		report.Sheets = append(report.Sheets, sheet)
	}

	for _, defined := range file.GetDefinedName() {
		report.DefinedNames = append(report.DefinedNames, nameReport{
			Name:     defined.Name,
			RefersTo: defined.RefersTo,
			Scope:    defined.Scope,
		})
	}

	if props, err := file.GetDocProps(); err == nil && props != nil {
		if props.Title != "" || props.Creator != "" || props.Modified != "" || props.Subject != "" {
			report.Properties = &propertiesReport{
				Title:    props.Title,
				Creator:  props.Creator,
				Modified: props.Modified,
				Subject:  props.Subject,
			}
		}
	}

	return report, nil
}

// plural is the tool messages' way of saying "1 sheet" and "2 sheets".
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func humanBytes(size int64) string {
	switch {
	case size >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(size)/(1<<20))
	case size >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(size)/(1<<10))
	default:
		return fmt.Sprintf("%d B", size)
	}
}
