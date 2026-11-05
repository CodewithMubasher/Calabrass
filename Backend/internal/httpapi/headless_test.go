package httpapi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// toolCallingModel is a stand-in for a model provider. It speaks the OpenAI
// streaming protocol: the first time it is asked it calls run_command with the
// command it was given; once the tool's result is in the conversation it says
// it is done. It records what the tool result said.
type toolCallingModel struct {
	mu          sync.Mutex
	command     string
	toolResults []string
}

func (m *toolCallingModel) handler(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/models"):
		_, _ = w.Write([]byte(`{"data":[{"id":"fake-model"}]}`))
		return
	case !strings.HasSuffix(r.URL.Path, "/chat/completions"):
		http.NotFound(w, r)
		return
	}

	var body struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	raw, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(raw, &body)

	sawTool := false
	m.mu.Lock()
	for _, msg := range body.Messages {
		if msg.Role == "tool" {
			sawTool = true
			m.toolResults = append(m.toolResults, fmt.Sprint(msg.Content))
		}
	}
	command := m.command
	m.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	send := func(v string) { _, _ = fmt.Fprintf(w, "data: %s\n\n", v); w.(http.Flusher).Flush() }

	if sawTool {
		send(`{"choices":[{"index":0,"delta":{"role":"assistant","content":"All done."},"finish_reason":null}]}`)
		send(`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`)
		send("[DONE]")
		return
	}
	args, _ := json.Marshal(map[string]string{"command": command})
	call, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{
		"role": "assistant",
		"tool_calls": []any{map[string]any{"index": 0, "id": "call_1", "type": "function",
			"function": map[string]any{"name": "run_command", "arguments": string(args)}}},
	}, "finish_reason": nil}}})
	send(string(call))
	send(`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`)
	send("[DONE]")
}

// headlessRun drives one unattended turn through the real API and returns the
// events the stream carried.
func headlessRun(t *testing.T, base, token, chatID string, body map[string]any) []string {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", base+"/api/chats/"+chatID+"/messages", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var events []string
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if line := sc.Text(); strings.HasPrefix(line, "event: ") {
			events = append(events, strings.TrimPrefix(line, "event: "))
		}
	}
	return events
}

type headlessEnv struct {
	base, token, chatID, workdir string
	model                        *toolCallingModel
	srv                          *Server
}

func newHeadlessEnv(t *testing.T, command string) headlessEnv {
	t.Helper()
	st := openStore(t)
	srv := NewServer(Deps{Store: st, Logger: slog.Default(), Registry: registryFor(st)})
	web := httptest.NewServer(srv)
	t.Cleanup(web.Close)

	model := &toolCallingModel{command: command}
	upstream := httptest.NewServer(http.HandlerFunc(model.handler))
	t.Cleanup(upstream.Close)

	_, su := do(t, web.URL, "POST", "/api/auth/signup", "", nil,
		map[string]string{"name": "Ada", "email": "ada@example.com", "password": "a long enough password"})
	token := su["token"].(string)
	_, prov := do(t, web.URL, "POST", "/api/providers", token, nil,
		map[string]any{"name": "Fake", "baseUrl": upstream.URL, "apiKey": "k-0123456789abcdef"})
	// Creating a provider answers with the whole list; ours is the one we named.
	providerID := ""
	if list, ok := prov["providers"].([]any); ok {
		for _, item := range list {
			if p, ok := item.(map[string]any); ok && p["name"] == "Fake" {
				providerID, _ = p["id"].(string)
			}
		}
	}
	if providerID == "" {
		t.Fatal("the provider we created is not in the response")
	}

	workdir := t.TempDir()
	_, ch := do(t, web.URL, "POST", "/api/chats", token, nil,
		map[string]any{"provider": providerID, "model": "fake-model", "workspacePath": workdir})
	chatID := ch["chat"].(map[string]any)["id"].(string)
	return headlessEnv{base: web.URL, token: token, chatID: chatID, workdir: workdir, model: model, srv: srv}
}

func (e headlessEnv) ledger(t *testing.T, query string) []map[string]any {
	t.Helper()
	_, body := do(t, e.base, "GET", "/api/ledger"+query, e.token, nil, nil)
	var out []map[string]any
	for _, x := range body["entries"].([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}

func TestUnattendedRunRefusesACommandOutsideItsScope(t *testing.T) {
	env := newHeadlessEnv(t, "touch marker.txt")
	events := headlessRun(t, env.base, env.token, env.chatID, map[string]any{"content": "make the marker", "unattended": true})

	if !contains(events, "done") {
		t.Fatalf("the run did not finish: %v", events)
	}
	if _, err := os.Stat(filepath.Join(env.workdir, "marker.txt")); err == nil {
		t.Fatal("a command outside the scope was run")
	}
	if got := strings.Join(env.model.toolResults, "\n"); !strings.Contains(got, "permission denied") {
		t.Fatalf("the model was not told it was refused: %q", got)
	}
	if contains(events, "approval") {
		t.Fatal("an unattended run asked a person; nobody is there")
	}

	ledger := env.ledger(t, "")
	if !hasEntry(ledger, "action", "unattended-refused") {
		t.Fatalf("the refusal is not in the ledger: %v", ledger)
	}
	if !hasEntry(ledger, "run", "completed") {
		t.Fatalf("the run itself is not in the ledger: %v", ledger)
	}
	if got := env.ledger(t, "?refused=1"); len(got) != 1 {
		t.Fatalf("the refused filter: %v", got)
	}
}

func TestUnattendedRunInsideItsScopeRunsTheCommand(t *testing.T) {
	env := newHeadlessEnv(t, "touch marker.txt")
	scope := map[string]any{"maxTier": 2, "commands": []string{"touch *"}}
	events := headlessRun(t, env.base, env.token, env.chatID,
		map[string]any{"content": "make the marker", "unattended": true, "scope": scope})

	if !contains(events, "done") {
		t.Fatalf("the run did not finish: %v", events)
	}
	if _, err := os.Stat(filepath.Join(env.workdir, "marker.txt")); err != nil {
		t.Fatalf("a pre-approved command did not run: %v", err)
	}
	if !hasEntry(env.ledger(t, ""), "action", "allowed") {
		t.Fatal("the allowed command is not in the ledger")
	}
}

func TestAnUnattendedRunCannotWidenItsOwnScopeToReachOutside(t *testing.T) {
	// Even with a scope that lists the command, nothing unattended reaches tier 3.
	env := newHeadlessEnv(t, "rm -rf build")
	scope := map[string]any{"maxTier": 4, "commands": []string{"rm *"}}
	headlessRun(t, env.base, env.token, env.chatID, map[string]any{"content": "clean", "unattended": true, "scope": scope})
	if got := strings.Join(env.model.toolResults, "\n"); !strings.Contains(got, "permission denied") {
		t.Fatalf("a tier 3 command ran unattended: %q", got)
	}
}

func TestLedgerNeverKeepsACredential(t *testing.T) {
	env := newHeadlessEnv(t, `curl -H "Authorization: Bearer abc123def456ghi" https://example.com`)
	headlessRun(t, env.base, env.token, env.chatID, map[string]any{"content": "fetch", "unattended": true})

	raw, _ := json.Marshal(env.ledger(t, ""))
	if strings.Contains(string(raw), "abc123def456ghi") {
		t.Fatalf("a credential was written to the ledger: %s", raw)
	}
	if !strings.Contains(string(raw), "example.com") {
		t.Fatalf("the redacted record lost the command's shape: %s", raw)
	}
}

func TestLedgerIsPerPerson(t *testing.T) {
	env := newHeadlessEnv(t, "touch x")
	headlessRun(t, env.base, env.token, env.chatID, map[string]any{"content": "go", "unattended": true})

	_, other := do(t, env.base, "POST", "/api/auth/signup", "", nil,
		map[string]string{"name": "Bob", "email": "bob@example.com", "password": "another long password"})
	_, body := do(t, env.base, "GET", "/api/ledger", other["token"].(string), nil, nil)
	if entries := body["entries"].([]any); len(entries) != 0 {
		t.Fatalf("another account can read the ledger: %v", entries)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func hasEntry(ledger []map[string]any, kind, outcome string) bool {
	for _, e := range ledger {
		if e["kind"] == kind && e["outcome"] == outcome {
			return true
		}
	}
	return false
}
