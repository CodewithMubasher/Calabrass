package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Device is another machine a person has paired with this host.
type Device struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	CreatedAt  int64  `json:"createdAt"`
	LastSeenAt int64  `json:"lastSeenAt"`
	UserID     string `json:"-"`
}

// MaxActivePairingCodes is how many unused codes one person may have at a time.
// Asking for a new one beyond it retires the oldest, so a code that was never
// used does not sit around.
const MaxActivePairingCodes = 3

// CreatePairingCode stores the hash of a new one-time code.
func (s *Store) CreatePairingCode(ctx context.Context, codeHash, userID string, expiresAt int64) error {
	now := time.Now().UnixMilli()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: pairing code: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Used and expired codes have nothing left to do.
	if _, err := tx.ExecContext(ctx, `DELETE FROM pairing_codes WHERE used_at > 0 OR expires_at <= ?`, now); err != nil {
		return fmt.Errorf("store: pairing code: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM pairing_codes WHERE user_id = ? AND code_hash IN (
			SELECT code_hash FROM pairing_codes WHERE user_id = ? ORDER BY created_at DESC LIMIT -1 OFFSET ?)`,
		userID, userID, MaxActivePairingCodes-1); err != nil {
		return fmt.Errorf("store: pairing code: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO pairing_codes (code_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		codeHash, userID, now, expiresAt); err != nil {
		return fmt.Errorf("store: pairing code: %w", err)
	}
	return tx.Commit()
}

// ConsumePairingCode uses a code up and returns whose it was. It is atomic: of
// two requests that present the same code, exactly one succeeds. A code that is
// unknown, used or expired is ErrNotFound, and the three are not told apart.
func (s *Store) ConsumePairingCode(ctx context.Context, codeHash string, now int64) (string, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE pairing_codes SET used_at = ? WHERE code_hash = ? AND used_at = 0 AND expires_at > ?`,
		now, codeHash, now)
	if err != nil {
		return "", fmt.Errorf("store: use pairing code: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return "", ErrNotFound
	}
	var userID string
	if err := s.db.QueryRowContext(ctx, `SELECT user_id FROM pairing_codes WHERE code_hash = ?`, codeHash).Scan(&userID); err != nil {
		return "", fmt.Errorf("store: use pairing code: %w", err)
	}
	return userID, nil
}

// CreateDevice records a paired device.
func (s *Store) CreateDevice(ctx context.Context, d Device) error {
	if d.CreatedAt == 0 {
		d.CreatedAt = time.Now().UnixMilli()
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO devices (id, user_id, name, created_at, last_seen_at) VALUES (?, ?, ?, ?, ?)`,
		d.ID, d.UserID, d.Name, d.CreatedAt, d.CreatedAt); err != nil {
		return fmt.Errorf("store: create device: %w", err)
	}
	return nil
}

// CreateDeviceSession records a session that belongs to a device.
func (s *Store) CreateDeviceSession(ctx context.Context, tokenHash, userID, deviceID string, expiresAt int64) error {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (token_hash, user_id, created_at, expires_at, device_id) VALUES (?, ?, ?, ?, ?)`,
		tokenHash, userID, time.Now().UnixMilli(), expiresAt, deviceID); err != nil {
		return fmt.Errorf("store: create device session: %w", err)
	}
	return nil
}

// ListDevices returns a person's paired devices, newest first.
func (s *Store) ListDevices(ctx context.Context, userID string) ([]Device, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, created_at, last_seen_at FROM devices WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list devices: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []Device{}
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.Name, &d.CreatedAt, &d.LastSeenAt); err != nil {
			return nil, fmt.Errorf("store: list devices: %w", err)
		}
		d.UserID = userID
		out = append(out, d)
	}
	return out, rows.Err()
}

// RevokeDevice removes a device and every session it has, so it is signed out
// at once. It reports whether there was such a device for that person.
func (s *Store) RevokeDevice(ctx context.Context, userID, deviceID string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("store: revoke device: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `DELETE FROM devices WHERE id = ? AND user_id = ?`, deviceID, userID)
	if err != nil {
		return false, fmt.Errorf("store: revoke device: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE device_id = ? AND user_id = ?`, deviceID, userID); err != nil {
		return false, fmt.Errorf("store: revoke device: %w", err)
	}
	return true, tx.Commit()
}

// DeviceForSession returns the device a live session belongs to. A session that
// belongs to no device (a password sign-in on this machine) is ErrNotFound.
func (s *Store) DeviceForSession(ctx context.Context, tokenHash string, now int64) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `
		SELECT device_id FROM sessions WHERE token_hash = ? AND expires_at > ? AND device_id <> ''`,
		tokenHash, now).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: device session: %w", err)
	}
	return id, nil
}

// TouchDevice records that a device was just seen.
func (s *Store) TouchDevice(ctx context.Context, deviceID string, now int64) {
	_, _ = s.db.ExecContext(ctx, `UPDATE devices SET last_seen_at = ? WHERE id = ?`, now, deviceID)
}
