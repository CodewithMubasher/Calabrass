package tools

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/protocol"
	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/workbook"
)

func (s *session) exportCSV() protocol.Tool {
	return protocol.Tool{
		Name: "export_csv",
		Description: "Write a sheet out as a CSV file, for something that reads CSV — a " +
			"script, a data tool, an import — rather than for a person. Numbers come out as " +
			"numbers (4.5, not $4.50) unless raw is set false, which writes the text Excel " +
			"shows instead. The workbook is not changed.",
		Schema: protocol.Object(map[string]any{
			"path":   protocol.String("The workbook to read from."),
			"sheet":  protocol.String("The sheet to write out. Leave out for the active sheet."),
			"output": protocol.String("Where to put the CSV. A .csv or .tsv name; a relative path lands beside this server's folder. The sheet's name is used when this is left out."),
			"raw":    protocol.Boolean("Write the stored numbers rather than the displayed text. On unless set false."),
		}, "path"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := s.path(args)
			if err != nil {
				return "", err
			}
			output, err := protocol.OptionalString(args, "output", "", maxPath)
			if err != nil {
				return "", err
			}
			raw, err := protocol.OptionalBool(args, "raw", true)
			if err != nil {
				return "", err
			}

			return s.read(path, func(book *workbook.Book) (string, error) {
				sheet, err := s.sheet(book, args)
				if err != nil {
					return "", err
				}
				used, hasData, err := book.UsedRange(sheet)
				if err != nil {
					return "", err
				}
				if !hasData {
					return "", fmt.Errorf("the sheet %q is empty: there is nothing to write out", sheet)
				}
				start, end, err := workbook.ParseRange(used)
				if err != nil {
					return "", err
				}
				block, err := book.ReadBlock(sheet, start, end, raw, false)
				if err != nil {
					return "", err
				}
				if block.Truncated {
					return "", fmt.Errorf("the sheet %q holds more than one CSV can take in a call: "+
						"read it in ranges, or ask for a smaller sheet", sheet)
				}

				target := output
				if target == "" {
					name := strings.TrimSuffix(book.Name(), workbook.Extension)
					if sheet != book.DefaultSheet() {
						name += "-" + sheet
					}
					target = filepath.Join(filepath.Dir(book.Path()), name)
				}
				destination, err := s.root.ResolveFile(target, ".csv", ".csv", ".tsv")
				if err != nil {
					return "", err
				}

				file, err := os.Create(destination)
				if err != nil {
					return "", fmt.Errorf("create %s: %w", destination, err)
				}
				defer func() { _ = file.Close() }()

				writer := csv.NewWriter(file)
				if strings.EqualFold(filepath.Ext(destination), ".tsv") {
					writer.Comma = '\t'
				}
				if err := writer.WriteAll(block.Values()); err != nil {
					return "", fmt.Errorf("write %s: %w", destination, err)
				}
				writer.Flush()
				if err := writer.Error(); err != nil {
					return "", fmt.Errorf("write %s: %w", destination, err)
				}

				rows := len(block.Values())
				return fmt.Sprintf("Wrote %d row%s from %s to %s.", rows, plural(rows), sheet, s.root.Relative(destination)), nil
			})
		},
	}
}
