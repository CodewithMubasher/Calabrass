package workbook

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// This file is the small language of spreadsheet addresses: "A1", "A1:C10",
// "'Q1 report'!$B$2:$D$9". The tools read the first, write formulas in the
// third with Absolute and Quote, and turn a block of values into the second with
// Covering.

// NormalizeCell checks one cell address and returns it in the form the library
// wants: upper case, with no dollar signs.
func NormalizeCell(cell string) (string, error) {
	trimmed := strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(cell, "$", "")))
	if trimmed == "" {
		return "", fmt.Errorf("a cell address is required, such as A1")
	}
	if _, _, err := excelize.CellNameToCoordinates(trimmed); err != nil {
		return "", fmt.Errorf("%q is not a cell address: give one like A1 or C7", cell)
	}
	return trimmed, nil
}

// ParseRange reads a range: one cell, or two joined by a colon. A bare column
// ("A") or row ("2") is not a range here — a caller that means a column says
// "A1:A1048576" or sets the column width — because guessing whether "A" meant
// the column or the cell would be a guess.
func ParseRange(ref string) (start, end string, err error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", "", fmt.Errorf("a range is required, such as A1:C10")
	}
	parts := strings.SplitN(trimmed, ":", 2)
	start, err = NormalizeCell(parts[0])
	if err != nil {
		return "", "", err
	}
	if len(parts) == 1 {
		return start, start, nil
	}
	end, err = NormalizeCell(parts[1])
	if err != nil {
		return "", "", err
	}
	// The library reads a range as top-left to bottom-right, so a caller that
	// wrote it the other way round is put right rather than refused.
	sx, sy, _ := excelize.CellNameToCoordinates(start)
	ex, ey, _ := excelize.CellNameToCoordinates(end)
	if ex < sx || ey < sy {
		start, end = end, start
	}
	return start, end, nil
}

// Covering is the range a block of values fills, given its top-left cell.
func Covering(start string, rows, cols int) (string, string, error) {
	cell, err := NormalizeCell(start)
	if err != nil {
		return "", "", err
	}
	x, y, err := excelize.CellNameToCoordinates(cell)
	if err != nil {
		return "", "", err
	}
	if rows < 1 || cols < 1 {
		return "", "", fmt.Errorf("a block of values must have at least one row and one column")
	}
	end, err := excelize.CoordinatesToCellName(x+cols-1, y+rows-1)
	if err != nil {
		return "", "", err
	}
	return cell, end, nil
}

// Shift moves a cell down and right by a number of rows and columns.
func Shift(cell string, rows, cols int) (string, error) {
	x, y, err := excelize.CellNameToCoordinates(strings.ToUpper(strings.TrimSpace(cell)))
	if err != nil {
		return "", fmt.Errorf("%q is not a cell address", cell)
	}
	x, y = x+cols, y+rows
	if x < 1 || y < 1 {
		return "", fmt.Errorf("that would be above or left of the first cell")
	}
	return excelize.CoordinatesToCellName(x, y)
}

// Corners decomposes a range into its four coordinates.
func Corners(start, end string) (x1, y1, x2, y2 int, err error) {
	if x1, y1, err = excelize.CellNameToCoordinates(start); err != nil {
		return 0, 0, 0, 0, err
	}
	if x2, y2, err = excelize.CellNameToCoordinates(end); err != nil {
		return 0, 0, 0, 0, err
	}
	return x1, y1, x2, y2, nil
}

// ColumnLetter is the name of a column by number: 1 is A.
func ColumnLetter(number int) string {
	name, err := excelize.ColumnNumberToName(number)
	if err != nil {
		return ""
	}
	return name
}

// ColumnNumber is the number of a column by name: A is 1.
func ColumnNumber(name string) (int, error) {
	number, err := excelize.ColumnNameToNumber(strings.ToUpper(strings.TrimSpace(name)))
	if err != nil {
		return 0, fmt.Errorf("%q is not a column letter: give one like A or AB", name)
	}
	return number, nil
}

// LastRow is the bottom of a range, and LastColumn its right edge, for a message
// that has to say where the data ends.
func LastRow(end string) int {
	_, y, err := excelize.CellNameToCoordinates(end)
	if err != nil {
		return 0
	}
	return y
}

// Quote wraps a sheet name for use inside a formula: a name with a space or a
// digit in it needs single quotes, and a quote inside the name is doubled.
func Quote(sheet string) string {
	return "'" + strings.ReplaceAll(sheet, "'", "''") + "'"
}

// Absolute renders a range as an absolute reference on a sheet, the way a chart
// series or a defined name wants it: 'Sheet name'!$A$1:$B$10.
func Absolute(sheet, start, end string) string {
	return Quote(sheet) + "!$" + ColumnOf(start) + "$" + RowOf(start) + ":$" + ColumnOf(end) + "$" + RowOf(end)
}

// ColumnOf is the letters of a cell address: "C" for C7.
func ColumnOf(cell string) string {
	letters := strings.TrimRight(strings.ToUpper(strings.TrimSpace(cell)), "0123456789")
	return letters
}

// RowOf is the digits of a cell address: "7" for C7.
func RowOf(cell string) string {
	return strings.TrimLeft(strings.ToUpper(strings.TrimSpace(cell)), "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
}

// UsedRange is the block a sheet actually has content in. Excel's own files
// carry a record of this in the sheet's dimension, which is used when it is
// there; a workbook made by this server has no such record, so the cells are
// read to find where the data reaches.
func (b *Book) UsedRange(sheet string) (string, bool, error) {
	if recorded, ok, err := b.recordedRange(sheet); err != nil {
		return "", false, err
	} else if ok {
		return recorded, true, nil
	}
	return b.scanRange(sheet)
}

// recordedRange reads the range the sheet's dimension element holds. A
// dimension of A1 is ambiguous — it is what an empty sheet reports and what a
// sheet holding one cell reports — so the cell itself is checked.
func (b *Book) recordedRange(sheet string) (string, bool, error) {
	dimension, err := b.file.GetSheetDimension(sheet)
	if err != nil {
		return "", false, fmt.Errorf("read the size of %q: %w", sheet, err)
	}
	dimension = strings.TrimSpace(dimension)

	if dimension == "" || dimension == "A1" {
		if value, err := b.file.GetCellValue(sheet, "A1"); err == nil && strings.TrimSpace(value) != "" {
			return "A1", true, nil
		}
		if formula, err := b.file.GetCellFormula(sheet, "A1"); err == nil && formula != "" {
			return "A1", true, nil
		}
		return "", false, nil
	}
	if !strings.Contains(dimension, ":") {
		return dimension, true, nil
	}
	start, end, err := ParseRange(dimension)
	if err != nil {
		return dimension, true, nil
	}
	return start + ":" + end, true, nil
}

// scanRange reads the sheet's rows to find where the data reaches. It is the
// honest answer when there is no dimension record to trust, and is deliberately
// separate from UsedRange so that a caller that must not be fooled by a stale
// record can ask for it.
func (b *Book) scanRange(sheet string) (string, bool, error) {
	rows, err := b.file.GetRows(sheet)
	if err != nil {
		return "", false, fmt.Errorf("read %q: %w", sheet, err)
	}

	last, widest := 0, 0
	for index, row := range rows {
		if len(row) == 0 {
			continue
		}
		last = index + 1
		if len(row) > widest {
			widest = len(row)
		}
	}
	if last == 0 || widest == 0 {
		return "A1", false, nil
	}
	end, err := excelize.CoordinatesToCellName(widest, last)
	if err != nil {
		return "", false, err
	}
	if widest == 1 && last == 1 {
		return "A1", true, nil
	}
	return "A1:" + end, true, nil
}

// Recount puts the sheet's dimension record back in step with what it holds. It
// is what a delete or a clear owes the next caller: a record that still claims
// rows that are gone would have rows appended after a gap of nothing.
func (b *Book) Recount(sheet string) {
	used, hasData, err := b.scanRange(sheet)
	if err != nil {
		return
	}
	if !hasData {
		_ = b.file.SetSheetDimension(sheet, "")
		return
	}
	_ = b.file.SetSheetDimension(sheet, used)
}

// grow records that the sheet now reaches at least as far as the range given,
// so the next UsedRange does not answer with the sheet's older, smaller shape.
func (b *Book) grow(sheet, start, end string) {
	first, last := start, end

	if current, hasData, err := b.recordedRange(sheet); err == nil && hasData {
		if recordedStart, recordedEnd, err := ParseRange(current); err == nil {
			cx1, cy1, cx2, cy2, firstErr := Corners(recordedStart, recordedEnd)
			ex1, ey1, ex2, ey2, secondErr := Corners(start, end)
			if firstErr == nil && secondErr == nil {
				if grown, err := excelize.CoordinatesToCellName(min(cx1, ex1), min(cy1, ey1)); err == nil {
					first = grown
				}
				if grown, err := excelize.CoordinatesToCellName(max(cx2, ex2), max(cy2, ey2)); err == nil {
					last = grown
				}
			}
		}
	}
	_ = b.file.SetSheetDimension(sheet, first+":"+last)
}

// DefaultSheet is the sheet a call acts on when it did not name one: the active
// sheet, falling back to the first.
func (b *Book) DefaultSheet() string {
	if index := b.file.GetActiveSheetIndex(); index >= 0 {
		if name := b.file.GetSheetName(index); name != "" {
			return name
		}
	}
	if names := b.file.GetSheetList(); len(names) > 0 {
		return names[0]
	}
	return ""
}

// Sheet resolves the sheet a call names, or the default one, and refuses a name
// that is not there — naming the sheets that are is what turns a dead end into a
// next call. Sheet names are matched the way Excel matches them: without regard
// to case.
func (b *Book) Sheet(named string) (string, error) {
	name := strings.TrimSpace(named)
	names := b.file.GetSheetList()
	if name == "" {
		if fallback := b.DefaultSheet(); fallback != "" {
			return fallback, nil
		}
		return "", fmt.Errorf("the workbook has no sheets")
	}
	for _, existing := range names {
		if strings.EqualFold(existing, name) {
			return existing, nil
		}
	}
	return "", fmt.Errorf("there is no sheet called %q: this workbook has %s", name, joinNames(names))
}

// SheetIndex is the position of a named sheet, and whether it is there at all.
// The library's own GetSheetIndex answers -1 without an error for a sheet that
// does not exist, which is the shape of answer that turns into a bug when it is
// read as "no error, so it must be there".
func (b *Book) SheetIndex(name string) (int, bool) {
	index, _ := b.file.GetSheetIndex(strings.TrimSpace(name))
	return index, index >= 0
}

func joinNames(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, fmt.Sprintf("%q", name))
	}
	return strings.Join(quoted, ", ")
}
