package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/CodewithMubasher/Calabrass/mcp/win-control/internal/desktop"
	"github.com/CodewithMubasher/Calabrass/mcp/win-control/internal/protocol"
)

func find(t *testing.T, name string) protocol.Tool {
	t.Helper()
	for _, tool := range All() {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("no tool called %q", name)
	return protocol.Tool{}
}

// TestPlayMusicSearchesYouTube is the promise the tool makes: YouTube's search
// page, with the words in it, in the form YouTube itself uses.
func TestPlayMusicSearchesYouTube(t *testing.T) {
	t.Setenv(desktop.DryRunEnv, "1")

	said, err := find(t, "play_music").Handler(context.Background(), map[string]any{
		"name": "whysokai terrible time to meet",
	})
	if err != nil {
		t.Fatalf("play_music: %v", err)
	}

	want := "https://www.youtube.com/results?search_query=whysokai+terrible+time+to+meet"
	if !strings.Contains(said, want) {
		t.Errorf("got %q, want it to mention %q", said, want)
	}
}

func TestSearchWebPicksTheEngine(t *testing.T) {
	t.Setenv(desktop.DryRunEnv, "1")
	tool := find(t, "search_web")

	chosen, err := tool.Handler(context.Background(), map[string]any{"query": "go mcp server", "engine": "duckduckgo"})
	if err != nil {
		t.Fatalf("search_web: %v", err)
	}
	if !strings.Contains(chosen, "https://duckduckgo.com/?q=go+mcp+server") {
		t.Errorf("got %q, want the address that was asked for", chosen)
	}

	// Google unless there is a reason for another.
	defaulted, err := tool.Handler(context.Background(), map[string]any{"query": "go mcp server"})
	if err != nil {
		t.Fatalf("search_web: %v", err)
	}
	if !strings.Contains(defaulted, "https://www.google.com/search?q=go+mcp+server") {
		t.Errorf("got %q, want Google by default", defaulted)
	}

	if _, err := tool.Handler(context.Background(), map[string]any{"query": "x", "engine": "askjeeves"}); err == nil {
		t.Error("an engine this server cannot use should be refused, not quietly ignored")
	}
}

func TestOpenURLOnlyOpensTheWeb(t *testing.T) {
	t.Setenv(desktop.DryRunEnv, "1")
	tool := find(t, "open_url")

	said, err := tool.Handler(context.Background(), map[string]any{"url": "https://go.dev/blog"})
	if err != nil {
		t.Fatalf("open_url: %v", err)
	}
	if !strings.Contains(said, "https://go.dev/blog") {
		t.Errorf("got %q", said)
	}

	// A shell would follow any of these. This server acts on whatever the model
	// was talked into asking for, so it follows none of them.
	for _, refused := range []string{
		"file:///C:/Windows/System32/calc.exe",
		"javascript:alert(1)",
		"cmd.exe",
		"",
	} {
		if _, err := tool.Handler(context.Background(), map[string]any{"url": refused}); err == nil {
			t.Errorf("%q should have been refused", refused)
		}
	}

	// The refusal a Windows URI gets has to point at the tool that can meet it:
	// a dead end is what turns "open the settings app" into "I can't".
	_, err = tool.Handler(context.Background(), map[string]any{"url": "ms-settings:"})
	if err == nil {
		t.Fatal("ms-settings: is not a web address and should have been refused")
	}
	if !strings.Contains(err.Error(), "open_app") {
		t.Errorf("the refusal is %q, and it should name the tool that can do it instead", err)
	}
}

// TestOpenAppSearchesTheStartMenu is what open_app promises for a name it only
// half knows — the case that had no tool at all.
func TestOpenAppSearchesTheStartMenu(t *testing.T) {
	t.Setenv(desktop.DryRunEnv, "1")

	said, err := find(t, "open_app").Handler(context.Background(), map[string]any{"name": "settings"})
	if err != nil {
		t.Fatalf("open_app: %v", err)
	}
	if !strings.Contains(said, `"settings"`) {
		t.Errorf("got %q, want it to say what would be typed into the Start menu", said)
	}
	if !strings.Contains(said, "Enter") {
		t.Errorf("got %q, want it to say what would be pressed", said)
	}
}

// TestOpenAppOpensAKnownThingDirectly: the second way in, for an exact program,
// path or Windows URI.
func TestOpenAppOpensAKnownThingDirectly(t *testing.T) {
	t.Setenv(desktop.DryRunEnv, "1")
	tool := find(t, "open_app")

	said, err := tool.Handler(context.Background(), map[string]any{"name": "ms-settings:", "mode": "shell"})
	if err != nil {
		t.Fatalf("open_app: %v", err)
	}
	if !strings.Contains(said, "ms-settings:") {
		t.Errorf("got %q, want the URI it opened", said)
	}

	if _, err := tool.Handler(context.Background(), map[string]any{"name": "notepad", "mode": "telepathy"}); err == nil {
		t.Error("a mode this server cannot use should be refused, not quietly ignored")
	}
}

func TestArgumentsAreCheckedBeforeTheyReachTheDesktop(t *testing.T) {
	t.Setenv(desktop.DryRunEnv, "1")
	play := find(t, "play_music")

	for name, args := range map[string]map[string]any{
		"nothing at all":    {},
		"only spaces":       {"name": "   "},
		"too long":          {"name": strings.Repeat("a", 201)},
		"control character": {"name": "a song\nrm -rf"},
	} {
		if _, err := play.Handler(context.Background(), args); err == nil {
			t.Errorf("%s should have been refused", name)
		}
	}
}

// TestEveryToolIsDescribed: a client shows the model the name, the description
// and the schema, so a tool missing any of them is one the model cannot use.
func TestEveryToolIsDescribed(t *testing.T) {
	for _, tool := range All() {
		if tool.Name == "" {
			t.Error("a tool without a name")
		}
		if len(tool.Description) < 20 {
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
	}
}
