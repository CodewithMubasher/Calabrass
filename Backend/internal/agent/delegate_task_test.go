package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// A model that misses workerId still delegates: the other names it reaches for
// mean the same thing, and the task alias does too.
func TestDelegateTaskArgsAcceptsTheNamesModelsUse(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		worker string
		task   string
	}{
		{
			name:   "the schema name",
			body:   `{"workerId":"builtin-frontend-developer","task":"Build it"}`,
			worker: "builtin-frontend-developer",
			task:   "Build it",
		},
		{
			name:   "worker",
			body:   `{"worker":"Frontend Developer","task":"Build it"}`,
			worker: "Frontend Developer",
			task:   "Build it",
		},
		{
			name:   "workerName",
			body:   `{"workerName":"Frontend Developer","task":"Build it"}`,
			worker: "Frontend Developer",
			task:   "Build it",
		},
		{
			name:   "agentId",
			body:   `{"agentId":"builtin-frontend-developer","task":"Build it"}`,
			worker: "builtin-frontend-developer",
			task:   "Build it",
		},
		{
			name:   "id",
			body:   `{"id":"builtin-frontend-developer","task":"Build it"}`,
			worker: "builtin-frontend-developer",
			task:   "Build it",
		},
		{
			name:   "instruction",
			body:   `{"workerId":"builtin-frontend-developer","instruction":"Build it"}`,
			worker: "builtin-frontend-developer",
			task:   "Build it",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var args DelegateTaskArgs
			if err := json.Unmarshal([]byte(tc.body), &args); err != nil {
				t.Fatalf("read the call: %v", err)
			}
			if args.WorkerID != tc.worker {
				t.Errorf("workerId = %q, want %q", args.WorkerID, tc.worker)
			}
			if args.Task != tc.task {
				t.Errorf("task = %q, want %q", args.Task, tc.task)
			}
		})
	}
}

// The ids sit in the tool description as well as the prompt, because the tool
// definition is what the model reads when it writes the call.
func TestDelegateTaskToolNamesTheWorkerIDs(t *testing.T) {
	workers := []WorkerSummary{
		{ID: "builtin-frontend-developer", Name: "Frontend Developer", Description: "Builds user interfaces."},
	}

	tool := DelegateTaskTool(workers, func(_ context.Context, _ DelegateTaskArgs) (string, error) {
		return "", nil
	})
	if !strings.Contains(tool.Description, "builtin-frontend-developer") {
		t.Fatalf("the tool description does not name the worker id:\n%s", tool.Description)
	}
	for _, want := range []string{`"workerId"`, `"task"`} {
		if !strings.Contains(string(tool.InputSchema), want) {
			t.Errorf("the input schema does not offer %s: %s", want, tool.InputSchema)
		}
	}
}

// The section a prompt renders must say to pass the id, name the id exactly,
// and show a call that does it.
func TestWorkerSectionShowsTheExactID(t *testing.T) {
	section := RenderTemplate("{{workers_section}}", PromptVars{
		Workers: []WorkerSummary{
			{ID: "builtin-frontend-developer", Name: "Frontend Developer", Description: "Builds user interfaces."},
		},
	})

	for _, want := range []string{
		"Available workers:",
		"id: builtin-frontend-developer",
		"Pass the id, not the name, as workerId",
		`"workerId": "builtin-frontend-developer"`,
	} {
		if !strings.Contains(section, want) {
			t.Errorf("the workers section does not contain %q:\n%s", want, section)
		}
	}
}
