package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/permission"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// writeLedger records one line of the run ledger. It never fails the work it
// is recording: a ledger that cannot be written is logged and the agent carries
// on, because refusing to act would punish the person for a storage problem.
func (s *Server) writeLedger(entry store.LedgerEntry) {
	if s.store == nil || entry.UserID == "" {
		return
	}
	// A command line can carry a credential; the record keeps the shape and not
	// the secret.
	entry.Summary = permission.Redact(entry.Summary)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.store.AddLedgerEntry(ctx, entry); err != nil {
		s.log.Warn("could not write the run ledger", "error", err)
	}
}

// recordPermission is what the broker calls for every decision at the Execute
// tier or above: it goes in the log and in the ledger.
func (s *Server) recordPermission(e permission.Entry) {
	summary := permission.Redact(e.Summary)
	s.log.Info("permission",
		"user", e.UserID, "chat", e.ChatID, "tool", e.Tool,
		"tier", int(e.Tier), "outcome", e.Outcome,
		"decision", string(e.Decision), "unattended", e.Unattended,
		"action", summary)

	s.writeLedger(store.LedgerEntry{
		UserID:     e.UserID,
		At:         e.Time.UnixMilli(),
		Kind:       "action",
		ChatID:     e.ChatID,
		Tool:       e.Tool,
		Tier:       int(e.Tier),
		Summary:    summary,
		Outcome:    e.Outcome,
		Decision:   string(e.Decision),
		Unattended: e.Unattended,
	})
}

// handleListLedger answers "what did the agent do while I was away?".
//
//	GET /api/ledger?limit=100&before=<ms>&chat=<id>&unattended=1&refused=1
func (s *Server) handleListLedger(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	q := r.URL.Query()

	query := store.LedgerQuery{
		ChatID:     q.Get("chat"),
		Unattended: q.Get("unattended") == "1",
		Refused:    q.Get("refused") == "1",
	}
	if v, err := strconv.Atoi(q.Get("limit")); err == nil {
		query.Limit = v
	}
	if v, err := strconv.ParseInt(q.Get("before"), 10, 64); err == nil {
		query.Before = v
	}

	entries, err := s.store.ListLedger(r.Context(), user.ID, query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}
