package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// LedgerEntry is one line of the run ledger.
type LedgerEntry struct {
	ID   string `json:"id"`
	At   int64  `json:"at"`
	Kind string `json:"kind"` // "action" or "run"
	// ChatID is the conversation the action belonged to.
	ChatID  string `json:"chatId"`
	Tool    string `json:"tool"`
	Tier    int    `json:"tier"`
	Summary string `json:"summary"`
	// Outcome is how it ended: allowed, remembered, approved, refused,
	// unattended-refused, timed-out; or for a run: completed, failed, missed.
	Outcome    string `json:"outcome"`
	Decision   string `json:"decision,omitempty"`
	Unattended bool   `json:"unattended"`
	UserID     string `json:"-"`
}

// LedgerKeep is how many rows a person keeps. The ledger is a record for
// answering "what happened while I was away", not an archive; older rows are
// dropped as new ones arrive.
const LedgerKeep = 5000

// ledgerKeep is LedgerKeep, held in a variable so a test can use a small one.
var ledgerKeep = LedgerKeep

// AddLedgerEntry appends a row. userID and the entry's UserID must agree.
func (s *Store) AddLedgerEntry(ctx context.Context, entry LedgerEntry) error {
	if entry.UserID == "" {
		return fmt.Errorf("store: a ledger entry needs a user")
	}
	if entry.ID == "" {
		entry.ID = NewID()
	}
	if entry.At == 0 {
		entry.At = time.Now().UnixMilli()
	}
	if entry.Kind == "" {
		entry.Kind = "action"
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO ledger (id, user_id, at, kind, chat_id, tool, tier, summary, outcome, decision, unattended)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.UserID, entry.At, entry.Kind, entry.ChatID, entry.Tool, entry.Tier,
		entry.Summary, entry.Outcome, entry.Decision, boolToInt(entry.Unattended)); err != nil {
		return fmt.Errorf("store: add ledger entry: %w", err)
	}
	// Keep the newest LedgerKeep rows for this person.
	if _, err := s.db.ExecContext(ctx, `
		DELETE FROM ledger WHERE user_id = ? AND id IN (
			SELECT id FROM ledger WHERE user_id = ? ORDER BY at DESC, id DESC LIMIT -1 OFFSET ?)`,
		entry.UserID, entry.UserID, ledgerKeep); err != nil {
		return fmt.Errorf("store: trim the ledger: %w", err)
	}
	return nil
}

// LedgerQuery narrows a listing.
type LedgerQuery struct {
	Limit      int    // default 100, at most 500
	Before     int64  // only rows older than this time (ms); 0 means from the newest
	ChatID     string // only this conversation
	Unattended bool   // only what ran with nobody watching
	Refused    bool   // only what was refused or timed out
}

// ListLedger returns a person's rows, newest first.
func (s *Store) ListLedger(ctx context.Context, userID string, q LedgerQuery) ([]LedgerEntry, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	where := []string{"user_id = ?"}
	args := []any{userID}
	if q.Before > 0 {
		where = append(where, "at < ?")
		args = append(args, q.Before)
	}
	if q.ChatID != "" {
		where = append(where, "chat_id = ?")
		args = append(args, q.ChatID)
	}
	if q.Unattended {
		where = append(where, "unattended = 1")
	}
	if q.Refused {
		where = append(where, "outcome IN ('refused', 'unattended-refused', 'timed-out', 'failed', 'missed')")
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, at, kind, chat_id, tool, tier, summary, outcome, decision, unattended
		FROM ledger WHERE `+strings.Join(where, " AND ")+` ORDER BY at DESC, id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list ledger: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []LedgerEntry{}
	for rows.Next() {
		var e LedgerEntry
		var unattended int
		if err := rows.Scan(&e.ID, &e.At, &e.Kind, &e.ChatID, &e.Tool, &e.Tier,
			&e.Summary, &e.Outcome, &e.Decision, &unattended); err != nil {
			return nil, fmt.Errorf("store: list ledger: %w", err)
		}
		e.Unattended = unattended != 0
		out = append(out, e)
	}
	return out, rows.Err()
}
