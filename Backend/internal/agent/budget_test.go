package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/zendev-sh/goai"
)

type sampleArgs struct {
	Path string `json:"path" jsonschema:"description=The path to read"`
}

// The middleware once rebuilt every tool with goai.NewTool, which replaced the
// real input schema with one generated from map[string]any. A model then had no
// parameter names to call the tool with. The definition must survive wrapping.
func TestBudgetMiddlewarePreservesToolDefinition(t *testing.T) {
	original := goai.NewTool("read_file", "read a file",
		func(ctx context.Context, args sampleArgs) (string, error) {
			return "read " + args.Path, nil
		})

	wrapped := BudgetMiddleware(original, NewRunState(DefaultBudget(ModeChat)))
	if wrapped.Name != original.Name {
		t.Fatalf("name = %q, want %q", wrapped.Name, original.Name)
	}
	if wrapped.Description != original.Description {
		t.Fatalf("description = %q, want %q", wrapped.Description, original.Description)
	}
	if string(wrapped.InputSchema) != string(original.InputSchema) {
		t.Fatalf("input schema changed:\n got %s\nwant %s", wrapped.InputSchema, original.InputSchema)
	}

	out, err := wrapped.Execute(context.Background(), json.RawMessage(`{"path":"a.go"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "read a.go" {
		t.Fatalf("output = %q, want the typed call to decode", out)
	}
}

func allowed(t *testing.T, rs *RunState, tool string, args map[string]any) {
	t.Helper()
	ok, reason := rs.BeforeToolCall(tool, args)
	if !ok {
		t.Fatalf("%s was refused: %s", tool, reason)
	}
}

func TestRunStateRefusesRepeatedIdenticalFailureAfterOneRetry(t *testing.T) {
	rs := NewRunState(DefaultBudget(ModeChat))
	args := map[string]any{"path": "missing.go"}
	failure := errors.New("no such file: missing.go")

	for attempt := 0; attempt < 2; attempt++ {
		allowed(t, rs, "read_file", args)
		rs.RecordToolCall("read_file", args, failure)
	}

	ok, reason := rs.BeforeToolCall("read_file", args)
	if ok {
		t.Fatal("the third identical failing call should be refused")
	}
	if !strings.Contains(reason, "failed 2 times") {
		t.Fatalf("reason = %q, want the failure count", reason)
	}
}

func TestRunStateAllowsRetryAfterSuccessfulMutation(t *testing.T) {
	rs := NewRunState(Budget{MaxToolCalls: 20, MaxConsecutiveFail: 10})
	read := map[string]any{"path": "app.go"}

	allowed(t, rs, "read_file", read)
	rs.RecordToolCall("read_file", read, nil)
	allowed(t, rs, "read_file", read)
	rs.RecordToolCall("read_file", read, nil)

	if ok, _ := rs.BeforeToolCall("read_file", read); ok {
		t.Fatal("a third identical read with no change should be refused")
	}

	// A successful edit changes the file; the cached read is stale now.
	edit := map[string]any{"path": "app.go"}
	allowed(t, rs, "edit_file", edit)
	rs.RecordToolCall("edit_file", edit, nil)

	if ok, reason := rs.BeforeToolCall("read_file", read); !ok {
		t.Fatalf("read after a mutation should be allowed, got: %s", reason)
	}
}

func TestRunStateStopsAtToolBudget(t *testing.T) {
	rs := NewRunState(Budget{MaxToolCalls: 2, MaxConsecutiveFail: 10})
	allowed(t, rs, "read_file", map[string]any{"path": "a"})
	rs.RecordToolCall("read_file", map[string]any{"path": "a"}, nil)
	allowed(t, rs, "read_file", map[string]any{"path": "b"})
	rs.RecordToolCall("read_file", map[string]any{"path": "b"}, nil)

	ok, reason := rs.BeforeToolCall("read_file", map[string]any{"path": "c"})
	if ok {
		t.Fatal("the call over the budget should be refused")
	}
	if !strings.Contains(reason, "tool budget") {
		t.Fatalf("reason = %q, want the budget named", reason)
	}
	if _, stopped := rs.StopReason(); !stopped {
		t.Fatal("the run should be marked stopped")
	}
}

func TestRunStateLeavesRefusedCallOutOfInFlight(t *testing.T) {
	rs := NewRunState(DefaultBudget(ModeChat))
	args := map[string]any{"path": "a.go"}

	allowed(t, rs, "write_file", args)
	// A second identical call while the first is in flight is refused, but it
	// must not clear or corrupt the first call's slot.
	if ok, _ := rs.BeforeToolCall("write_file", args); ok {
		t.Fatal("an identical in-flight call should be refused")
	}
	rs.RecordToolCall("write_file", args, nil)

	// Once the first call has recorded, the slot is free. A later identical
	// write is a new action against a changed workspace, not a duplicate.
	if ok, reason := rs.BeforeToolCall("write_file", args); !ok {
		t.Fatalf("the in-flight slot was not released: %s", reason)
	}
}
