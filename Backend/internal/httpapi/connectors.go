package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// listConnectors answers with the full list, in the same shape the providers and
// workspaces endpoints use, so one call refreshes the page after any change.
func (s *Server) listConnectors(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListConnectors(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectors": rows})
}

func (s *Server) handleListConnectors(w http.ResponseWriter, r *http.Request) {
	s.listConnectors(w, r)
}

// handleCreateConnector registers an MCP server. Nothing starts it yet: the row
// is the record the agent will read once connectors are wired into a turn.
func (s *Server) handleCreateConnector(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Command string `json:"command"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	command := strings.TrimSpace(req.Command)
	if name == "" || command == "" {
		writeError(w, http.StatusBadRequest, errors.New("a name and a command are required"))
		return
	}

	if _, err := s.store.CreateConnector(r.Context(), name, command); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.listConnectors(w, r)
}

// handleUpdateConnector turns a connector on or off.
func (s *Server) handleUpdateConnector(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Enabled == nil {
		writeError(w, http.StatusBadRequest, errors.New("enabled is required"))
		return
	}

	_, err := s.store.SetConnectorEnabled(r.Context(), r.PathValue("connectorID"), *req.Enabled)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.listConnectors(w, r)
}

func (s *Server) handleDeleteConnector(w http.ResponseWriter, r *http.Request) {
	err := s.store.DeleteConnector(r.Context(), r.PathValue("connectorID"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.listConnectors(w, r)
}
