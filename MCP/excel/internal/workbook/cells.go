package workbook

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// This file is where a value a model sent becomes a cell, and a cell becomes
// something a model can read.

// maxReadCells caps how much of a sheet one read returns. A used range can be
// millions of cells, and the answer to "what is in this workbook" must not be
// an answer nobody can carry.
const maxReadCells = 20000

// WriteBlock writes a grid of values starting at a cell, and returns the range
// it filled. Text that starts with "=" is written as a formula, the way typing
// it into a spreadsheet would; text that starts with an apostrophe is written
// literally, with the apostrophe removed, which is how a spreadsheet is told to
// keep an "=" as text.
func (b *Book) WriteBlock(sheet, start string, rows [][]any) (string, string, error) {
	if len(rows) == 0 {
		return "", "", fmt.Errorf("there are no rows to write")
	}
	cols := 0
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	first, last, err := Covering(start, len(rows), cols)
	if err != nil {
		return "", "", err
	}
	sx, sy, _, _, err := Corners(first, last)
	if err != nil {
		return "", "", err
	}

	for r, row := range rows {
		for c, cell := range row {
			ref, err := excelize.CoordinatesToCellName(sx+c, sy+r)
			if err != nil {
				return "", "", err
			}
			if err := b.setCell(sheet, ref, cell); err != nil {
				return "", "", err
			}
		}
	}
	// The sheet's own record of how far it reaches has to learn about what was
	// just written, or the next read of "everything" would miss it.
	b.grow(sheet, first, last)
	return first, last, nil
}

// setCell writes one value. Null and the empty string are left alone rather than
// written as empty cells, so a blank in the middle of a table does not become a
// cell that holds nothing.
func (b *Book) setCell(sheet, ref string, value any) error {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		text := typed
		switch {
		case strings.HasPrefix(text, "'"):
			text = strings.TrimPrefix(text, "'")
		case strings.HasPrefix(text, "="):
			formula := strings.TrimSpace(strings.TrimPrefix(text, "="))
			if formula == "" {
				return nil
			}
			if err := b.file.SetCellFormula(sheet, ref, formula); err != nil {
				return fmt.Errorf("write the formula in %s: %w", ref, err)
			}
			return nil
		}
		if text == "" {
			return nil
		}
		if err := b.file.SetCellValue(sheet, ref, text); err != nil {
			return fmt.Errorf("write %s: %w", ref, err)
		}
		return nil
	case bool:
		if err := b.file.SetCellValue(sheet, ref, typed); err != nil {
			return fmt.Errorf("write %s: %w", ref, err)
		}
		return nil
	case float64:
		if err := b.file.SetCellValue(sheet, ref, typed); err != nil {
			return fmt.Errorf("write %s: %w", ref, err)
		}
		return nil
	case int:
		if err := b.file.SetCellValue(sheet, ref, float64(typed)); err != nil {
			return fmt.Errorf("write %s: %w", ref, err)
		}
		return nil
	case int64:
		if err := b.file.SetCellValue(sheet, ref, float64(typed)); err != nil {
			return fmt.Errorf("write %s: %w", ref, err)
		}
		return nil
	case json.Number:
		if number, err := typed.Int64(); err == nil {
			if err := b.file.SetCellValue(sheet, ref, float64(number)); err != nil {
				return fmt.Errorf("write %s: %w", ref, err)
			}
			return nil
		}
		number, err := typed.Float64()
		if err != nil {
			return fmt.Errorf("%s cannot be a number: %v", ref, typed)
		}
		if err := b.file.SetCellValue(sheet, ref, number); err != nil {
			return fmt.Errorf("write %s: %w", ref, err)
		}
		return nil
	default:
		return fmt.Errorf("%s cannot hold a %T", ref, value)
	}
}

// Cell is one cell as the model should read it.
type Cell struct {
	Ref     string `json:"ref"`
	Value   string `json:"value"`
	Formula string `json:"formula,omitempty"`
}

// Block is a square of cells, in rows.
type Block struct {
	Sheet     string   `json:"sheet"`
	Range     string   `json:"range"`
	Rows      [][]Cell `json:"rows"`
	Truncated bool     `json:"truncated,omitempty"`
}

// ReadBlock reads a range. With raw set, a cell answers with the number it
// stores rather than the text it shows — the date serial rather than the date —
// which is what a caller that means to do arithmetic on it needs.
func (b *Book) ReadBlock(sheet, start, end string, raw bool, wantFormulas bool) (Block, error) {
	sx, sy, ex, ey, err := Corners(start, end)
	if err != nil {
		return Block{}, err
	}

	options := []excelize.Options{{RawCellValue: raw}}
	rows := make([][]Cell, 0, ey-sy+1)
	total := 0
	truncated := false

	for y := sy; y <= ey; y++ {
		row := make([]Cell, 0, ex-sx+1)
		for x := sx; x <= ex; x++ {
			if total >= maxReadCells {
				truncated = true
				break
			}
			ref, err := excelize.CoordinatesToCellName(x, y)
			if err != nil {
				return Block{}, err
			}
			value, err := b.file.GetCellValue(sheet, ref, options...)
			if err != nil {
				return Block{}, fmt.Errorf("read %s!%s: %w", sheet, ref, err)
			}

			cell := Cell{Ref: ref, Value: value}
			// A formula has no value until something works it out, and a workbook
			// made here has never been opened by Excel. The result is calculated
			// where the library's engine can, so a model that just wrote
			// "=B2*C2" can read back what it makes — and can tell when it makes
			// nothing, because a formula the engine cannot evaluate stays empty.
			if wantFormulas || value == "" {
				if formula, err := b.file.GetCellFormula(sheet, ref); err == nil && formula != "" {
					if wantFormulas {
						cell.Formula = "=" + formula
					}
					if value == "" {
						if computed, err := b.file.CalcCellValue(sheet, ref, options...); err == nil {
							cell.Value = computed
						}
					}
				}
			}
			row = append(row, cell)
			total++
		}
		rows = append(rows, row)
		if truncated {
			break
		}
	}

	return Block{
		Sheet:     sheet,
		Range:     start + ":" + end,
		Rows:      rows,
		Truncated: truncated,
	}, nil
}

// TrimBlock drops the empty cells at the end of every row, so a read of a range
// wider than the data does not come back as mostly blanks.
func TrimBlock(block Block) Block {
	trimmed := make([][]Cell, 0, len(block.Rows))
	for _, row := range block.Rows {
		last := len(row)
		for last > 0 && row[last-1].Value == "" && row[last-1].Formula == "" {
			last--
		}
		trimmed = append(trimmed, row[:last])
	}
	block.Rows = trimmed
	return block
}

// Values is the block as rows of text — what a model reading a sheet wants,
// with the cell addresses left out because the range already says where the
// first one is.
func (b Block) Values() [][]string {
	rows := make([][]string, 0, len(b.Rows))
	for _, row := range b.Rows {
		values := make([]string, 0, len(row))
		for _, cell := range row {
			values = append(values, cell.Value)
		}
		rows = append(rows, values)
	}
	return rows
}

// Formulas is the cell addresses in the block that hold a formula, and the
// formula each holds. It is empty unless the read asked for formulas.
func (b Block) Formulas() map[string]string {
	formulas := map[string]string{}
	for _, row := range b.Rows {
		for _, cell := range row {
			if cell.Formula != "" {
				formulas[cell.Ref] = cell.Formula
			}
		}
	}
	return formulas
}

// Autofit widths the columns a block of values sits in, from the values
// themselves — reading them back would be a second pass over the sheet for
// something already in hand.
func (b *Book) Autofit(sheet, start string, rows [][]any) {
	x, _, err := excelize.CellNameToCoordinates(strings.ToUpper(strings.TrimSpace(start)))
	if err != nil {
		return
	}

	widths := map[int]float64{}
	for _, row := range rows {
		for c, cell := range row {
			text := displayText(cell)
			width := float64(displayWidth(text)) + 2
			// A number that will be formatted needs room for separators; the
			// style is applied after this, so the extra is a small guess.
			if width < 8 {
				width = 8
			}
			if width > 60 {
				width = 60
			}
			column := x + c
			if width > widths[column] {
				widths[column] = width
			}
		}
	}

	for column, width := range widths {
		name := ColumnLetter(column)
		if name == "" {
			continue
		}
		_ = b.file.SetColWidth(sheet, name, name, width)
	}
}

func displayText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimPrefix(typed, "'")
	case bool:
		if typed {
			return "TRUE"
		}
		return "FALSE"
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return fmt.Sprint(value)
	}
}

// displayWidth counts a rune that is drawn twice as wide — the CJK block — as
// two, so a sheet of Chinese text is not fitted into half the room it needs.
func displayWidth(text string) int {
	width := 0
	for _, r := range text {
		switch {
		case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
			r >= 0x2E80 && r <= 0xA4CF, // CJK radicals to Yi
			r >= 0xAC00 && r <= 0xD7A3, // Hangul syllables
			r >= 0xF900 && r <= 0xFAFF, // CJK compatibility ideographs
			r >= 0xFE30 && r <= 0xFE6F, // CJK compatibility forms
			r >= 0xFF00 && r <= 0xFF60, // full-width forms
			r >= 0xFFE0 && r <= 0xFFE6:
			width += 2
		default:
			width++
		}
	}
	return width
}
