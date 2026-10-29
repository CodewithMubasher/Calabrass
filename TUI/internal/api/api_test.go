package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sse writes one event the way the backend does.
func sse(w http.ResponseWriter, name string, payload any) {
	data, _ := json.Marshal(payload)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
	w.(http.Flusher).Flush()
}

func TestSendDecodesEveryEventInOrder(t *testing.T) {
	big := strings.Repeat("x", 200_000) // far larger than bufio.Scanner's default token limit

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chats/c1/messages" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		var req SendRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Content != "hi" || req.Model != "m1" {
			t.Errorf("request body = %+v", req)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		sse(w, "chat", Chat{ID: "c1", Title: "hi"})
		sse(w, "message", Message{ID: "u1", Role: "user", Content: "hi"})
		sse(w, "reasoning", map[string]string{"text": "hmm"})
		sse(w, "token", map[string]string{"text": "Hel"})
		sse(w, "token", map[string]string{"text": "lo"})
		sse(w, "trace", map[string]any{"trace": Trace{Steps: []TraceStep{{Number: 1, Parts: []TracePart{
			{Kind: PartText, Text: big},
		}}}}})
		sse(w, "done", Done{Message: Message{ID: "a1", Role: "assistant", Content: "Hello"}, Usage: Usage{InputTokens: 5, OutputTokens: 7}})
		sse(w, "error", map[string]string{"message": "boom"})
		fmt.Fprint(w, "event: mystery\ndata: {}\n\n") // unknown events are ignored
	}))
	defer srv.Close()

	c := New(srv.URL)
	c.Token = "tok"

	var kinds []string
	var traceLen int
	var done *Done
	var errText string
	err := c.Send(context.Background(), "c1", SendRequest{Content: "hi", Model: "m1"}, func(e Event) {
		kinds = append(kinds, e.Kind)
		switch e.Kind {
		case EventTrace:
			traceLen = len(e.Trace.Steps[0].Parts[0].Text)
		case EventDone:
			done = e.Done
		case EventError:
			errText = e.Text
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "chat,message,reasoning,token,token,trace,done,error"
	if got := strings.Join(kinds, ","); got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
	if traceLen != 200_000 {
		t.Errorf("large trace was truncated: %d", traceLen)
	}
	if done == nil || done.Usage.InputTokens != 5 || done.Message.ID != "a1" {
		t.Errorf("done = %+v", done)
	}
	if errText != "boom" {
		t.Errorf("error text = %q", errText)
	}
}

func TestSendReturnsAPIErrorForJSONFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"no model is available; add a provider key"}`))
	}))
	defer srv.Close()

	err := New(srv.URL).Send(context.Background(), "c1", SendRequest{Content: "x"}, func(Event) {})
	if !IsAPIError(err) || !strings.Contains(err.Error(), "no model is available") {
		t.Fatalf("err = %v", err)
	}
}

func TestSendStopsWhenContextIsCancelled(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w, "chat", Chat{ID: "c1"})
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan string, 4)
	done := make(chan error, 1)
	go func() {
		done <- New(srv.URL).Send(ctx, "c1", SendRequest{Content: "x"}, func(e Event) { got <- e.Kind })
	}()
	if k := <-got; k != EventChat {
		t.Fatalf("first event = %s", k)
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("expected an error after cancel")
	}
}

func TestUnauthorizedAndAPIErrorHelpers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"sign in"}`))
	}))
	defer srv.Close()
	_, err := New(srv.URL).Me(context.Background())
	if !IsUnauthorized(err) || !IsAPIError(err) {
		t.Fatalf("err = %v", err)
	}

	// An address nobody listens on is a transport error, not an API error.
	dead := httptest.NewServer(http.NotFoundHandler())
	url := dead.URL
	dead.Close()
	if err := New(url).Health(context.Background()); err == nil || IsAPIError(err) {
		t.Fatalf("expected a transport error, got %v", err)
	}
}

func TestChatDetailFlattensChatFields(t *testing.T) {
	var out struct {
		Chat ChatDetail `json:"chat"`
	}
	raw := `{"chat":{"id":"c1","title":"T","workspaceDir":"/p","messages":[{"id":"m1","role":"assistant","content":"hi","trace":{"steps":[{"number":1,"parts":[{"kind":"text","text":"hi"}]}]}},{"id":"m2","role":"assistant","content":"s","compacted":true,"trace":"one line summary"}]}}`
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	if out.Chat.ID != "c1" || out.Chat.WorkspaceDir != "/p" || len(out.Chat.Messages) != 2 {
		t.Fatalf("decoded = %+v", out.Chat)
	}
	if tr, ok := out.Chat.Messages[0].ParsedTrace(); !ok || tr.Steps[0].Parts[0].Text != "hi" {
		t.Errorf("trace not parsed: %+v %v", tr, ok)
	}
	if _, ok := out.Chat.Messages[1].ParsedTrace(); ok {
		t.Error("a compacted summary must not parse as a trace")
	}
}

func TestBaseURL(t *testing.T) {
	for in, want := range map[string]string{
		"":                       "http://127.0.0.1:8787",
		"127.0.0.1:9000":         "http://127.0.0.1:9000",
		"http://localhost:1/":    "http://localhost:1",
		"https://example.com/x/": "https://example.com/x",
	} {
		if got := BaseURL(in); got != want {
			t.Errorf("BaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPairingCodeIsSentWhenSetInTheEnvironment(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Calabrass-Pairing")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	t.Setenv("CALABRASS_PAIRING_CODE", "a-long-enough-pairing-secret")
	if err := New(srv.URL).Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got != "a-long-enough-pairing-secret" {
		t.Fatalf("pairing header = %q", got)
	}
	t.Setenv("CALABRASS_PAIRING_CODE", "")
	got = "unset"
	_ = New(srv.URL).Health(context.Background())
	if got != "" {
		t.Fatalf("no code configured, but header = %q", got)
	}
}
