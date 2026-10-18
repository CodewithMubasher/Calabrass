package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// UserRecord is one local account. PasswordHash is never sent to the client.
type UserRecord struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
	CreatedAt    int64  `json:"createdAt"`
}

// CreateUser inserts a new account. The caller checks for a duplicate email
// first so it can report that case clearly.
func (s *Store) CreateUser(ctx context.Context, rec UserRecord) (UserRecord, error) {
	if rec.CreatedAt == 0 {
		rec.CreatedAt = time.Now().UnixMilli()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, name, email, password_hash, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		rec.ID, rec.Name, rec.Email, rec.PasswordHash, rec.CreatedAt)
	if err != nil {
		return UserRecord{}, fmt.Errorf("store: create user: %w", err)
	}
	return rec, nil
}

// UserByEmail returns the account with this email, case-insensitively.
func (s *Store) UserByEmail(ctx context.Context, email string) (UserRecord, error) {
	return scanUser(s.db.QueryRowContext(ctx, `
		SELECT id, name, email, password_hash, created_at
		FROM users WHERE email = ? COLLATE NOCASE`, email))
}

// UserByID returns the account with this ID.
func (s *Store) UserByID(ctx context.Context, id string) (UserRecord, error) {
	return scanUser(s.db.QueryRowContext(ctx, `
		SELECT id, name, email, password_hash, created_at
		FROM users WHERE id = ?`, id))
}

func scanUser(row interface{ Scan(...any) error }) (UserRecord, error) {
	var rec UserRecord
	err := row.Scan(&rec.ID, &rec.Name, &rec.Email, &rec.PasswordHash, &rec.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return UserRecord{}, ErrNotFound
	}
	if err != nil {
		return UserRecord{}, fmt.Errorf("store: scan user: %w", err)
	}
	return rec, nil
}

// CreateSession records a session token hash for a user.
func (s *Store) CreateSession(ctx context.Context, tokenHash, userID string, expiresAt int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (token_hash, user_id, created_at, expires_at)
		VALUES (?, ?, ?, ?)`,
		tokenHash, userID, time.Now().UnixMilli(), expiresAt)
	if err != nil {
		return fmt.Errorf("store: create session: %w", err)
	}
	return nil
}

// UserBySession resolves a live session to its account. Expired rows are not
// returned; DeleteExpiredSessions clears them out.
func (s *Store) UserBySession(ctx context.Context, tokenHash string, now int64) (UserRecord, error) {
	return scanUser(s.db.QueryRowContext(ctx, `
		SELECT u.id, u.name, u.email, u.password_hash, u.created_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?`, tokenHash, now))
}

// DeleteSession removes one session. A missing row is not an error: signing out
// twice should still succeed.
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash); err != nil {
		return fmt.Errorf("store: delete session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions clears sessions that can no longer be used.
func (s *Store) DeleteExpiredSessions(ctx context.Context, now int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now); err != nil {
		return fmt.Errorf("store: delete expired sessions: %w", err)
	}
	return nil
}
