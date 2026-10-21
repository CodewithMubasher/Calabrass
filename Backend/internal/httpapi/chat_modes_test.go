package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CodewithMubasher/Calabrass/backend/internal/ai"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// modeTestServer builds a server over a fresh database, with the provider
// registry the real app wires up: the stored provider rows, scoped to one
// account.
func modeTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	registry := ai.NewRegistry(
		func(ctx context.Context, userID string) ([]ai.ProviderRecord, error) {
			rows, err := st.ListProviders(ctx, userID)
			if err != nil {
				return nil, err
			}
			records := make([]ai.ProviderRecord, 0, len(rows))
			for _, row := range rows {
				records = append(records, ai.ProviderRecord{
					ID:      row.ID,
					Name:    row.Name,
					BaseURL: row.BaseURL,
					APIKey:  row.APIKey,
					Enabled: row.Enabled,
				})
			}
			return records, nil
		})

	server := httptest.NewServer(NewServer(Deps{Store: st, Logger: slog.Default(), Registry: registry}))
	t.Cleanup(server.Close)

	return server, signUpAndGetToken(t, server)
}

// addProvider gives the account a provider with one model, so a chat can be
// created at all. The endpoint is a local one that answers the model listing.
func addProvider(t *testing.T, server *httptest.Server, token string) {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"test-model"}]}`))
	}))
	t.Cleanup(upstream.Close)

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/providers",
		strings.NewReader(`{"name":"Test","baseUrl":"`+upstream.URL+`","apiKey":"k"}`))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("post provider: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		t.Fatalf("adding a provider answered %d", res.StatusCode)
	}
}

// createChat makes one chat through the API and returns what the server
// answered with.
func createChat(t *testing.T, server *httptest.Server, token, body string) (store.Chat, int) {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/chats", strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("post chat: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	var payload struct {
		Chat store.Chat `json:"chat"`
	}
	if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusCreated {
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			t.Fatalf("read the answer: %v", err)
		}
	}
	return payload.Chat, res.StatusCode
}

// A chat is created in the mode it asks for: an ordinary one is a chat, a crew
// run is a crew run.
func TestChatModeIsKept(t *testing.T) {
	server, token := modeTestServer(t)
	addProvider(t, server, token)

	chat, status := createChat(t, server, token, `{"mode":"crew"}`)
	if status != http.StatusCreated {
		t.Fatalf("creating a crew chat answered %d", status)
	}
	if chat.Mode != "crew" {
		t.Fatalf("the chat came back in mode %q, want crew", chat.Mode)
	}

	chat, status = createChat(t, server, token, `{"mode":"chat"}`)
	if status != http.StatusCreated {
		t.Fatalf("creating a chat answered %d", status)
	}
	if chat.Mode != "chat" {
		t.Fatalf("the chat came back in mode %q, want chat", chat.Mode)
	}
}

// A chat with no mode at all is an ordinary one: the old names are gone, and
// anything this build does not know is refused rather than stored.
func TestChatModeDefaultsAndRefusesUnknownModes(t *testing.T) {
	server, token := modeTestServer(t)
	addProvider(t, server, token)

	chat, status := createChat(t, server, token, `{}`)
	if status != http.StatusCreated {
		t.Fatalf("creating a chat answered %d", status)
	}
	if chat.Mode != "chat" {
		t.Fatalf("the chat came back in mode %q, want the chat default", chat.Mode)
	}

	for _, mode := range []string{"build", "plan", "think"} {
		if _, status := createChat(t, server, token, `{"mode":"`+mode+`"}`); status != http.StatusBadRequest {
			t.Errorf("the mode %q was answered with %d, want 400", mode, status)
		}
	}
}

// An agent's mode is the default a chat of its own starts in. A chat that says
// which mode it wants keeps it.
func TestAgentModeIsTheDefaultForItsChats(t *testing.T) {
	server, token := modeTestServer(t)
	addProvider(t, server, token)

	agents := callAgents(t, server, token, http.MethodPost, "/api/agents",
		`{"name":"Builder","prompt":"Do the work.","mode":"crew"}`)
	if len(agents) != 1 {
		t.Fatalf("creating an agent answered with %d of them", len(agents))
	}
	agentID := agents[0].ID

	chat, status := createChat(t, server, token, `{"agentId":"`+agentID+`"}`)
	if status != http.StatusCreated {
		t.Fatalf("creating a chat answered %d", status)
	}
	if chat.Mode != "crew" {
		t.Fatalf("the chat came back in mode %q, want the agent's crew", chat.Mode)
	}

	chat, status = createChat(t, server, token, `{"agentId":"`+agentID+`","mode":"chat"}`)
	if status != http.StatusCreated {
		t.Fatalf("creating a chat answered %d", status)
	}
	if chat.Mode != "chat" {
		t.Fatalf("the chat came back in mode %q, want the mode it asked for", chat.Mode)
	}
}
