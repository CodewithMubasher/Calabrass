package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/permission"
)

// This file is where the permission tiers meet the HTTP API. The rules live in
// the permission package; here a turn is given the session it runs under, and
// an approval the agent raised is put in front of whoever can answer it.

// interactiveSession is the session for a turn a person is watching. Approvals
// it raises are announced on the turn's own stream and are also listed at
// /api/approvals, so any face can answer them.
func (s *Server) interactiveSession(userID, chatID, workspaceRoot string, stream *sseWriter) *permission.Session {
	sess := &permission.Session{
		Mode:      permission.Interactive,
		UserID:    userID,
		ChatID:    chatID,
		Workspace: workspaceRoot,
		Broker:    s.broker,
	}
	if stream != nil {
		sess.Notify = func(event string, req permission.Request) {
			// The stream refuses writes once its handler has returned, so a tool
			// that outlives the request cannot write to a finished response.
			_ = stream.event(event, req)
		}
	}
	return sess
}

// unattendedSession is the session for a turn nobody is watching. It runs
// inside the scope a person wrote down for the task, and nothing else.
func (s *Server) unattendedSession(userID, chatID, workspaceRoot string, scope permission.Scope) *permission.Session {
	return &permission.Session{
		Mode:      permission.Unattended,
		UserID:    userID,
		ChatID:    chatID,
		Workspace: workspaceRoot,
		Scope:     scope.Normalized(),
		Broker:    s.broker,
	}
}

// sessionFor returns the session a turn is built under. A handler that knows
// who is asking puts one on the context; a turn built without one is treated as
// unattended with the default scope, which is the cautious reading: it can read
// and edit files, and it cannot run anything.
func (s *Server) sessionFor(ctx context.Context, userID, chatID, workspaceRoot string) *permission.Session {
	base := permission.SessionFrom(ctx)
	if base == nil {
		return s.unattendedSession(userID, chatID, workspaceRoot, permission.DefaultScope())
	}
	sess := *base
	if sess.UserID == "" {
		sess.UserID = userID
	}
	if sess.ChatID == "" {
		sess.ChatID = chatID
	}
	if sess.Workspace == "" {
		sess.Workspace = workspaceRoot
	}
	if sess.Broker == nil {
		sess.Broker = s.broker
	}
	return &sess
}

// --- endpoints --------------------------------------------------------------

func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"approvals": s.pendingApprovals(user.ID)})
}

func (s *Server) pendingApprovals(userID string) []permission.Request {
	if s.broker == nil {
		return []permission.Request{}
	}
	return s.broker.Pending(userID)
}

// handleDecideApproval records a person's answer to one approval.
func (s *Server) handleDecideApproval(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	var body struct {
		Decision permission.Decision `json:"decision"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if s.broker == nil {
		writeError(w, http.StatusNotFound, permission.ErrNotFound)
		return
	}
	switch err := s.broker.Decide(user.ID, r.PathValue("approvalID"), body.Decision); {
	case errors.Is(err, permission.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, permission.ErrBadDecision):
		writeError(w, http.StatusBadRequest, err)
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"approvals": s.pendingApprovals(user.ID)})
	}
}

// handleForgetApprovals drops every "allow this kind of command" answer the
// person gave. It is the "reset my permissions" button.
func (s *Server) handleForgetApprovals(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	if s.broker != nil {
		s.broker.Forget(user.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleApprovalStream pushes the waiting approvals when the connection opens
// and whenever they change, so a face that is not the one streaming the turn
// (another window, the terminal, a phone) still sees a request and can answer.
func (s *Server) handleApprovalStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := currentUser(ctx)

	stream, err := newSSEWriter(w)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer stream.close()

	send := func() bool {
		return stream.event("approvals", map[string]any{"approvals": s.pendingApprovals(user.ID)}) == nil
	}
	if !send() {
		return
	}
	if s.broker == nil {
		<-ctx.Done()
		return
	}

	changes, stop := s.broker.Watch()
	defer stop()
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-changes:
			if !send() {
				return
			}
		case <-heartbeat.C:
			if stream.comment("keep-alive") != nil {
				return
			}
		}
	}
}
