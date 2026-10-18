package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// workspacePayload is a workspace as the UI sees it: the stored row, plus where
// the folder is and whether it is still there.
type workspacePayload struct {
	store.WorkspaceRecord
	// Path is the folder itself, so the user can find it on disk.
	Path string `json:"path"`
	// Missing reports a folder that was removed from disk by hand. The next
	// turn recreates it, but it is worth showing.
	Missing bool `json:"missing"`
}

func (s *Server) workspacePayloads(rows []store.WorkspaceRecord, userID string) []workspacePayload {
	lib := s.libraryFor(userID)
	out := make([]workspacePayload, 0, len(rows))
	for _, row := range rows {
		payload := workspacePayload{WorkspaceRecord: row, Missing: true}
		if lib != nil {
			if path, err := lib.Path(row.Dir); err == nil {
				payload.Path = path
				payload.Missing = !lib.Exists(row.Dir)
			}
		}
		out = append(out, payload)
	}
	return out
}

// ensureDefaultWorkspace makes sure an account has the folder and row every
// chat starts in. It is idempotent and runs the first time the account opens a
// workspace page or starts a turn.
func (s *Server) ensureDefaultWorkspace(ctx context.Context, userID string) (*agent.Workspace, error) {
	lib := s.libraryFor(userID)
	if lib == nil {
		return nil, errors.New("the workspaces folder is unavailable; check the server log")
	}
	workspace, err := lib.Ensure(agent.DefaultWorkspaceName)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.EnsureWorkspace(ctx, store.WorkspaceRecord{
		Name:        "Default",
		Description: "Where chats work until they pick a workspace of their own.",
		Dir:         agent.DefaultWorkspaceName,
		Builtin:     true,
	}, userID); err != nil {
		return nil, err
	}
	return workspace, nil
}

// workspacesUnavailable reports a server that booted without a usable
// workspaces folder, which leaves every endpoint that touches one unusable.
func (s *Server) workspacesUnavailable(w http.ResponseWriter) bool {
	if s.library != nil {
		return false
	}
	writeError(w, http.StatusServiceUnavailable,
		errors.New("the workspaces folder is unavailable; check the server log"))
	return true
}

// listWorkspaces answers with the full list, in the same shape the providers
// endpoints use, so one call refreshes the page after any change.
func (s *Server) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	if _, err := s.ensureDefaultWorkspace(r.Context(), user.ID); err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	rows, err := s.store.ListWorkspaces(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": s.workspacePayloads(rows, user.ID)})
}

func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	if s.workspacesUnavailable(w) {
		return
	}
	s.listWorkspaces(w, r)
}

// handleCreateWorkspace makes a workspace and the folder it works in. The folder
// name comes from the workspace's name, and never lands on one already there.
func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.workspacesUnavailable(w) {
		return
	}
	user := currentUser(r.Context())
	lib := s.libraryFor(user.ID)
	if lib == nil {
		writeError(w, http.StatusServiceUnavailable,
			errors.New("the workspaces folder is unavailable; check the server log"))
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, errors.New("a workspace name is required"))
		return
	}

	dir := lib.Unique(name)
	if _, err := lib.Ensure(dir); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if _, err := s.store.CreateWorkspace(r.Context(), store.WorkspaceRecord{
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Dir:         dir,
	}, user.ID); err != nil {
		// The empty folder left behind is harmless; the row is the record.
		s.log.Error("could not store workspace", "name", name, "dir", dir, "error", err)
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.listWorkspaces(w, r)
}

// handleDeleteWorkspace forgets a workspace. Its files stay on disk: they are
// the user's, and deleting them is not this endpoint's business.
func (s *Server) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.workspacesUnavailable(w) {
		return
	}

	err := s.store.DeleteWorkspace(r.Context(), r.PathValue("workspaceID"), currentUser(r.Context()).ID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
		return
	case errors.Is(err, store.ErrProtected):
		writeError(w, http.StatusBadRequest, err)
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.listWorkspaces(w, r)
}
