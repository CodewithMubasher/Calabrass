package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/ai"
	"github.com/CodewithMubasher/Calabrass/backend/internal/crew"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// The first user turn is the objective. It must stay in front of the window even
// when the conversation is long enough that the tail no longer includes it.
func TestHistoryWindowPinsTheFirstUserTurn(t *testing.T) {
	history := make([]store.Message, 0, 30)
	history = append(history, store.Message{Role: ai.RoleUser, Content: "the original objective"})
	for i := 1; i < 30; i++ {
		role := ai.RoleAssistant
		if i%2 == 0 {
			role = ai.RoleUser
		}
		history = append(history, store.Message{Role: role, Content: "later"})
	}

	window := historyWindow(history, 20)
	if len(window) != 21 {
		t.Fatalf("window has %d messages, want the pinned turn plus 20", len(window))
	}
	if window[0].Content != "the original objective" {
		t.Fatalf("the window starts with %q, not the original objective", window[0].Content)
	}
	if window[len(window)-1].Content != history[len(history)-1].Content {
		t.Fatalf("the window does not end at the newest message")
	}
}

func TestHistoryWindowDoesNotDuplicateAYoungConversation(t *testing.T) {
	history := make([]store.Message, 0, 3)
	history = append(history,
		store.Message{Role: ai.RoleUser, Content: "one"},
		store.Message{Role: ai.RoleAssistant, Content: "two"},
		store.Message{Role: ai.RoleUser, Content: "three"},
	)
	window := historyWindow(history, 20)
	if len(window) != len(history) {
		t.Fatalf("window has %d messages, want all %d", len(window), len(history))
	}
	if window[0].Content != "one" {
		t.Fatalf("window starts with %q", window[0].Content)
	}
	if got := historyWindow(history, 0); got != nil {
		t.Fatalf("a zero window returned %d messages", len(got))
	}
}

// A run stopped by its tool budget must say so in the answer it saves.
func TestRunLimitNoteNamesTheLimit(t *testing.T) {
	state := agent.NewRunState(agent.Budget{MaxToolCalls: 1, MaxConsecutiveFail: 3})
	if ok, _ := state.BeforeToolCall("read_file", map[string]any{"path": "a"}); !ok {
		t.Fatal("the first call was refused")
	}
	if ok, _ := state.BeforeToolCall("read_file", map[string]any{"path": "b"}); ok {
		t.Fatal("the second call was allowed past a one-call budget")
	}

	note := runLimitNote(ai.Reply{Steps: 4}, state)
	if !strings.Contains(note, "tool budget") {
		t.Fatalf("note = %q, want the tool budget", note)
	}

	stepNote := runLimitNote(ai.Reply{Steps: 40, StepsExhausted: true}, nil)
	if !strings.Contains(stepNote, "step limit") || !strings.Contains(stepNote, "40") {
		t.Fatalf("step note = %q", stepNote)
	}

	if got := runLimitNote(ai.Reply{Steps: 4}, agent.NewRunState(agent.DefaultBudget(agent.ModeChat))); got != "" {
		t.Fatalf("an unlimited run produced the note %q", got)
	}
}

// Switching a chat between modes has to leave a usable binding behind. The
// Orchestrator is crew-only; an ordinary chat with it would be stuck with a
// delegate-only prompt.
func TestSwitchingModesKeepsTheAgentBindingUsable(t *testing.T) {
	server, token := modeTestServer(t)
	addProvider(t, server, token)

	chat, status := createChat(t, server, token, `{"mode":"crew"}`)
	if status != http.StatusCreated {
		t.Fatalf("creating a crew chat answered %d", status)
	}
	if chat.AgentID != crew.BuiltinOrchestratorID {
		t.Fatalf("a crew chat with no agent is bound to %q, want the Orchestrator", chat.AgentID)
	}

	chat = updateChat(t, server, token, chat.ID, `{"mode":"chat"}`)
	if chat.Mode != agent.ModeChat {
		t.Fatalf("mode = %q, want chat", chat.Mode)
	}
	if chat.AgentID != "" {
		t.Fatalf("leaving crew kept the agent %q, want the orchestrator binding cleared", chat.AgentID)
	}

	chat = updateChat(t, server, token, chat.ID, `{"mode":"crew"}`)
	if chat.Mode != agent.ModeCrew {
		t.Fatalf("mode = %q, want crew", chat.Mode)
	}
	if chat.AgentID != crew.BuiltinOrchestratorID {
		t.Fatalf("returning to crew bound the chat to %q, want the Orchestrator", chat.AgentID)
	}
}

func updateChat(t *testing.T, server *httptest.Server, token, chatID, body string) store.Chat {
	t.Helper()

	req, err := http.NewRequest(http.MethodPatch, server.URL+"/api/chats/"+chatID, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("patch chat: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("updating a chat answered %d", res.StatusCode)
	}

	var payload struct {
		Chat store.Chat `json:"chat"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("read the answer: %v", err)
	}
	return payload.Chat
}
