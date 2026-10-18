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

	agents := callAgents(t, server, http.MethodPost, "/api/agents",
		`{"name":"Researcher","prompt":"Find the primary sources.","mode":"plan",
		  "connectorId":"c1","provider":"google","model":"gemini-2.5-pro",
		  "altProvider":"groq","altModel":"llama-3.3-70b"}`)
	if len(agents) != 1 {
		t.Fatalf("creating an agent answered with %d of them", len(agents))
	}
	agent := agents[0]
	if agent.Name != "Researcher" || agent.Mode != "plan" || agent.Model != "gemini-2.5-pro" {
		t.Fatalf("the agent came back as %+v", agent)
	}
	if agent.ConnectorID != "c1" || agent.AltProvider != "groq" || agent.AltModel != "llama-3.3-70b" {
		t.Fatalf("the connector or fallback model was lost: %+v", agent)
	}
	if agent.CreatedAt == 0 || agent.UpdatedAt == 0 {
		t.Errorf("the agent has no timestamps: %+v", agent)
	}

	agents = callAgents(t, server, http.MethodGet, "/api/agents", "")
	if len(agents) != 1 {
		t.Fatalf("the list has %d agents", len(agents))
	}

	agents = callAgents(t, server, http.MethodPut, "/api/agents/"+agent.ID,
		`{"name":"Researcher","prompt":"Find and cite.","mode":"build"}`)
	if len(agents) != 1 {
		t.Fatalf("saving it answered with %d of them", len(agents))
	}
	if agents[0].Prompt != "Find and cite." || agents[0].Mode != "build" {
		t.Fatalf("the save did not take: %+v", agents[0])
	}
	// The dialog is one form, so a save writes every field it shows; a model the
	// request leaves out is cleared rather than kept.
	if agents[0].Model != "" || agents[0].ConnectorID != "" {
		t.Errorf("a field left out of the save survived: %+v", agents[0])
	}

	agents = callAgents(t, server, http.MethodDelete, "/api/agents/"+agent.ID, "")
	if len(agents) != 0 {
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
	if rec.Mode != "build" {
		t.Errorf("got mode %q, want the build default", rec.Mode)
	}
}

// callAgents makes one request and returns the list it answers with.
func callAgents(t *testing.T, server *httptest.Server, method, path, body string) []store.AgentRecord {
	t.Helper()

	req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")

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
