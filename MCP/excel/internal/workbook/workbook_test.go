package workbook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testRoot(t *testing.T) *Root {
	t.Helper()
	root, err := NewRoot(t.TempDir())
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}
	return root
}

func TestResolveAddsTheWorkbookExtension(t *testing.T) {
	root := testRoot(t)

	path, err := root.ResolveWorkbook("budgets/q1")
	if err != nil {
		t.Fatalf("ResolveWorkbook: %v", err)
	}
	if !strings.HasSuffix(path, ".xlsx") {
		t.Errorf("got %s, want it to end in .xlsx", path)
	}
	if !strings.HasPrefix(path, root.Dir()) {
		t.Errorf("got %s, want it inside %s", path, root.Dir())
	}
	if got := root.Relative(path); got != "budgets/q1.xlsx" {
		t.Errorf("Relative = %q, want budgets/q1.xlsx", got)
	}
}

func TestResolveRefusesWhatThisServerDoesNotWrite(t *testing.T) {
	root := testRoot(t)

	// A macro workbook is refused rather than opened and saved without its
	// macros, which is a workbook quietly losing something.
	if _, err := root.ResolveWorkbook("report.xlsm"); err == nil {
		t.Error("an .xlsm should be refused: the library cannot keep its macros")
	}
	if _, err := root.ResolveWorkbook(""); err == nil {
		t.Error("an empty path should be refused")
	}
}

func TestResolveKeepsPathsInsideTheRoot(t *testing.T) {
	root := testRoot(t)

	if _, err := root.ResolveWorkbook("../outside.xlsx"); err == nil {
		t.Error("a path climbing out of the root should be refused")
	}
	if _, err := root.ResolveWorkbook(filepath.Join(t.TempDir(), "elsewhere.xlsx")); err == nil {
		t.Error("an absolute path outside the root should be refused")
	}

	inside := filepath.Join(root.Dir(), "sub", "ok.xlsx")
	path, err := root.ResolveWorkbook(inside)
	if err != nil {
		t.Fatalf("an absolute path inside the root should be accepted: %v", err)
	}
	if path != inside {
		t.Errorf("got %s, want %s", path, inside)
	}
}

func TestParseRangeOrdersTheCorners(t *testing.T) {
	start, end, err := ParseRange("C10:A1")
	if err != nil {
		t.Fatalf("ParseRange: %v", err)
	}
	if start != "A1" || end != "C10" {
		t.Errorf("got %s:%s, want A1:C10", start, end)
	}

	if _, _, err := ParseRange("banana"); err == nil {
		t.Error("a range that is not a range should be refused")
	}
	if _, _, err := ParseRange(""); err == nil {
		t.Error("an empty range should be refused")
	}
}

func TestCoveringAndShift(t *testing.T) {
	first, last, err := Covering("B2", 3, 2)
	if err != nil {
		t.Fatalf("Covering: %v", err)
	}
	if first != "B2" || last != "C4" {
		t.Errorf("got %s:%s, want B2:C4", first, last)
	}

	moved, err := Shift("B2", 1, 1)
	if err != nil {
		t.Fatalf("Shift: %v", err)
	}
	if moved != "C3" {
		t.Errorf("got %s, want C3", moved)
	}
}

func TestSheetNamesAreChecked(t *testing.T) {
	for _, good := range []string{"Summary", "Q1 2026", "a-b_c"} {
		if err := ValidSheetName(good); err != nil {
			t.Errorf("%q should be a valid sheet name: %v", good, err)
		}
	}
	for _, bad := range []string{"", "  ", "a/b", "a:b", "a*b", "a?b", "a[b]", strings.Repeat("x", 32)} {
		if err := ValidSheetName(bad); err == nil {
			t.Errorf("%q should have been refused", bad)
		}
	}
}

func TestCreateWriteAndOpenAgain(t *testing.T) {
	root := testRoot(t)
	path, err := root.ResolveWorkbook("report.xlsx")
	if err != nil {
		t.Fatalf("ResolveWorkbook: %v", err)
	}

	book, err := Create(path, []string{"Summary", "Data"}, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := book.File().GetSheetList(); len(got) != 2 || got[0] != "Summary" || got[1] != "Data" {
		t.Fatalf("sheets = %v, want [Summary Data]", got)
	}
	if _, last, err := book.WriteBlock("Summary", "A1", [][]any{{"Item", "Cost"}, {"Pen", 1.5}}); err != nil || last != "B2" {
		t.Fatalf("WriteBlock: %v (last %s)", err, last)
	}
	if err := book.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := book.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// The file is a real workbook on disk, and it opens again.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the workbook was not written: %v", err)
	}
	again, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = again.Close() }()

	block, err := again.ReadBlock("Summary", "A1", "B2", false, false)
	if err != nil {
		t.Fatalf("ReadBlock: %v", err)
	}
	if got := block.Values(); got[0][0] != "Item" || got[1][1] != "1.5" {
		t.Errorf("read back %v, want the values that were written", got)
	}
}

func TestCreateRefusesToStandOnAWorkbook(t *testing.T) {
	root := testRoot(t)
	path, _ := root.ResolveWorkbook("keep.xlsx")

	book, err := Create(path, nil, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := book.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	_ = book.Close()

	if _, err := Create(path, nil, false); err == nil {
		t.Error("creating over an existing workbook should be refused unless asked")
	}
	if _, err := Create(path, nil, true); err != nil {
		t.Errorf("overwrite true should be accepted: %v", err)
	}
}

func TestWriteBlockTreatsLeadingEqualsAsAFormula(t *testing.T) {
	root := testRoot(t)
	path, _ := root.ResolveWorkbook("formulas.xlsx")

	book, err := Create(path, nil, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = book.Close() }()

	if _, _, err := book.WriteBlock("Sheet1", "A1", [][]any{
		{"=SUM(B1:B2)"},
		{"'=not a formula"},
	}); err != nil {
		t.Fatalf("WriteBlock: %v", err)
	}

	formula, err := book.File().GetCellFormula("Sheet1", "A1")
	if err != nil || formula != "SUM(B1:B2)" {
		t.Errorf("A1 formula = %q, %v; want SUM(B1:B2)", formula, err)
	}
	if value, _ := book.File().GetCellValue("Sheet1", "A2"); value != "=not a formula" {
		t.Errorf("A2 = %q, want the text kept as text", value)
	}
}

func TestNumberFormatPresets(t *testing.T) {
	for name, want := range map[string]string{
		"currency": `"$"#,##0.00`,
		"percent":  "0.00%",
		"date":     "yyyy-mm-dd",
		"general":  "",
	} {
		got, err := NumberFormatCode(name)
		if err != nil {
			t.Fatalf("NumberFormatCode(%q): %v", name, err)
		}
		if got != want {
			t.Errorf("NumberFormatCode(%q) = %q, want %q", name, got, want)
		}
	}

	// A format code this server has never heard of is a code, not a mistake.
	if got, err := NumberFormatCode("#,##0.00 kg"); err != nil || got != "#,##0.00 kg" {
		t.Errorf("got %q, %v; want the code as written", got, err)
	}
}

func TestStyleFromRefusesAnEmptyStyle(t *testing.T) {
	if _, err := StyleFrom(nil); err == nil {
		t.Error("an empty style should be refused: a call that changes nothing is a mistake")
	}

	style, err := StyleFrom(map[string]any{
		"bold":          true,
		"fill_color":    "#DDEBF7",
		"number_format": "currency",
		"horizontal":    "center",
	})
	if err != nil {
		t.Fatalf("StyleFrom: %v", err)
	}
	if style.Font == nil || !style.Font.Bold {
		t.Error("the style should be bold")
	}
	if style.CustomNumFmt == nil || *style.CustomNumFmt != `"$"#,##0.00` {
		t.Errorf("CustomNumFmt = %v, want the currency code", style.CustomNumFmt)
	}
	if style.Fill.Color[0] != "DDEBF7" {
		t.Errorf("fill = %v, want DDEBF7", style.Fill.Color)
	}

	if _, err := StyleFrom(map[string]any{"font_color": "not-a-colour"}); err == nil {
		t.Error("a colour that is not a colour should be refused")
	}
}

func TestListFindsWorkbooksAndSkipsOtherFiles(t *testing.T) {
	root := testRoot(t)

	book, err := Create(filepath.Join(root.Dir(), "one.xlsx"), nil, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = book.Save()
	_ = book.Close()

	if err := os.WriteFile(filepath.Join(root.Dir(), "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := root.List("", true)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(found) != 1 || found[0].Path != "one.xlsx" {
		t.Errorf("got %v, want only one.xlsx", found)
	}
}
