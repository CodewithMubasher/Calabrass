package httpapi

import (
	"testing"

	goai "github.com/zendev-sh/goai"

	"github.com/CodewithMubasher/Calabrass/backend/internal/connectors"
)

func toolsNamed(names ...string) []goai.Tool {
	out := make([]goai.Tool, 0, len(names))
	for _, n := range names {
		out = append(out, goai.Tool{Name: n})
	}
	return out
}

func names(tools []goai.Tool) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Name)
	}
	return out
}

func twoConnectors() ([]connectors.Brief, []goai.Tool) {
	briefs := []connectors.Brief{
		{ID: "a", Name: "Alpha", Tools: []string{"a_read", "a_write"}},
		{ID: "b", Name: "Beta", Tools: []string{"b_send"}},
	}
	return briefs, toolsNamed("a_read", "a_write", "b_send")
}

// The bug: an agent bound to connector A was told only about A, but was still
// handed B's tools, and could call them.
func TestAnAgentBoundToOneConnectorIsOfferedOnlyItsTools(t *testing.T) {
	briefs, offered := twoConnectors()
	gotBriefs, gotTools := restrictToConnector(briefs, offered, "a")

	if len(gotBriefs) != 1 || gotBriefs[0].ID != "a" {
		t.Fatalf("briefs = %+v", gotBriefs)
	}
	got := names(gotTools)
	if len(got) != 2 || got[0] != "a_read" || got[1] != "a_write" {
		t.Fatalf("tools = %v, want only Alpha's", got)
	}
	for _, name := range got {
		if name == "b_send" {
			t.Fatal("another connector's tool was offered to a bound agent")
		}
	}
}

func TestTheBriefAndTheToolsAlwaysAgree(t *testing.T) {
	briefs, offered := twoConnectors()
	for _, id := range []string{"a", "b"} {
		gotBriefs, gotTools := restrictToConnector(briefs, offered, id)
		described := map[string]bool{}
		for _, b := range gotBriefs {
			for _, name := range b.Tools {
				described[name] = true
			}
		}
		for _, name := range names(gotTools) {
			if !described[name] {
				t.Errorf("connector %s: %q is offered but not described to the model", id, name)
			}
		}
		if len(described) != len(gotTools) {
			t.Errorf("connector %s: described %d tools, offered %d", id, len(described), len(gotTools))
		}
	}
}

// A connector that is off or failed to start is not in the list. The agent must
// then get no connector tools, not all of them.
func TestABoundAgentWhoseConnectorIsUnavailableGetsNothing(t *testing.T) {
	briefs, offered := twoConnectors()
	gotBriefs, gotTools := restrictToConnector(briefs, offered, "gone")
	if len(gotBriefs) != 0 || len(gotTools) != 0 {
		t.Fatalf("a missing connector must offer nothing; got briefs=%d tools=%v", len(gotBriefs), names(gotTools))
	}
}

func TestNothingOfferedStaysNothing(t *testing.T) {
	gotBriefs, gotTools := restrictToConnector(nil, nil, "a")
	if len(gotBriefs) != 0 || len(gotTools) != 0 {
		t.Fatalf("%v %v", gotBriefs, gotTools)
	}
}

// A tool the connector does not list (a name clash with another connector's tool
// that the pool skipped) must not slip through on a matching name alone.
func TestOnlyToolsTheConnectorListsAreKept(t *testing.T) {
	briefs := []connectors.Brief{{ID: "a", Tools: []string{"a_read"}}}
	offered := toolsNamed("a_read", "a_extra")
	_, got := restrictToConnector(briefs, offered, "a")
	if len(got) != 1 || got[0].Name != "a_read" {
		t.Fatalf("tools = %v", names(got))
	}
}
