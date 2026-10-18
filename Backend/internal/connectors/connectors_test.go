package connectors

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	goai "github.com/zendev-sh/goai"

	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// dryRun is the win-control server's own switch: it reports what it would open
// instead of opening it. A child inherits this process's environment, so setting
// it here means the test never touches the desktop it runs on.
const dryRun = "CALABRASS_WIN_CONTROL_DRY_RUN"

// TestTheAgentCanCallAConnectorTool is this package's whole promise, end to end
// and without a model in the middle: a row in the database, the MCP server
// started from it, its tools turned into tools the harness can call, and a call
// that travels to the child process and comes back with an answer.
func TestTheAgentCanCallAConnectorTool(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("this asserts that the tool actually opened something")
	}

	binary := buildWinControl(t)
	t.Setenv(dryRun, "1")

	st := openStore(t)
	if _, err := st.CreateConnector(context.Background(), "win-control", binary); err != nil {
		t.Fatalf("create connector: %v", err)
	}

	pool := New(st, repoRoot(t), testLogger())
	defer pool.Close()

	briefs, tools := pool.Turn(context.Background(), "")
	byName := make(map[string]goai.Tool, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}

	// The tools the server was written with.
	for _, name := range []string{"open_app", "play_music", "search_web", "open_url"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("no %s among %v", name, names(tools))
		}
	}

	// And the note the prompt is built from: the connector's own words about what
	// it is for, and the tools it is really offering. Without this the model is
	// handed a desktop it has not been told about.
	if len(briefs) != 1 {
		t.Fatalf("got %d briefs, want one per connector", len(briefs))
	}
	if briefs[0].Name != "win-control" {
		t.Errorf("brief name = %q, want the connector's name", briefs[0].Name)
	}
	if !strings.Contains(briefs[0].Instructions, "computer") {
		t.Errorf("the connector's own words about itself did not come through: %q", briefs[0].Instructions)
	}
	if len(briefs[0].Tools) != 4 {
		t.Errorf("the brief lists %v, want every tool that was handed over", briefs[0].Tools)
	}

	answer, err := byName["play_music"].Execute(context.Background(),
		json.RawMessage(`{"name":"whysokai terrible time to meet"}`))
	if err != nil {
		t.Fatalf("play_music: %v", err)
	}
	want := "https://www.youtube.com/results?search_query=whysokai+terrible+time+to+meet"
	if !strings.Contains(answer, want) {
		t.Errorf("got %q, want it to mention %q", answer, want)
	}

	// A second turn is served by the same process, not a new one: the tools of
	// the first turn call back into the server they came from.
	pool.Turn(context.Background(), "")
	if running := pool.started(); running != 1 {
		t.Errorf("%d connectors are running, want the one that was already started", running)
	}
}

// TestADisabledConnectorIsNotStarted: the switch does what it says.
func TestADisabledConnectorIsNotStarted(t *testing.T) {
	binary := buildWinControl(t)

	st := openStore(t)
	record, err := st.CreateConnector(context.Background(), "win-control", binary)
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}
	if _, err := st.SetConnectorEnabled(context.Background(), record.ID, false); err != nil {
		t.Fatalf("disable connector: %v", err)
	}

	pool := New(st, repoRoot(t), testLogger())
	defer pool.Close()

	if _, tools := pool.Turn(context.Background(), ""); len(tools) != 0 {
		t.Errorf("got %v from a connector that is turned off, want nothing", names(tools))
	}
	if running := pool.started(); running != 0 {
		t.Errorf("%d connectors were started, want none", running)
	}
}

// TestAConnectorThatWillNotStartIsLeftOut: one broken server must not take the
// turn down with it. It is reported, and the rest of the tools still work.
func TestAConnectorThatWillNotStartIsLeftOut(t *testing.T) {
	st := openStore(t)
	if _, err := st.CreateConnector(context.Background(), "nonsense", "no-such-program-anywhere-at-all"); err != nil {
		t.Fatalf("create connector: %v", err)
	}

	pool := New(st, t.TempDir(), testLogger())
	defer pool.Close()

	if _, tools := pool.Turn(context.Background(), ""); len(tools) != 0 {
		t.Errorf("got %v from a connector that cannot start, want nothing", names(tools))
	}
}

// TestTwoConnectorsOfferingTheSameNamesAreDeduped: two servers that offer the
// same tool would put two tools with one name in front of the model, which is a
// name that means nothing. The first one to arrive keeps it.
func TestTwoConnectorsOfferingTheSameNamesAreDeduped(t *testing.T) {
	binary := buildWinControl(t)
	t.Setenv(dryRun, "1")

	st := openStore(t)
	for _, name := range []string{"win-control", "win-control again"} {
		if _, err := st.CreateConnector(context.Background(), name, binary); err != nil {
			t.Fatalf("create connector: %v", err)
		}
	}

	pool := New(st, repoRoot(t), testLogger())
	defer pool.Close()

	seen := map[string]bool{}
	_, tools := pool.Turn(context.Background(), "")
	for _, tool := range tools {
		if seen[tool.Name] {
			t.Errorf("%s is offered twice", tool.Name)
		}
		seen[tool.Name] = true
	}
	if len(seen) != 4 {
		t.Errorf("got %d distinct tools, want the four the server offers", len(seen))
	}
}

func (p *Pool) started() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.running)
}

func names(tools []goai.Tool) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.Name)
	}
	return out
}

// buildWinControl builds the MCP server that lives beside this project, so the
// test talks to the real thing rather than to a stand-in. It is skipped rather
// than failed when it cannot be built: the point of the test is the wiring, not
// whether a checkout happens to have a compiler free.
func buildWinControl(t *testing.T) string {
	t.Helper()

	module := filepath.Join(repoRoot(t), "MCP", "win-control")
	binary := filepath.Join(t.TempDir(), "win-control-mcp.exe")

	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = module
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("could not build the win-control MCP server: %v\n%s", err, out)
	}
	return binary
}

// repoRoot walks up to the checkout, which is what a connector's relative
// command is resolved against.
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not read the working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "MCP", "win-control", "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("could not find the checkout from the test's directory")
		}
		dir = parent
	}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
