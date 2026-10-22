package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/crew"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// agentRow is one agent as the pages see it: the stored record, plus whether
// it is one of the built-ins the app ships with. Built-ins arrive even when
// nothing about them was ever saved, which is why the flag rides outside the
// record.
type agentRow struct {
	store.AgentRecord
	Builtin bool `json:"builtin"`
}

// listAgents answers with the full list — the built-ins the app ships with,
// overridden by any saved row with the same id, and then the custom agents —
// in the shape the other admin pages use, so one call refreshes the page
// after any change.
func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	views, err := s.agentRowsFor(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": views})
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	s.listAgents(w, r)
}

// agentRowsFor merges the shipped built-ins with what this account has saved:
// a database row with a built-in's id replaces that built-in's fields, one for
// one; rows the app does not ship are custom agents, carried as they are.
func (s *Server) agentRowsFor(ctx context.Context, userID string) ([]agentRow, error) {
	rows, err := s.store.ListAgents(ctx, userID)
	if err != nil {
		return nil, err
	}

	byID := make(map[string]store.AgentRecord, len(rows))
	for _, rec := range rows {
		byID[rec.ID] = rec
	}

	views := []agentRow{}
	now := time.Now().UnixMilli()
	for _, b := range crew.Builtins() {
		rec, ok := byID[b.ID]
		if ok {
			delete(byID, b.ID)
		} else {
			rec = store.AgentRecord{
				ID:          b.ID,
				Name:        b.Name,
				Prompt:      b.Persona(),
				SystemPrompt: "",
				Description: b.Description,
				Mode:        b.Mode,
				ConnectorID: "",
				Provider:    "",
				Model:       "",
				CreatedAt:   now,
				UpdatedAt:   now,
			}
		}
		views = append(views, agentRow{AgentRecord: rec, Builtin: true})
	}
	for _, rec := range byID {
		views = append(views, agentRow{AgentRecord: rec, Builtin: crew.IsBuiltinID(rec.ID)})
	}

	return views, nil
}

// agentRequest is the form the Agents page writes: everything an agent needs to
// be handed work, all of it optional except the name.
type agentRequest struct {
	Name           string `json:"name"`
	Prompt         string `json:"prompt"`
	SystemPrompt   string `json:"systemPrompt"`
	Mode           string `json:"mode"`
	Description    string `json:"description"`
	ConnectorID    string `json:"connectorId"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	AltProvider    string `json:"altProvider"`
	AltModel       string `json:"altModel"`
}

// record validates the request and turns it into a stored row. Only the name is
// required, and the mode falls back to "chat" — the same default the composer
// starts on — so a half-filled form still saves something usable.
func (a agentRequest) record() (store.AgentRecord, error) {
	name := strings.TrimSpace(a.Name)
	if name == "" {
		return store.AgentRecord{}, errors.New("a name is required")
	}

	mode := agent.NormalMode(a.Mode)
	if mode == "" {
		if strings.TrimSpace(a.Mode) != "" {
			return store.AgentRecord{}, errors.New(`mode must be "chat" or "crew"`)
		}
		mode = agent.ModeChat
	}

	systemPrompt := strings.TrimSpace(a.SystemPrompt)

	return store.AgentRecord{
		Name:           name,
		Prompt:         strings.TrimSpace(a.Prompt),
		SystemPrompt:   systemPrompt,
		Mode:           mode,
		Description:    strings.TrimSpace(a.Description),
		ConnectorID:    strings.TrimSpace(a.ConnectorID),
		Provider:       strings.TrimSpace(a.Provider),
		Model:          strings.TrimSpace(a.Model),
		AltProvider:    strings.TrimSpace(a.AltProvider),
		AltModel:       strings.TrimSpace(a.AltModel),
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

// handleUpsertAgent writes an agent with a specific id, whether or not it was
// ever in the database. This is how edits to built-ins work: the built-in's
// fields are used until the account saves an override, and this writes that
// override without creating a second agent.
func (s *Server) handleUpsertAgent(w http.ResponseWriter, r *http.Request) {
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

	agentID := r.PathValue("agentID")
	if _, err = s.store.UpsertAgent(r.Context(), agentID, record, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.listAgents(w, r)
}

// handleUpdateAgent saves an agent in full: the dialog is one form, so it writes
// every field it shows. Uses upsert so built-ins can be edited.
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

	agentID := r.PathValue("agentID")
	if _, err = s.store.UpsertAgent(r.Context(), agentID, record, user.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.listAgents(w, r)
}

// handleDeleteAgent removes an agent. A built-in is refused: it ships with the
// app and only its changes can be rolled back, which is what reset does.
func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	if crew.IsBuiltinID(agentID) {
		writeError(w, http.StatusBadRequest, errors.New(
			"built-in agents cannot be deleted; reset the agent instead"))
		return
	}

	err := s.store.DeleteAgent(r.Context(), agentID, currentUser(r.Context()).ID)
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

// handleResetAgent drops an agent's override row so its fields go back to
// what the app shipped. Only built-ins have anything to reset: a custom agent
// that is deleted is gone, not reset.
func (s *Server) handleResetAgent(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	agentID := r.PathValue("agentID")
	builtin, ok := crew.Find(agentID)
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("only built-in agents can be reset"))
		return
	}
	_ = builtin

	// There may be nothing saved at all — that is reset already, and the
	// answer is the same merged list either way.
	if err := s.store.DeleteAgent(r.Context(), agentID, user.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.listAgents(w, r)
}

// handleGetAgent returns a single agent by ID.
func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	rec, err := s.store.GetAgent(r.Context(), r.PathValue("agentID"), user.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	builtin := crew.IsBuiltinID(rec.ID)
	writeJSON(w, http.StatusOK, map[string]any{"agent": agentRow{AgentRecord: rec, Builtin: builtin}})
}