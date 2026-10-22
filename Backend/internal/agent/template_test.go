package agent

import (
	"strings"
	"testing"
)

// TestRenderTemplate checks that the template renderer fills placeholders
// correctly and removes empty sections entirely.
func TestRenderTemplate(t *testing.T) {
	template := `You are the Orchestrator.

Workspace: {{workspace}}
Date: {{date}}

{{tools_section}}

{{workers_section}}

Style: direct.
`

	vars := PromptVars{
		Workspace: "/home/user/project",
		Date:      "2026-10-22 12:00:00",
		Tools: []ToolSummary{
			{Name: "read_file", Description: "Reads a file from the project"},
			{Name: "write_file", Description: "Writes a file to the project"},
		},
		Workers: []WorkerSummary{
			{Name: "Frontend Worker", Description: "Builds React interfaces"},
			{Name: "Backend Worker", Description: "Builds APIs and persistence"},
		},
	}

	out := RenderTemplate(template, vars)

	// Check placeholders were replaced
	if !strings.Contains(out, "Orchestrator") {
		t.Errorf("role placeholder not replaced: %s", out)
	}
	if !strings.Contains(out, "/home/user/project") {
		t.Errorf("workspace placeholder not replaced: %s", out)
	}
	if !strings.Contains(out, "2026-10-22 12:00:00") {
		t.Errorf("date placeholder not replaced: %s", out)
	}

	// Check tools section rendered
	if !strings.Contains(out, "Available tools:") {
		t.Errorf("tools section missing: %s", out)
	}
	if !strings.Contains(out, "read_file") {
		t.Errorf("tool name not in output: %s", out)
	}
	if !strings.Contains(out, "Writes a file") {
		t.Errorf("tool description not in output: %s", out)
	}

	// Check workers section rendered
	if !strings.Contains(out, "Available workers:") {
		t.Errorf("workers section missing: %s", out)
	}
	if !strings.Contains(out, "1. Frontend Worker") {
		t.Errorf("worker 1 not in output: %s", out)
	}
	if !strings.Contains(out, "Builds React interfaces") {
		t.Errorf("worker 1 description not in output: %s", out)
	}
	if !strings.Contains(out, "2. Backend Worker") {
		t.Errorf("worker 2 not in output: %s", out)
	}
	if !strings.Contains(out, "Builds APIs") {
		t.Errorf("worker 2 description not in output: %s", out)
	}
}

// TestRenderTemplateWithNoWorkers checks that the workers section is
// completely removed when there are no workers.
func TestRenderTemplateWithNoWorkers(t *testing.T) {
	template := `You are the Orchestrator.

{{workers_section}}

End.
`

	vars := PromptVars{
		Workspace: "/home/user/project",
		Date:      "2026-10-22",
		Tools:     []ToolSummary{{Name: "read_file", Description: "Reads a file"}},
		Workers:   []WorkerSummary{},
	}

	out := RenderTemplate(template, vars)

	// The workers section should be completely gone
	if strings.Contains(out, "Available workers:") {
		t.Errorf("workers section should be removed when empty: %s", out)
	}
	if strings.Contains(out, "{{workers_section}}") {
		t.Errorf("placeholder not removed: %s", out)
	}
}

// TestRenderTemplateWithNoTools checks that the tools section is
// completely removed when there are no tools.
func TestRenderTemplateWithNoTools(t *testing.T) {
	template := `You are the Orchestrator.

{{tools_section}}

End.
`

	vars := PromptVars{
		Workspace: "/home/user/project",
		Date:      "2026-10-22",
		Tools:     []ToolSummary{},
		Workers:   []WorkerSummary{{Name: "Worker", Description: "Does work"}},
	}

	out := RenderTemplate(template, vars)

	if strings.Contains(out, "Available tools:") {
		t.Errorf("tools section should be removed when empty: %s", out)
	}
}

// TestFirstSentence checks the description truncation logic.
func TestFirstSentence(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Reads a file.", "Reads a file."},
		{"Reads a file. Writes it back.", "Reads a file."},
		{"No period here", "No period here"},
		{"", ""},
		{"   spaced   ", "spaced"},
	}
	for _, tc := range cases {
		if got := firstSentence(tc.in); got != tc.want {
			t.Errorf("firstSentence(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}