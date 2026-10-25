package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"calabrass-tui/internal/api"
	"calabrass-tui/internal/settings"
)

// --- a fake backend that speaks the real wire format ---------------------------

type fakeAPI struct {
	mu        sync.Mutex
	models    []api.ProviderInfo
	providers []api.ProviderRow
	connect   []api.Connector
	chats     map[string]*api.ChatDetail
	sent      []api.SendRequest
	created   []api.CreateChatRequest
	script    func(w http.ResponseWriter, r *http.Request, emit func(string, any))
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{chats: map[string]*api.ChatDetail{}}
}

func (f *fakeAPI) serve() *httptest.Server {
	mux := http.NewServeMux()
	js := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { js(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /api/auth/signup", func(w http.ResponseWriter, r *http.Request) {
		js(w, 201, api.Session{Token: "tok", User: api.User{ID: "u", Email: "x@y.z"}})
	})
	mux.HandleFunc("GET /api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			js(w, 401, map[string]string{"error": "no"})
			return
		}
		js(w, 200, map[string]api.User{"user": {ID: "u"}})
	})
	mux.HandleFunc("GET /api/models", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		js(w, 200, map[string]any{"providers": f.models})
	})
	mux.HandleFunc("GET /api/providers", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		js(w, 200, map[string]any{"providers": f.providers})
	})
	mux.HandleFunc("PUT /api/providers/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var u api.ProviderUpdate
		_ = json.NewDecoder(r.Body).Decode(&u)
		for i := range f.providers {
			if f.providers[i].ID != r.PathValue("id") {
				continue
			}
			if u.APIKey != nil {
				f.providers[i].APIKey = *u.APIKey
				f.providers[i].Connected = *u.APIKey != ""
			}
			if u.Enabled != nil {
				f.providers[i].Enabled = *u.Enabled
			}
			if f.providers[i].Enabled && f.providers[i].Connected {
				f.models = []api.ProviderInfo{{ID: f.providers[i].ID, Name: f.providers[i].Name, DefaultModel: "m-1",
					Models: []api.ModelInfo{{ID: "m-1", Label: "Model One", Provider: f.providers[i].ID}}}}
			}
		}
		js(w, 200, map[string]any{"providers": f.providers})
	})
	mux.HandleFunc("GET /api/connectors", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		js(w, 200, map[string]any{"connectors": f.connect})
	})
	mux.HandleFunc("PATCH /api/connectors/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var b struct{ Enabled bool }
		_ = json.NewDecoder(r.Body).Decode(&b)
		for i := range f.connect {
			if f.connect[i].ID == r.PathValue("id") {
				f.connect[i].Enabled = b.Enabled
			}
		}
		js(w, 200, map[string]any{"connectors": f.connect})
	})
	mux.HandleFunc("GET /api/chats", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var list []api.Chat
		for _, c := range f.chats {
			list = append(list, c.Chat)
		}
		js(w, 200, map[string]any{"chats": list})
	})
	mux.HandleFunc("POST /api/chats", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var req api.CreateChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.created = append(f.created, req)
		id := fmt.Sprintf("chat-%d", len(f.chats)+1)
		c := &api.ChatDetail{Chat: api.Chat{ID: id, Provider: req.Provider, Model: req.Model, Mode: "chat", WorkspaceDir: req.WorkspacePath}}
		f.chats[id] = c
		js(w, 201, map[string]any{"chat": c.Chat})
	})
	mux.HandleFunc("GET /api/chats/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		c, ok := f.chats[r.PathValue("id")]
		if !ok {
			js(w, 404, map[string]string{"error": "not found"})
			return
		}
		js(w, 200, map[string]any{"chat": c})
	})
	mux.HandleFunc("DELETE /api/chats/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		delete(f.chats, r.PathValue("id"))
		js(w, 200, map[string]bool{"deleted": true})
	})
	mux.HandleFunc("POST /api/chats/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		var req api.SendRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.sent = append(f.sent, req)
		script := f.script
		f.mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fl := w.(http.Flusher)
		emit := func(name string, payload any) {
			data, _ := json.Marshal(payload)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
			fl.Flush()
		}
		script(w, r, emit)
	})
	return httptest.NewServer(mux)
}

// --- a harness that runs the real Update loop -----------------------------------

type harness struct {
	t *testing.T
	m Model
}

func isAppMsg(msg tea.Msg) bool {
	switch msg.(type) {
	case bootDoneMsg, modelsMsg, connectorsMsg, chatsMsg, chatLoadedMsg, providersMsg, providerSavedMsg,
		chatDeletedMsg, renamedMsg, cdDoneMsg, turnBatchMsg, ListSelectMsg, ListKeyMsg, ProviderSaveMsg,
		ProviderFormCancelMsg, QuestionsAnsweredMsg, DialogSelectMsg, QuickSelectMsg, SlashSelectMsg, PromptSubmitMsg:
		return true
	}
	return false
}

// runCmd runs a command (with a timeout, since tick commands sleep) and returns
// the app messages it produced.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		if b, ok := msg.(tea.BatchMsg); ok {
			var out []tea.Msg
			for _, c := range b {
				out = append(out, runCmd(c)...)
			}
			return out
		}
		if isAppMsg(msg) {
			return []tea.Msg{msg}
		}
	case <-time.After(1500 * time.Millisecond):
	}
	return nil
}

func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	queue := []tea.Msg{msg}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 500 {
			h.t.Fatal("update loop did not settle")
		}
		next := queue[0]
		queue = queue[1:]
		model, cmd := h.m.Update(next)
		h.m = model.(Model)
		queue = append(queue, runCmd(cmd)...)
	}
}

func (h *harness) screen() string { return ansi.Strip(h.m.viewString()) }

func (h *harness) typeAndEnter(text string) {
	h.t.Helper()
	h.m.prompt.SetValue(text)
	h.send(tea.KeyPressMsg{Code: tea.KeyEnter})
}

func boot(t *testing.T, f *fakeAPI) (*harness, func()) {
	t.Helper()
	msgCache = map[int]cachedMsg{} // IDs restart with every Model; the real app has one
	t.Setenv(settings.EnvDir, t.TempDir())
	t.Setenv("CALABRASS_EMAIL", "")
	t.Setenv("CALABRASS_PASSWORD", "")
	srv := f.serve()
	st := &settings.Settings{}
	app := &appCtx{client: api.New(srv.URL), st: st}
	h := &harness{t: t, m: NewModel(app)}
	h.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	// Init() starts the boot sequence.
	for _, msg := range runCmd(bootCmd(app)) {
		h.send(msg)
	}
	return h, srv.Close
}

func oneModel() []api.ProviderInfo {
	return []api.ProviderInfo{{ID: "p1", Name: "Prov", DefaultModel: "m-1",
		Models: []api.ModelInfo{{ID: "m-1", Label: "Model One", Provider: "p1"}, {ID: "m-2", Label: "Model Two", Provider: "p1"}}}}
}

// --- tests ----------------------------------------------------------------------

func TestBootLoadsRealModelsAndConnectors(t *testing.T) {
	f := newFakeAPI()
	f.models = oneModel()
	f.connect = []api.Connector{{ID: "c1", Name: "Excel", Enabled: true}, {ID: "c2", Name: "Gmail"}}
	h, closeFn := boot(t, f)
	defer closeFn()

	if h.m.boot != bootReady {
		t.Fatalf("boot = %v (%s)", h.m.boot, h.m.bootErr)
	}
	if h.m.prompt.Model != "Model One" || h.m.prompt.Provider != "Prov" {
		t.Errorf("prompt shows %q / %q", h.m.prompt.Model, h.m.prompt.Provider)
	}
	if h.m.enabledMCPs() != 1 {
		t.Errorf("enabled MCPs = %d", h.m.enabledMCPs())
	}
	if !strings.Contains(h.screen(), "1 MCP") {
		t.Errorf("home status bar does not show the real MCP count:\n%s", h.screen())
	}
}

func TestNoModelsShowsHowToConnect(t *testing.T) {
	f := newFakeAPI()
	h, closeFn := boot(t, f)
	defer closeFn()

	if !strings.Contains(h.screen(), "/connect") {
		t.Errorf("home screen should explain /connect:\n%s", h.screen())
	}
	h.typeAndEnter("hello")
	if len(f.sent) != 0 || len(f.created) != 0 {
		t.Error("nothing may be sent without a model")
	}
	if !strings.Contains(h.screen(), "No model is available") {
		t.Errorf("missing explanation:\n%s", h.screen())
	}
}

func TestFullAgentTurnRendersReasoningToolsEditsAndAnswer(t *testing.T) {
	f := newFakeAPI()
	f.models = oneModel()
	f.script = func(w http.ResponseWriter, r *http.Request, emit func(string, any)) {
		emit("chat", api.Chat{ID: "chat-1", Title: "make hello", Provider: "p1", Model: "m-1"})
		emit("message", api.Message{ID: "u1", Role: "user", Content: "make hello"})
		emit("reasoning", map[string]string{"text": "I should create the file."})
		emit("token", map[string]string{"text": "Let me look first. "})
		pend := api.Trace{Steps: []api.TraceStep{{Number: 1, Parts: []api.TracePart{
			{Kind: "reasoning", Text: "I should create the file."},
			{Kind: "text", Text: "Let me look first. "},
			{Kind: "tool", CallID: "a", Name: "list_dir", Input: `{"path":"."}`, Status: "pending"},
		}}}}
		emit("trace", map[string]any{"trace": pend})
		step1 := pend
		step1.Steps[0].FinishReason = "tool-calls"
		step1.Steps[0].Parts[2].Status = "ok"
		step1.Steps[0].Parts[2].Output = "main.go\nREADME.md"
		step1.Steps[0].Parts[2].DurationMs = 12
		emit("trace", map[string]any{"trace": step1})
		// step 2 streams as deltas: write a file
		wf := api.TracePart{Kind: "tool", CallID: "b", Name: "write_file",
			Input: `{"path":"hello.py","content":"print('hi')\nprint('there')\n"}`, Status: "pending"}
		step2 := api.Trace{Steps: append(append([]api.TraceStep{}, step1.Steps...), api.TraceStep{Number: 2, Parts: []api.TracePart{wf}})}
		emit("trace", map[string]any{"trace": step2})
		step2.Steps[1].Parts[0].Status = "ok"
		step2.Steps[1].Parts[0].Output = "wrote hello.py"
		step2.Steps[1].FinishReason = "tool-calls"
		emit("trace", map[string]any{"trace": step2})
		// step 3: an edit, then the closing answer as plain deltas
		ef := api.TracePart{Kind: "tool", CallID: "c", Name: "edit_file",
			Input:  `{"path":"hello.py","oldString":"print('hi')","newString":"print('hello')"}`,
			Status: "ok", Output: "edited hello.py: replaced 1 occurrence(s) at line 1"}
		step3 := api.Trace{Steps: append(append([]api.TraceStep{}, step2.Steps...), api.TraceStep{Number: 3, FinishReason: "tool-calls", Parts: []api.TracePart{ef}})}
		emit("trace", map[string]any{"trace": step3})
		emit("token", map[string]string{"text": "Done: "})
		emit("token", map[string]string{"text": "created **hello.py**."})
		final := step3
		final.Steps = append(append([]api.TraceStep{}, step3.Steps...), api.TraceStep{Number: 4, FinishReason: "stop",
			Parts: []api.TracePart{{Kind: "text", Text: "Done: created **hello.py**."}}})
		emit("trace", map[string]any{"trace": final})
		raw, _ := json.Marshal(final)
		emit("done", api.Done{Message: api.Message{ID: "a1", Role: "assistant", Content: "Done: created **hello.py**.",
			Trace: raw, ThinkingMs: 640}, Usage: api.Usage{InputTokens: 120, OutputTokens: 30}})
	}
	h, closeFn := boot(t, f)
	defer closeFn()

	h.typeAndEnter("make hello")

	if h.m.running() {
		t.Fatal("turn should be finished")
	}
	if len(f.created) != 1 || f.created[0].Mode != "chat" || f.created[0].WorkspacePath != h.m.cwdPath ||
		f.created[0].Provider != "p1" || f.created[0].Model != "m-1" {
		t.Errorf("chat was created with %+v (cwd %q)", f.created, h.m.cwdPath)
	}
	if len(f.sent) != 1 || f.sent[0].Content != "make hello" || f.sent[0].Model != "m-1" {
		t.Errorf("sent = %+v", f.sent)
	}
	if h.m.usageIn != 120 || h.m.usageOut != 30 {
		t.Errorf("usage = %d/%d", h.m.usageIn, h.m.usageOut)
	}
	if h.m.chat == nil || h.m.chat.ID != "chat-1" {
		t.Fatalf("chat = %+v", h.m.chat)
	}

	last := h.m.messages[len(h.m.messages)-1]
	if last.Role != RoleAssistant || last.Streaming || last.Pending || last.Interrupted || last.Err != "" {
		t.Fatalf("final message state: %+v", last)
	}
	var kinds []string
	for _, p := range last.Parts {
		switch p.Kind {
		case PartThought:
			kinds = append(kinds, "thought")
		case PartText:
			kinds = append(kinds, "text")
		case PartTool:
			kinds = append(kinds, "tool:"+p.Label)
		case PartEdit:
			kinds = append(kinds, "edit:"+p.File)
		case PartPatch:
			kinds = append(kinds, fmt.Sprintf("patch:%s@%d", p.File, p.Start))
		}
	}
	want := "thought,text,tool:Listed .,edit:hello.py,patch:hello.py@1,text"
	if got := strings.Join(kinds, ","); got != want {
		t.Fatalf("parts = %s\nwant    %s", got, want)
	}
	if last.Parts[0].Ms != 640 {
		t.Errorf("thought duration = %d, want 640 (from thinkingMs)", last.Parts[0].Ms)
	}
	if last.Parts[3].Code != "print('hi')\nprint('there')\n" {
		t.Errorf("written code = %q", last.Parts[3].Code)
	}

	// Open every row and check the screen shows the real content.
	h.send(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	scr := h.screen()
	for _, s := range []string{"make hello", "Wrote hello.py", "print('there')", "Edit hello.py", "print('hello')",
		"Listed .", "README.md", "created hello.py.", "Chat", "Model One"} {
		if !strings.Contains(scr, s) {
			t.Errorf("screen is missing %q:\n%s", s, scr)
		}
	}
	if !strings.Contains(scr, "150 tokens") {
		t.Errorf("footer should show the 150 real tokens:\n%s", scr)
	}
}

func TestEscInterruptsAndEnterIsBlockedWhileRunning(t *testing.T) {
	f := newFakeAPI()
	f.models = oneModel()
	release := make(chan struct{})
	f.script = func(w http.ResponseWriter, r *http.Request, emit func(string, any)) {
		emit("chat", api.Chat{ID: "chat-1"})
		emit("token", map[string]string{"text": "partial answer"})
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}
	h, closeFn := boot(t, f)
	defer closeFn()
	defer close(release)

	// Start the turn but do not wait for it to finish: pump only what is ready.
	h.m.prompt.SetValue("go")
	model, cmd := h.m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	h.m = model.(Model)
	deadline := time.Now().Add(3 * time.Second)
	pending := runCmd(cmd)
	for time.Now().Before(deadline) && !strings.Contains(h.screen(), "partial answer") {
		for _, msg := range pending {
			model, cmd := h.m.Update(msg)
			h.m = model.(Model)
			pending = append(pending[:0:0], runCmd(cmd)...)
		}
		if len(pending) == 0 {
			break
		}
	}
	if !strings.Contains(h.screen(), "partial answer") {
		t.Fatalf("streamed text never appeared:\n%s", h.screen())
	}
	if !h.m.running() {
		t.Fatal("turn should still be running")
	}

	// Enter while running must not send or clear the typed text.
	h.m.prompt.SetValue("second")
	h.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if h.m.prompt.Value() != "second" || len(f.sent) != 1 {
		t.Errorf("enter while running: prompt=%q sent=%d", h.m.prompt.Value(), len(f.sent))
	}

	h.send(tea.KeyPressMsg{Code: tea.KeyEscape})
	if h.m.running() {
		t.Fatal("esc should interrupt")
	}
	last := h.m.messages[len(h.m.messages)-1]
	if !last.Interrupted || last.Streaming || len(last.Parts) != 1 || last.Parts[0].Text != "partial answer" {
		t.Errorf("after interrupt: %+v", last)
	}
	if !strings.Contains(h.screen(), "interrupted") {
		t.Errorf("screen should say interrupted:\n%s", h.screen())
	}
}

func TestBackendErrorIsShownInline(t *testing.T) {
	f := newFakeAPI()
	f.models = oneModel()
	f.script = func(w http.ResponseWriter, r *http.Request, emit func(string, any)) {
		emit("chat", api.Chat{ID: "chat-1"})
		emit("error", map[string]string{"message": "provider said: rate limited"})
	}
	h, closeFn := boot(t, f)
	defer closeFn()
	h.typeAndEnter("hi")
	if h.m.running() {
		t.Fatal("should have finished")
	}
	if !strings.Contains(h.screen(), "provider said: rate limited") {
		t.Errorf("error not shown:\n%s", h.screen())
	}
}

func TestQuestionsFormSendsAnswersInTheWebFormat(t *testing.T) {
	f := newFakeAPI()
	f.models = oneModel()
	calls := 0
	f.script = func(w http.ResponseWriter, r *http.Request, emit func(string, any)) {
		calls++
		emit("chat", api.Chat{ID: "chat-1"})
		if calls > 1 {
			emit("token", map[string]string{"text": "Thanks!"})
			emit("done", api.Done{Message: api.Message{ID: "a2", Role: "assistant", Content: "Thanks!"}})
			return
		}
		tr := api.Trace{Steps: []api.TraceStep{{Number: 1, FinishReason: "tool-calls", Parts: []api.TracePart{
			{Kind: "tool", CallID: "q", Name: "ask_questions", Status: "ok",
				Input: `{"questions":[{"question":"Which database?","options":["Postgres","SQLite"]},{"question":"App name?"}]}`},
		}}}}
		raw, _ := json.Marshal(tr)
		emit("trace", map[string]any{"trace": tr})
		emit("done", api.Done{Message: api.Message{ID: "a1", Role: "assistant", Content: "", Trace: raw}})
	}
	h, closeFn := boot(t, f)
	defer closeFn()

	h.typeAndEnter("build an app")
	if !h.m.qform.IsOpen() {
		t.Fatal("the questions form should open when the agent asks")
	}
	if !strings.Contains(h.screen(), "Which database?") {
		t.Errorf("form does not show the question:\n%s", h.screen())
	}
	h.send(tea.KeyPressMsg{Code: tea.KeyDown})  // SQLite
	h.send(tea.KeyPressMsg{Code: tea.KeyEnter}) // choose, next question
	for _, r := range "Notes" {
		h.send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	h.send(tea.KeyPressMsg{Code: tea.KeyEnter})

	if len(f.sent) != 2 {
		t.Fatalf("sent %d messages", len(f.sent))
	}
	want := "Answers to your questions:\n1. Which database? → SQLite\n2. App name? → Notes"
	if f.sent[1].Content != want {
		t.Errorf("answers message =\n%q\nwant\n%q", f.sent[1].Content, want)
	}
}

func TestResumeRebuildsHistoryLikeALiveTurn(t *testing.T) {
	f := newFakeAPI()
	f.models = oneModel()
	tr := api.Trace{Steps: []api.TraceStep{
		{Number: 1, Parts: []api.TracePart{
			{Kind: "reasoning", Text: "plan"},
			{Kind: "tool", CallID: "w", Name: "write_file", Status: "ok", Input: `{"path":"a.go","content":"package a\n"}`},
		}},
		{Number: 2, Parts: []api.TracePart{{Kind: "text", Text: "All done."}}},
	}}
	raw, _ := json.Marshal(tr)
	cwd := t.TempDir()
	f.chats["old"] = &api.ChatDetail{
		Chat: api.Chat{ID: "old", Title: "Earlier work", Provider: "p1", Model: "m-2", WorkspaceDir: cwd, UpdatedAt: time.Now().UnixMilli()},
		Messages: []api.Message{
			{ID: "1", Role: "user", Content: "write a.go"},
			{ID: "2", Role: "assistant", Content: "All done.", Trace: raw, ThinkingMs: 300, DurationMs: 2500},
		},
	}
	h, closeFn := boot(t, f)
	defer closeFn()
	h.m.setCwd(cwd)

	h.m.app.resume = false
	h.send(SlashSelectMsg{Command: SlashCommand{Name: "/continue"}})
	if !h.m.list.IsOpen() || h.m.list.Kind != "sessions" {
		t.Fatal("sessions list should be open")
	}
	if !strings.Contains(h.screen(), "Earlier work") {
		t.Errorf("sessions list is missing the chat:\n%s", h.screen())
	}
	h.send(tea.KeyPressMsg{Code: tea.KeyEnter})

	if h.m.chat == nil || h.m.chat.ID != "old" || len(h.m.messages) != 2 {
		t.Fatalf("chat=%+v messages=%d", h.m.chat, len(h.m.messages))
	}
	if h.m.selModel != "m-2" {
		t.Errorf("the chat's model should be selected, got %q", h.m.selModel)
	}
	a := h.m.messages[1]
	if len(a.Parts) != 3 || a.Parts[0].Kind != PartThought || a.Parts[0].Ms != 300 || a.Parts[1].Kind != PartEdit || a.Parts[2].Text != "All done." {
		t.Errorf("history parts: %+v", a.Parts)
	}
	if !strings.Contains(h.screen(), "All done.") {
		t.Errorf("history not on screen:\n%s", h.screen())
	}
}

func TestProviderFormAddsAKeyAndModelsAppear(t *testing.T) {
	f := newFakeAPI()
	f.providers = []api.ProviderRow{{ID: "p1", Name: "Prov", BaseURL: "https://api.example/v1"}}
	h, closeFn := boot(t, f)
	defer closeFn()

	if h.m.selModel != "" {
		t.Fatal("no model expected yet")
	}
	h.send(SlashSelectMsg{Command: SlashCommand{Name: "/connect"}})
	if !h.m.list.IsOpen() || !strings.Contains(h.screen(), "Prov") {
		t.Fatalf("providers list not shown:\n%s", h.screen())
	}
	h.send(tea.KeyPressMsg{Code: tea.KeyDown}) // skip "+ Add custom provider"
	h.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !h.m.provForm.IsOpen() {
		t.Fatal("form should open")
	}
	h.send(tea.KeyPressMsg{Code: tea.KeyTab}) // base URL -> API key
	for _, r := range "sk-secret" {
		h.send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if strings.Contains(h.screen(), "sk-secret") {
		t.Error("the API key must be masked on screen")
	}
	h.send(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

	f.mu.Lock()
	key, en := f.providers[0].APIKey, f.providers[0].Enabled
	f.mu.Unlock()
	if key != "sk-secret" || !en {
		t.Errorf("saved key=%q enabled=%v", key, en)
	}
	if h.m.selModel != "m-1" || h.m.prompt.Model != "Model One" {
		t.Errorf("models were not refreshed: %q %q", h.m.selModel, h.m.prompt.Model)
	}
}

func TestMCPToggleHitsTheBackend(t *testing.T) {
	f := newFakeAPI()
	f.models = oneModel()
	f.connect = []api.Connector{{ID: "c1", Name: "Excel"}}
	h, closeFn := boot(t, f)
	defer closeFn()

	h.send(SlashSelectMsg{Command: SlashCommand{Name: "/mcps"}})
	h.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !h.m.connectors[0].Enabled || !h.m.list.IsOpen() {
		t.Fatalf("connector should be enabled and the list still open: %+v", h.m.connectors)
	}
	if h.m.enabledMCPs() != 1 {
		t.Error("count not updated")
	}
}

func TestRetryRegeneratesInsteadOfAddingAMessage(t *testing.T) {
	f := newFakeAPI()
	f.models = oneModel()
	f.script = func(w http.ResponseWriter, r *http.Request, emit func(string, any)) {
		emit("chat", api.Chat{ID: "chat-1"})
		emit("token", map[string]string{"text": "answer"})
		emit("done", api.Done{Message: api.Message{ID: "a", Role: "assistant", Content: "answer"}})
	}
	h, closeFn := boot(t, f)
	defer closeFn()
	h.typeAndEnter("question")
	h.send(SlashSelectMsg{Command: SlashCommand{Name: "/retry"}})

	if len(f.sent) != 2 || !f.sent[1].Regenerate || f.sent[1].Content != "question" {
		t.Fatalf("sent = %+v", f.sent)
	}
	users := 0
	for _, m := range h.m.messages {
		if m.Role == RoleUser {
			users++
		}
	}
	if users != 1 || len(h.m.messages) != 2 {
		t.Errorf("messages after retry: %d (users %d)", len(h.m.messages), users)
	}
}

func TestBackendDownShowsRetryScreen(t *testing.T) {
	msgCache = map[int]cachedMsg{}
	t.Setenv(settings.EnvDir, t.TempDir())
	srv := newFakeAPI().serve()
	addr := srv.URL
	srv.Close()
	app := &appCtx{client: api.New(addr), st: &settings.Settings{}}
	h := &harness{t: t, m: NewModel(app)}
	h.send(tea.WindowSizeMsg{Width: 100, Height: 30})
	for _, msg := range runCmd(bootCmd(app)) {
		h.send(msg)
	}
	if h.m.boot != bootFailed || !strings.Contains(h.screen(), "Can't reach") {
		t.Errorf("expected the unreachable screen:\n%s", h.screen())
	}
}
