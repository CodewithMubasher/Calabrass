package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// listAgents answers with the full list, in the shape the other admin pages
// use, so one call refreshes the page after any change.
func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListAgents(r.Context(), currentUser(r.Context()).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": rows})
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	s.listAgents(w, r)
}

// agentRequest is the form the Agents page writes: everything an agent needs to
// be handed work, all of it optional except the name.
type agentRequest struct {
	Name        string `json:"name"`
	Prompt      string `json:"prompt"`
	Mode        string `json:"mode"`
	ConnectorID string `json:"connectorId"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	AltProvider string `json:"altProvider"`
	AltModel    string `json:"altModel"`
}

// record validates the request and turns it into a stored row. Only the name is
// required, and the mode falls back to "build" — the same default the composer
// starts on — so a half-filled form still saves something usable.
func (a agentRequest) record() (store.AgentRecord, error) {
	name := strings.TrimSpace(a.Name)
	if name == "" {
		return store.AgentRecord{}, errors.New("a name is required")
	}

	mode := strings.TrimSpace(a.Mode)
	switch mode {
	case "":
		mode = "build"
	case "build", "plan":
	default:
		return store.AgentRecord{}, errors.New(`mode must be "build" or "plan"`)
	}

	return store.AgentRecord{
		Name:        name,
		Prompt:      strings.TrimSpace(a.Prompt),
		Mode:        mode,
		ConnectorID: strings.TrimSpace(a.ConnectorID),
		Provider:    strings.TrimSpace(a.Provider),
		Model:       strings.TrimSpace(a.Model),
		AltProvider: strings.TrimSpace(a.AltProvider),
		AltModel:    strings.TrimSpace(a.AltModel),
	}, nil
}

// handleCreateAgent stores a new sub-agent.
func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	var req agentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	record, err := req.record()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if _, err := s.store.CreateAgent(r.Context(), record, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.listAgents(w, r)
}

// handleUpdateAgent saves an agent in full: the dialog is one form, so it writes
// every field it shows.
func (s *Server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	var req agentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	record, err := req.record()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	_, err = s.store.UpdateAgent(r.Context(), r.PathValue("agentID"), record, user.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.listAgents(w, r)
}

func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	err := s.store.DeleteAgent(r.Context(), r.PathValue("agentID"), currentUser(r.Context()).ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.listAgents(w, r)
}
