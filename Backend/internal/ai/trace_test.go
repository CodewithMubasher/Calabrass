package ai

import "testing"

// TestToolPartsAreStampedWithTheirServer: a connector's tools carry the
// connector's name on the part, so a timeline can say which MCP server a call
// came from instead of showing every unknown tool as a bare wrench.
func TestToolPartsAreStampedWithTheirServer(t *testing.T) {
	builder := traceBuilder{origins: map[string]string{
		"write_table": "Excel",
		"play_music":  "win-control",
	}}

	builder.toolStart("call-1", "write_table")
	builder.toolInput("call-1", "write_table", `{"sheet":"Sales"}`)
	builder.toolStart("call-2", "play_music")
	builder.toolStart("call-3", "read_file") // the harness's own tool, unowned

	parts := builder.trace.Steps[0].Parts
	if len(parts) != 3 {
		t.Fatalf("got %d parts, want 3", len(parts))
	}
	want := []struct{ callID, name, server string }{
		{"call-1", "write_table", "Excel"},
		{"call-2", "play_music", "win-control"},
		{"call-3", "read_file", ""},
	}
	for i, w := range want {
		if parts[i].CallID != w.callID || parts[i].Name != w.name || parts[i].Server != w.server {
			t.Errorf("part %d = %+v, want call %s/%s on %q", i, parts[i], w.callID, w.name, w.server)
		}
	}
}

// TestATurnWithoutConnectorsHasNoServers: with no origins there is nothing to
// name, and the harness's own tools stay unowned.
func TestATurnWithoutConnectorsHasNoServers(t *testing.T) {
	builder := traceBuilder{}
	builder.toolStart("call-1", "write_table")

	if server := builder.trace.Steps[0].Parts[0].Server; server != "" {
		t.Errorf("server = %q, want empty", server)
	}
}
