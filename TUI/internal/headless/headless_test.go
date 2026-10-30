package headless

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"calabrass-tui/internal/api"
)

func TestParseTaskFlagsAndOrder(t *testing.T) {
	o, err := Parse([]string{"fix", "the", "test", "--allow", "go test *", "-C", "/tmp/p", "--json", "-q"}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if o.Prompt != "fix the test" || o.Dir != "/tmp/p" || !o.JSON || !o.Quiet {
		t.Fatalf("%+v", o)
	}
	if len(o.Allow) != 1 || o.Allow[0] != "go test *" {
		t.Fatalf("allow = %v", o.Allow)
	}
	// Flags before the task work too, and --allow repeats and splits on commas.
	o, err = Parse([]string{"--allow", "npm test", "--allow=go *, make", "do it"}, nil, false)
	if err != nil || o.Prompt != "do it" || len(o.Allow) != 3 {
		t.Fatalf("%+v %v", o, err)
	}
	if o.Timeout != 15*time.Minute {
		t.Fatalf("default timeout = %v", o.Timeout)
	}
}

func TestParseReadsAPipe(t *testing.T) {
	o, err := Parse([]string{"explain this"}, strings.NewReader("error: boom\n"), true)
	if err != nil || !strings.HasPrefix(o.Prompt, "explain this") || !strings.Contains(o.Prompt, "error: boom") {
		t.Fatalf("%+v %v", o, err)
	}
	// Piped input alone is the task.
	o, err = Parse(nil, strings.NewReader("just the pipe"), true)
	if err != nil || o.Prompt != "just the pipe" {
		t.Fatalf("%+v %v", o, err)
	}
	// A terminal is not read.
	if _, err := Parse(nil, strings.NewReader("ignored"), false); err == nil {
		t.Fatal("a task was invented from a terminal")
	}
}

func TestParseRefusesWhatItShould(t *testing.T) {
	for name, args := range map[string][]string{
		"no task":      {},
		"bad model":    {"--model", "nope", "x"},
		"neg timeout":  {"--timeout", "-1s", "x"},
		"unknown flag": {"--frobnicate", "x"},
	} {
		if _, err := Parse(args, nil, false); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	big := strings.NewReader(strings.Repeat("x", MaxInput+10))
	if _, err := Parse([]string{"task"}, big, true); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("an oversized pipe was accepted: %v", err)
	}
}

// fakeHost speaks just enough of the Calabrass API for one run, and records
// what the run asked for.
type fakeHost struct {
	events  []string // pre-rendered SSE frames
	sent    api.SendRequest
	created api.CreateChatRequest
	srv     *httptest.Server
}

func frame(name string, payload any) string {
	data, _ := json.Marshal(payload)
	return fmt.Sprintf("event: %s\ndata: %s\n\n", name, data)
}

func newFakeHost(t *testing.T, events ...string) *fakeHost {
	h := &fakeHost{events: events}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chats", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&h.created)
		_, _ = w.Write([]byte(`{"chat":{"id":"c1"}}`))
	})
	mux.HandleFunc("POST /api/chats/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&h.sent)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, e := range h.events {
			_, _ = io.WriteString(w, e)
			w.(http.Flusher).Flush()
		}
	})
	h.srv = httptest.NewServer(mux)
	t.Cleanup(h.srv.Close)
	return h
}

func run(t *testing.T, h *fakeHost, opts Options) (Result, string, string) {
	t.Helper()
	var out, errw bytes.Buffer
	c := api.New(strings.TrimPrefix(h.srv.URL, "http://"))
	res := Run(context.Background(), c, opts, &out, &errw)
	return res, out.String(), errw.String()
}

// wrap is the shape the backend sends a trace in.
func wrap(t api.Trace) map[string]any { return map[string]any{"trace": t} }

func tool(id, name, input, status, errText string) api.Trace {
	return api.Trace{Steps: []api.TraceStep{{Number: 1, Parts: []api.TracePart{
		{Kind: api.PartTool, CallID: id, Name: name, Input: input, Status: status, Error: errText}}}}}
}

func TestASuccessfulRunPutsTheAnswerOnStdoutAndProgressOnStderr(t *testing.T) {
	h := newFakeHost(t,
		frame("token", map[string]string{"text": "All "}),
		frame("trace", wrap(tool("k1", "run_command", `{"command":"go test ./..."}`, "done", ""))),
		frame("token", map[string]string{"text": "good."}),
		frame("done", map[string]any{"message": map[string]any{"id": "m"}, "steps": 2}),
	)
	res, out, errw := run(t, h, Options{Prompt: "check it", Dir: "/work", Allow: []string{"go test *"}})

	if res.Exit != ExitOK {
		t.Fatalf("exit %d: %s", res.Exit, res.Err)
	}
	if out != "All good.\n" {
		t.Fatalf("stdout = %q (must be only the answer)", out)
	}
	if !strings.Contains(errw, "run_command: go test ./...") {
		t.Fatalf("stderr = %q", errw)
	}
	// The run is unattended, in the folder it was given, with exactly the scope asked for.
	if !h.sent.Unattended || h.sent.Scope == nil || h.sent.Scope.MaxTier != 2 || h.sent.Scope.Commands[0] != "go test *" {
		t.Fatalf("request = %+v scope=%+v", h.sent, h.sent.Scope)
	}
	if h.created.WorkspacePath != "/work" {
		t.Fatalf("workspace = %q", h.created.WorkspacePath)
	}
}

func TestWithNoAllowTheScopeIsFilesOnly(t *testing.T) {
	h := newFakeHost(t, frame("done", map[string]any{"message": map[string]any{}}))
	run(t, h, Options{Prompt: "x", Dir: "/w"})
	if h.sent.Scope == nil || h.sent.Scope.MaxTier != 1 || len(h.sent.Scope.Commands) != 0 {
		t.Fatalf("scope = %+v", h.sent.Scope)
	}
}

func TestARefusedActionExitsBlockedAndSaysHowToAllowIt(t *testing.T) {
	h := newFakeHost(t,
		frame("trace", wrap(tool("k1", "run_command", `{"command":"npm install"}`, "error", `permission denied: "Run: npm install" was not run`))),
		frame("token", map[string]string{"text": "I could not install."}),
		frame("done", map[string]any{"message": map[string]any{}}),
	)
	res, out, errw := run(t, h, Options{Prompt: "install", Dir: "/w"})
	if res.Exit != ExitBlocked || len(res.Blocked) != 1 {
		t.Fatalf("exit %d blocked %v", res.Exit, res.Blocked)
	}
	if !strings.Contains(out, "I could not install.") {
		t.Fatalf("the answer was lost: %q", out)
	}
	if !strings.Contains(errw, "refused") || !strings.Contains(errw, "--allow") || !strings.Contains(errw, "npm install") {
		t.Fatalf("stderr = %q", errw)
	}
}

func TestFailuresMapToTheirOwnExitCodes(t *testing.T) {
	h := newFakeHost(t, frame("error", map[string]string{"message": "the provider returned 500"}))
	if res, _, errw := run(t, h, Options{Prompt: "x", Dir: "/w"}); res.Exit != ExitFailed || !strings.Contains(errw, "500") {
		t.Fatalf("exit %d, stderr %q", res.Exit, errw)
	}

	h = newFakeHost(t, frame("error", map[string]string{"message": "no model is available; add a provider key"}))
	res, _, errw := run(t, h, Options{Prompt: "x", Dir: "/w"})
	if res.Exit != ExitSetup || !strings.Contains(errw, "Open Calabrass once") {
		t.Fatalf("no model: exit %d, stderr %q", res.Exit, errw)
	}

	h = newFakeHost(t, frame("done", map[string]any{"message": map[string]any{}, "stepsExhausted": true}))
	if res, _, _ := run(t, h, Options{Prompt: "x", Dir: "/w"}); res.Exit != ExitFailed {
		t.Fatalf("out of steps: exit %d", res.Exit)
	}
}

func TestATimeoutExitsFourEvenWhileTheStreamIsOpen(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chats", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"chat":{"id":"c"}}`)) })
	mux.HandleFunc("POST /api/chats/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done() // never answers
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var out, errw bytes.Buffer
	res := Run(context.Background(), api.New(strings.TrimPrefix(srv.URL, "http://")),
		Options{Prompt: "x", Dir: "/w", Timeout: 150 * time.Millisecond}, &out, &errw)
	if res.Exit != ExitTimeout {
		t.Fatalf("exit %d: %s", res.Exit, res.Err)
	}
}

func TestJSONModeIsOneObjectPerLineAndEndsWithTheResult(t *testing.T) {
	h := newFakeHost(t,
		frame("token", map[string]string{"text": "hi"}),
		frame("done", map[string]any{"message": map[string]any{}, "steps": 1}),
	)
	res, out, errw := run(t, h, Options{Prompt: "x", Dir: "/w", JSON: true})
	if res.Exit != ExitOK || errw != "" {
		t.Fatalf("exit %d stderr %q (json mode keeps stderr quiet)", res.Exit, errw)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var last map[string]any
	for i, l := range lines {
		var v map[string]any
		if err := json.Unmarshal([]byte(l), &v); err != nil {
			t.Fatalf("line %d is not JSON: %q", i, l)
		}
		last = v
	}
	if last["type"] != "result" || last["ok"] != true || last["answer"] != "hi" || last["exit"] != float64(0) {
		t.Fatalf("result line = %v", last)
	}
}

func TestQuietSuppressesProgress(t *testing.T) {
	h := newFakeHost(t,
		frame("trace", wrap(tool("k", "run_command", `{"command":"ls"}`, "done", ""))),
		frame("token", map[string]string{"text": "ok"}),
		frame("done", map[string]any{"message": map[string]any{}}),
	)
	_, _, errw := run(t, h, Options{Prompt: "x", Dir: "/w", Quiet: true})
	if errw != "" {
		t.Fatalf("stderr = %q", errw)
	}
}
