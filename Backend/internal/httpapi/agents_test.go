package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// An agent is created with everything the dialog offers, listed, saved in full
// and removed again: the same round trip the Agents page makes.
func TestAgentEndpoints(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	server := httptest.NewServer(NewServer(Deps{Store: st, Logger: slog.Default()}))
	t.Cleanup(server.Close)

	// Sign up a test user and get a token for authenticated requests
	token := signUpAndGetToken(t, server)

	agents := callAgents(t, server, token, http.MethodPost, "/api/agents",
		`{"name":"Researcher","prompt":"Find the primary sources.","mode":"crew",
		  "connectorId":"c1","provider":"google","model":"gemini-2.5-pro",
		  "altProvider":"groq","altModel":"llama-3.3-70b"}`)
	if len(agents) != 4 {
		t.Fatalf("creating an agent answered with %d of them", len(agents))
	}
	// The list includes the built-in orchestrator, the built-in workers, and the one we just created.
	var created store.AgentRecord
	for _, a := range agents {
		if a.Name == "Researcher" {
			created = a
			break
		}
	}
	if created.Name == "" {
		t.Fatalf("the created agent was not in the list")
	}
	if created.Name != "Researcher" || created.Mode != "crew" || created.Model != "gemini-2.5-pro" {
		t.Fatalf("the agent came back as %+v", created)
	}
	if created.ConnectorID != "c1" || created.AltProvider != "groq" || created.AltModel != "llama-3.3-70b" {
		t.Fatalf("the connector or fallback model was lost: %+v", created)
	}
	if created.CreatedAt == 0 || created.UpdatedAt == 0 {
		t.Errorf("the agent has no timestamps: %+v", created)
	}

	agents = callAgents(t, server, token, http.MethodGet, "/api/agents", "")
	if len(agents) != 4 {
		t.Fatalf("the list has %d agents", len(agents))
	}

	agents = callAgents(t, server, token, http.MethodPut, "/api/agents/"+created.ID,
		`{"name":"Researcher","prompt":"Find and cite.","mode":"chat"}`)
	if len(agents) != 4 {
		t.Fatalf("saving it answered with %d of them", len(agents))
	}
	// The saved agent is in the list alongside the built-ins.
	var updated store.AgentRecord
	for _, a := range agents {
		if a.ID == created.ID {
			updated = a
			break
		}
	}
	if updated.Prompt != "Find and cite." || updated.Mode != "chat" {
		t.Fatalf("the save did not take: %+v", updated)
	}
	// The dialog is one form, so a save writes every field it shows; a model the
	// request leaves out is cleared rather than kept.
	if updated.Model != "" || updated.ConnectorID != "" {
		t.Errorf("a field left out of the save survived: %+v", updated)
	}

	agents = callAgents(t, server, token, http.MethodDelete, "/api/agents/"+created.ID, "")
	// The built-ins remain.
	if len(agents) != 3 {
		t.Fatalf("after removing it the list has %d agents", len(agents))
	}
}

// The name is the only thing an agent cannot do without, and the mode has to be
// one the composer knows. Both are refused with a reason instead of being saved.
func TestAgentRequestValidation(t *testing.T) {
	if _, err := (agentRequest{Name: "   "}).record(); err == nil {
		t.Error("an agent with no name was accepted")
	}
	if _, err := (agentRequest{Name: "X", Mode: "think"}).record(); err == nil {
		t.Error("an unknown mode was accepted")
	}

	rec, err := (agentRequest{Name: "  Solo  ", Prompt: "  Keep it short.  "}).record()
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if rec.Name != "Solo" || rec.Prompt != "Keep it short." {
		t.Errorf("got %+v, want the name and prompt trimmed", rec)
	}
	if rec.Mode != "chat" {
		t.Errorf("got mode %q, want the chat default", rec.Mode)
	}
}

// callAgents makes one request and returns the list it answers with.
func callAgents(t *testing.T, server *httptest.Server, token, method, path, body string) []store.AgentRecord {
	t.Helper()

	req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s %s answered %s", method, path, res.Status)
	}

	var payload struct {
		Agents []store.AgentRecord `json:"agents"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("read the answer to %s %s: %v", method, path, err)
	}
	return payload.Agents
}
