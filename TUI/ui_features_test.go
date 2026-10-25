package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestLastResponseTextPrefersProse(t *testing.T) {
	msgs := []Message{
		{Role: RoleAssistant, Parts: []Part{
			{Kind: PartThought, Text: "internal reasoning"},
			{Kind: PartText, Text: " first answer "},
		}},
		{Role: RoleUser, Text: "next"},
		{Role: RoleAssistant, Parts: []Part{
			{Kind: PartThought, Text: "do not copy this"},
			{Kind: PartText, Text: "**last answer**"},
			{Kind: PartTool, Tool: "read_file", Label: "Read main.go"},
		}},
	}

	got, ok := lastResponseText(msgs)
	if !ok {
		t.Fatal("expected a response")
	}
	if got != "**last answer**" {
		t.Fatalf("copied %q, want the newest assistant prose", got)
	}
}

func TestLastResponseTextFallsBackToFiles(t *testing.T) {
	msgs := []Message{{Role: RoleAssistant, Parts: []Part{
		{Kind: PartEdit, File: "main.go", Lang: "go", Code: "package main\n"},
	}}}
	got, ok := lastResponseText(msgs)
	if !ok {
		t.Fatal("expected a response")
	}
	if !strings.Contains(got, "```go") || !strings.Contains(got, "package main") {
		t.Fatalf("file fallback = %q", got)
	}
	if _, ok := lastResponseText([]Message{{Role: RoleUser, Text: "hi"}}); ok {
		t.Fatal("a user message is not a response")
	}
}

func TestCopyCommandCopiesNewestAnswer(t *testing.T) {
	old := writeClipboard
	defer func() { writeClipboard = old }()
	var copied string
	writeClipboard = func(s string) error {
		copied = s
		return nil
	}

	f := newFakeAPI()
	h, closeFn := boot(t, f)
	defer closeFn()

	h.m.messages = []Message{{ID: 1, Role: RoleAssistant, Parts: []Part{
		{Kind: PartText, Text: "hello world"},
	}}}
	h.m.nextID = 1
	h.m.screen = screenChat

	h.typeAndEnter("/copy")

	if copied != "hello world" {
		t.Fatalf("clipboard got %q", copied)
	}
	if !strings.Contains(h.screen(), "Copied the last response.") {
		t.Fatalf("missing copy confirmation:\n%s", h.screen())
	}
}

func TestHelpOnlyShowsAToast(t *testing.T) {
	f := newFakeAPI()
	h, closeFn := boot(t, f)
	defer closeFn()

	h.typeAndEnter("/help")

	if h.m.screen != screenHome {
		t.Fatalf("help left the home screen (screen = %v)", h.m.screen)
	}
	if len(h.m.messages) != 0 {
		t.Fatalf("help dumped %d messages into the chat", len(h.m.messages))
	}
	if !strings.Contains(h.screen(), "Press ctrl+p to view all commands") {
		t.Fatalf("help toast is missing:\n%s", h.screen())
	}
	if !strings.Contains(h.screen(), "v1.0.4") {
		t.Errorf("home status bar does not show v1.0.4:\n%s", h.screen())
	}
}

func TestHoldDownOpensQuickActions(t *testing.T) {
	f := newFakeAPI()
	h, closeFn := boot(t, f)
	defer closeFn()

	down := tea.KeyPressMsg{Code: tea.KeyDown}

	// A single tap must not open the menu.
	model, cmd := h.m.Update(down)
	h.m = model.(Model)
	if cmd == nil {
		t.Fatal("first ↓ press should start the hold timer")
	}
	model, _ = h.m.Update(holdTickMsg{seq: h.m.holdSeq})
	h.m = model.(Model)
	if h.m.quick.IsOpen() {
		t.Fatal("a tap opened the quick actions menu")
	}

	// Holding produces repeated ↓ events, then the timer fires.
	for i := 0; i < 5; i++ {
		model, _ = h.m.Update(down)
		h.m = model.(Model)
	}
	model, _ = h.m.Update(holdTickMsg{seq: h.m.holdSeq})
	h.m = model.(Model)

	if !h.m.quick.IsOpen() {
		t.Fatal("holding ↓ did not open quick actions")
	}
	scr := h.screen()
	for _, want := range []string{"Quick actions", "Copy last response", "Help", "enter run"} {
		if !strings.Contains(scr, want) {
			t.Errorf("quick actions is missing %q:\n%s", want, scr)
		}
	}

	model, _ = h.m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	h.m = model.(Model)
	if h.m.quick.IsOpen() {
		t.Fatal("esc did not dismiss quick actions")
	}
}

func TestToastSitsInTheChatTopRightAndExpires(t *testing.T) {
	f := newFakeAPI()
	h, closeFn := boot(t, f)
	defer closeFn()

	h.m.screen = screenChat
	h.m.showToast("Toast works", false)

	lines := strings.Split(h.screen(), "\n")
	if len(lines) < toastTop+2 {
		t.Fatalf("screen too short for the toast:\n%s", h.screen())
	}
	// The toast has one blank row above the text, and a margin above that.
	if strings.Count(lines[toastTop], "│") < 2 {
		t.Fatalf("toast has no blank top row:\n%s", h.screen())
	}
	if !strings.Contains(lines[toastTop+1], "│ Toast works") || strings.Count(lines[toastTop+1], "│") < 2 {
		t.Fatalf("toast text is not in the middle row:\n%s", h.screen())
	}
	if strings.Count(lines[toastTop+2], "│") < 2 {
		t.Fatalf("toast has no blank bottom row:\n%s", h.screen())
	}

	// An older timer must not clear a newer toast.
	h.m.showToast("Second toast", false)
	h.send(toastMsg{seq: h.m.toastSeq - 1})
	if h.m.toastText != "Second toast" {
		t.Fatalf("stale timer cleared the toast: %q", h.m.toastText)
	}
	h.send(toastMsg{seq: h.m.toastSeq})
	if h.m.toastText != "" {
		t.Fatalf("toast did not expire: %q", h.m.toastText)
	}
}

func TestModelSwitchShowsAToast(t *testing.T) {
	f := newFakeAPI()
	f.models = oneModel()
	h, closeFn := boot(t, f)
	defer closeFn()

	h.send(SlashSelectMsg{Command: SlashCommand{Name: "/models"}})
	h.send(tea.KeyPressMsg{Code: tea.KeyDown}) // current -> Model Two
	h.send(tea.KeyPressMsg{Code: tea.KeyEnter})

	if h.m.selModel != "m-2" {
		t.Fatalf("selected model = %q", h.m.selModel)
	}
	if h.m.toastText != "Model switched to Model Two" {
		t.Fatalf("toast = %q", h.m.toastText)
	}
}

func TestProviderSaveShowsAToast(t *testing.T) {
	f := newFakeAPI()
	h, closeFn := boot(t, f)
	defer closeFn()

	h.send(providerSavedMsg{created: true})
	if h.m.toastText != "Provider added" {
		t.Fatalf("toast = %q", h.m.toastText)
	}
	h.send(providerSavedMsg{created: false})
	if h.m.toastText != "Provider saved" {
		t.Fatalf("toast = %q", h.m.toastText)
	}
}

func TestModelPickerGroupsByProvider(t *testing.T) {
	m := Model{
		selProvider: "p1", selModel: "m1",
		models: []modelEntry{
			{ProviderID: "p1", ProviderName: "Alpha", ID: "m1", Label: "Model One"},
			{ProviderID: "p1", ProviderName: "Alpha", ID: "m2", Label: "Model Two"},
			{ProviderID: "p2", ProviderName: "Beta", ID: "m3", Label: "Model Three"},
		},
	}
	items := m.modelItems()
	if len(items) != 3 {
		t.Fatalf("model items = %d", len(items))
	}
	if items[0].Section != "Alpha" || items[0].Right != "Alpha" || items[0].Title != "✓ Model One" {
		t.Errorf("current model row = %+v", items[0])
	}
	if items[1].Section != "Alpha" || items[2].Section != "Beta" {
		t.Errorf("sections = %q, %q, %q", items[0].Section, items[1].Section, items[2].Section)
	}

	d := NewListDialog()
	d.SetSize(140, 40)
	d.Show("models", "Select model", items)
	scr := ansi.Strip(d.View())
	for _, want := range []string{"Alpha  2 models", "Beta  1 model", "Model One", "Model Three"} {
		if !strings.Contains(scr, want) {
			t.Errorf("grouped model list is missing %q:\n%s", want, scr)
		}
	}

	// Searching a provider keeps its whole group.
	d.input.SetValue("beta")
	d.refilter()
	if len(d.filtered) != 1 || d.filtered[0].ID != "p2\x00m3" {
		t.Fatalf("provider search filtered to %+v", d.filtered)
	}
}
