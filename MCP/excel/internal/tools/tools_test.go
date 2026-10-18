package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/protocol"
	"github.com/CodewithMubasher/Calabrass/mcp/excel/internal/workbook"
)

// call runs one tool by name, the way a client would, and returns what the
// model would read.
func call(t *testing.T, registered []protocol.Tool, name string, args map[string]any) (string, error) {
	t.Helper()
	for _, tool := range registered {
		if tool.Name == name {
			return tool.Handler(context.Background(), args)
		}
	}
	t.Fatalf("no tool called %q", name)
	return "", nil
}

func mustCall(t *testing.T, registered []protocol.Tool, name string, args map[string]any) string {
	t.Helper()
	said, err := call(t, registered, name, args)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return said
}

func newTools(t *testing.T) ([]protocol.Tool, *workbook.Root) {
	t.Helper()
	root, err := workbook.NewRoot(t.TempDir())
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}
	return All(root), root
}

// TestAMonthOfBudgets is the whole promise in one run: make a workbook, fill it
// with a table, add formulas, style it, chart it, read it back and export it —
// the shape of what "make me a spreadsheet" actually means.
func TestAMonthOfBudgets(t *testing.T) {
	registered, root := newTools(t)

	made := mustCall(t, registered, "create_workbook", map[string]any{
		"path":    "budget.xlsx",
		"sheets":  []any{"Summary", "January"},
		"title":   "Household budget",
		"creator": "Calabrass",
	})
	if !strings.Contains(made, "Summary") || !strings.Contains(made, "January") {
		t.Fatalf("create_workbook said %q", made)
	}
	if _, err := os.Stat(filepath.Join(root.Dir(), "budget.xlsx")); err != nil {
		t.Fatalf("the workbook is not on disk: %v", err)
	}

	wrote := mustCall(t, registered, "write_table", map[string]any{
		"path":    "budget.xlsx",
		"sheet":   "January",
		"headers": []any{"Item", "Cost", "Count", "Total"},
		"rows": []any{
			[]any{"Coffee", 4.5, 20, "=B2*C2"},
			[]any{"Lunch", 12, 22, "=B3*C3"},
			[]any{"Books", 30, 2, "=B4*C4"},
		},
	})
	if !strings.Contains(wrote, "3 rows") {
		t.Errorf("write_table said %q", wrote)
	}

	mustCall(t, registered, "format_range", map[string]any{
		"path":  "budget.xlsx",
		"sheet": "January",
		"range": "B2:B4",
		"style": map[string]any{"number_format": "currency"},
	})

	read := mustCall(t, registered, "read_range", map[string]any{
		"path":     "budget.xlsx",
		"sheet":    "January",
		"range":    "A1:D4",
		"formulas": true,
	})
	if !strings.Contains(read, "Coffee") || !strings.Contains(read, "=B2*C2") {
		t.Errorf("read_range said %q, want the values and the formulas", read)
	}

	mustCall(t, registered, "freeze_panes", map[string]any{
		"path": "budget.xlsx", "sheet": "January", "cell": "A2",
	})
	mustCall(t, registered, "add_chart", map[string]any{
		"path": "budget.xlsx", "sheet": "January",
		"source": "A1:B4", "type": "column", "title": "January spending",
		"position": "F2",
	})

	found := mustCall(t, registered, "search_cells", map[string]any{
		"path": "budget.xlsx", "query": "coffee",
	})
	if !strings.Contains(found, "A2") {
		t.Errorf("search_cells said %q, want it to find the row", found)
	}

	info := mustCall(t, registered, "get_workbook_info", map[string]any{"path": "budget.xlsx"})
	for _, want := range []string{"January", "A2", "freezePanes", "Household budget"} {
		if !strings.Contains(info, want) {
			t.Errorf("get_workbook_info said %q, want it to mention %q", info, want)
		}
	}

	exported := mustCall(t, registered, "export_csv", map[string]any{
		"path": "budget.xlsx", "sheet": "January",
	})
	if !strings.Contains(exported, "budget-January.csv") {
		t.Errorf("export_csv said %q", exported)
	}
	csv, err := os.ReadFile(filepath.Join(root.Dir(), "budget-January.csv"))
	if err != nil {
		t.Fatalf("the CSV was not written: %v", err)
	}
	if !strings.Contains(string(csv), "Coffee,4.5,20,90") {
		t.Errorf("CSV = %q, want the table in it with the formula worked out", csv)
	}

	list := mustCall(t, registered, "list_workbooks", map[string]any{})
	if !strings.Contains(list, "budget.xlsx") {
		t.Errorf("list_workbooks said %q", list)
	}
}

func TestSheetsCanBeAddedRenamedCopiedAndHidden(t *testing.T) {
	registered, _ := newTools(t)
	mustCall(t, registered, "create_workbook", map[string]any{"path": "book.xlsx"})

	mustCall(t, registered, "write_range", map[string]any{
		"path": "book.xlsx", "sheet": "Sheet1", "values": []any{[]any{"keep"}},
	})
	mustCall(t, registered, "add_sheet", map[string]any{"path": "book.xlsx", "name": "Notes"})
	mustCall(t, registered, "rename_sheet", map[string]any{"path": "book.xlsx", "sheet": "Notes", "new_name": "Ideas"})
	mustCall(t, registered, "copy_sheet", map[string]any{"path": "book.xlsx", "source": "Sheet1", "name": "Copy"})

	read := mustCall(t, registered, "read_range", map[string]any{"path": "book.xlsx", "sheet": "Copy", "range": "A1"})
	if !strings.Contains(read, "keep") {
		t.Errorf("the copy is missing its values: %q", read)
	}

	mustCall(t, registered, "set_sheet_visibility", map[string]any{"path": "book.xlsx", "sheet": "Ideas", "state": "hidden"})

	info := mustCall(t, registered, "get_workbook_info", map[string]any{"path": "book.xlsx"})
	if !strings.Contains(info, "hidden") {
		t.Errorf("the sheet should be reported as hidden: %q", info)
	}

	mustCall(t, registered, "delete_sheet", map[string]any{"path": "book.xlsx", "sheet": "Copy"})
	if _, err := call(t, registered, "delete_sheet", map[string]any{"path": "book.xlsx", "sheet": "Copy"}); err == nil {
		t.Error("deleting a sheet that is not there should be refused")
	}
}

func TestRowsColumnsAndClearing(t *testing.T) {
	registered, _ := newTools(t)
	mustCall(t, registered, "create_workbook", map[string]any{"path": "grid.xlsx"})
	mustCall(t, registered, "write_range", map[string]any{
		"path": "grid.xlsx",
		"values": []any{
			[]any{"a", 1},
			[]any{"b", 2},
			[]any{"c", 3},
		},
	})

	mustCall(t, registered, "insert_rows", map[string]any{"path": "grid.xlsx", "row": 2, "count": 2})
	mustCall(t, registered, "insert_columns", map[string]any{"path": "grid.xlsx", "column": "A"})
	read := mustCall(t, registered, "read_range", map[string]any{"path": "grid.xlsx", "range": "B4:C6"})
	if !strings.Contains(read, "a") {
		t.Errorf("the row should have moved down and right: %q", read)
	}

	mustCall(t, registered, "clear_range", map[string]any{"path": "grid.xlsx", "range": "B4:C4", "what": "contents"})
	mustCall(t, registered, "delete_rows", map[string]any{"path": "grid.xlsx", "row": 1, "count": 3})
	mustCall(t, registered, "delete_columns", map[string]any{"path": "grid.xlsx", "column": "A", "count": 1})

	read = mustCall(t, registered, "read_range", map[string]any{"path": "grid.xlsx", "range": "A1:B2"})
	if !strings.Contains(read, "c") {
		t.Errorf("after the deletes the last row should be left: %q", read)
	}
}

func TestValidationAndConditionalFormatting(t *testing.T) {
	registered, _ := newTools(t)
	mustCall(t, registered, "create_workbook", map[string]any{"path": "rules.xlsx"})
	mustCall(t, registered, "write_table", map[string]any{
		"path":    "rules.xlsx",
		"headers": []any{"Task", "Owner", "Days"},
		"rows": []any{
			[]any{"Draft", "Ali", 3},
			[]any{"Review", "Sam", 12},
		},
	})

	mustCall(t, registered, "data_validation", map[string]any{
		"path": "rules.xlsx", "range": "B2:B200", "kind": "list",
		"values": []any{"Ali", "Sam", "Ines"},
		"title":  "Pick an owner", "message": "Choose a name from the list.",
	})
	mustCall(t, registered, "conditional_format", map[string]any{
		"path": "rules.xlsx", "range": "C2:C200", "rule": "greater_than",
		"value": "7", "fill_color": "#FFC7CE", "font_color": "#9C0006",
	})
	// A colour scale and a data bar carry their own colours rather than a style,
	// and are the two rules that used to fail because they sent no criteria.
	mustCall(t, registered, "conditional_format", map[string]any{
		"path": "rules.xlsx", "range": "D2:D200", "rule": "color_scale",
	})
	mustCall(t, registered, "conditional_format", map[string]any{
		"path": "rules.xlsx", "range": "E2:E200", "rule": "data_bar",
		"bar_color": "#638EC6",
	})
	// Several rules on one range is the normal case and must not clobber the
	// others already there.
	mustCall(t, registered, "conditional_format", map[string]any{
		"path": "rules.xlsx", "range": "C2:C200", "rule": "less_than",
		"value": "2", "bold": true,
	})

	info := mustCall(t, registered, "get_workbook_info", map[string]any{"path": "rules.xlsx"})
	if !strings.Contains(info, `"dataValidations": 1`) {
		t.Errorf("the validation should be reported: %q", info)
	}
	if !strings.Contains(info, `"conditionalFormats": 3`) {
		t.Errorf("all three conditional format ranges should be reported: %q", info)
	}
}

func TestReadsAndWritesAreRefusedWhenTheyShouldBe(t *testing.T) {
	registered, root := newTools(t)

	// A workbook that is not there is a next step, not a dead end.
	if _, err := call(t, registered, "read_range", map[string]any{"path": "missing.xlsx"}); err == nil {
		t.Error("reading a workbook that is not there should be refused")
	}

	mustCall(t, registered, "create_workbook", map[string]any{"path": "hold.xlsx"})

	for name, args := range map[string]map[string]any{
		"a path outside the root":   {"path": filepath.Join(t.TempDir(), "outside.xlsx"), "values": []any{[]any{"x"}}},
		"a macro workbook":          {"path": "macro.xlsm", "values": []any{[]any{"x"}}},
		"a sheet that is not there": {"path": "hold.xlsx", "sheet": "Nope", "range": "A1", "values": []any{[]any{"x"}}},
		"a bad sheet name":          {"path": "bad.xlsx", "sheets": []any{"a/b"}},
		"an empty grid":             {"path": "hold.xlsx", "values": []any{}},
	} {
		tool := "write_range"
		if strings.Contains(name, "sheet name") {
			tool = "create_workbook"
		}
		if _, err := call(t, registered, tool, args); err == nil {
			t.Errorf("%s should have been refused", name)
		}
	}

	// The file that exists is still there and untouched.
	if _, err := os.Stat(filepath.Join(root.Dir(), "hold.xlsx")); err != nil {
		t.Errorf("the workbook should still be on disk: %v", err)
	}
}

// TestEveryToolIsDescribed: a client shows the model the name, the description
// and the schema, so a tool missing any of them is one the model cannot use.
func TestEveryToolIsDescribed(t *testing.T) {
	registered, _ := newTools(t)
	if len(registered) < 20 {
		t.Fatalf("got %d tools, want a full set", len(registered))
	}

	for _, tool := range registered {
		if tool.Name == "" {
			t.Error("a tool without a name")
		}
		if len(tool.Description) < 40 {
			t.Errorf("%s needs a description the model can act on", tool.Name)
		}
		if tool.Handler == nil {
			t.Errorf("%s has nothing to run", tool.Name)
		}
		if tool.Schema["type"] != "object" {
			t.Errorf("%s: inputSchema.type = %v, want object", tool.Name, tool.Schema["type"])
		}
		properties, ok := tool.Schema["properties"].(map[string]any)
		if !ok || len(properties) == 0 {
			t.Errorf("%s: inputSchema has no properties", tool.Name)
		}
		for _, required := range requiredNames(tool) {
			if _, ok := properties[required]; !ok {
				t.Errorf("%s: %q is required but not described in properties", tool.Name, required)
			}
		}
	}
}

func requiredNames(tool protocol.Tool) []string {
	raw, _ := tool.Schema["required"].([]string)
	return raw
}

// TestInstructionsSayWhereFilesLand: a model that does not know where a file
// will appear cannot tell the person it is helping.
func TestInstructionsSayWhereFilesLand(t *testing.T) {
	root, err := workbook.NewRoot(t.TempDir())
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}
	said := Instructions(root)
	if !strings.Contains(said, root.Dir()) {
		t.Errorf("Instructions = %q, want it to name the folder", said)
	}
	if !strings.Contains(said, "xlsx") {
		t.Errorf("Instructions = %q, want it to say what the tools write", said)
	}
}
