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

// handleCreateConnector registers an MCP server. The row is the record the
// agent reads before a turn, and it starts the server the first time a tool of
// its own is needed.
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

	id := r.PathValue("connectorID")
	_, err := s.store.SetConnectorEnabled(r.Context(), id, *req.Enabled)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// A connector that was turned off is stopped as well as ignored, so the
	// switch does what it says rather than only what the next turn reads.
	s.connectors.Forget(id)
	s.listConnectors(w, r)
}

func (s *Server) handleDeleteConnector(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("connectorID")
	err := s.store.DeleteConnector(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.connectors.Forget(id)
	s.listConnectors(w, r)
}

// handleConnectorTools answers with the tools a connector offers, which is what
// makes a connector more than a line of text in a form.
//
// Starting a server can fail — a mistyped command, a binary that is not built —
// and the reason is the answer the page needs, so it comes back as a 200 with an
// error beside an empty tool list rather than as a failed request.
func (s *Server) handleConnectorTools(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("connectorID")

	rows, err := s.store.ListConnectors(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var record *store.ConnectorRecord
	for i := range rows {
		if rows[i].ID == id {
			record = &rows[i]
			break
		}
	}
	if record == nil {
		writeError(w, http.StatusNotFound, errors.New("no connector with that id"))
		return
	}

	tools, err := s.connectors.Inspect(r.Context(), *record)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"tools": []any{}, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
}
