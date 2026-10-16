package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a chat does not exist.
var ErrNotFound = errors.New("store: not found")

// Chat is a conversation without its messages.
type Chat struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// Message is a single turn in a conversation. Reasoning holds the model's
// thinking text when the provider exposes it, and ThinkingMs is how long it
// spent producing it.
type Message struct {
	ID         string `json:"id"`
	ChatID     string `json:"chatId"`
	Role       string `json:"role"`
	Content    string `json:"content"`
	Reasoning  string `json:"reasoning,omitempty"`
	ThinkingMs int64  `json:"thinkingMs,omitempty"`
	CreatedAt  int64  `json:"createdAt"`
}

// ChatDetail is a chat together with its messages.
type ChatDetail struct {
	Chat
	Messages []Message `json:"messages"`
}

// Store is a SQLite-backed persistence layer.
type Store struct {
	db *sql.DB
}

// Open opens (and migrates) the database at path.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("store: create data dir: %w", err)
		}
	}

	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)",
		filepath.ToSlash(path),
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	// SQLite handles a single writer best; keeping one connection also avoids
	// SQLITE_BUSY between the WAL readers and the writer.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS chats (
	id         TEXT PRIMARY KEY,
	title      TEXT NOT NULL,
	provider   TEXT NOT NULL DEFAULT '',
	model      TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
	id         TEXT PRIMARY KEY,
	chat_id    TEXT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
	role       TEXT NOT NULL,
	content    TEXT NOT NULL,
	reasoning  TEXT NOT NULL DEFAULT '',
	thinking_ms INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_messages_chat ON messages(chat_id, created_at);
CREATE INDEX IF NOT EXISTS idx_chats_updated ON chats(updated_at DESC);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}

	// Columns added after the first release, for databases created earlier.
	added := []struct{ table, column, ddl string }{
		{"messages", "reasoning", "reasoning TEXT NOT NULL DEFAULT ''"},
		{"messages", "thinking_ms", "thinking_ms INTEGER NOT NULL DEFAULT 0"},
	}
	for _, col := range added {
		if err := s.ensureColumn(ctx, col.table, col.column, col.ddl); err != nil {
			return err
		}
	}
	return nil
}

// ensureColumn adds a column when an older database does not have it yet.
func (s *Store) ensureColumn(ctx context.Context, table, column, ddl string) error {
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column,
	).Scan(&count); err != nil {
		return fmt.Errorf("store: inspect %s.%s: %w", table, column, err)
	}
	if count > 0 {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", table, ddl)); err != nil {
		return fmt.Errorf("store: add %s.%s: %w", table, column, err)
	}
	return nil
}

// NewID returns a 14-digit numeric identifier, matching the format the
// frontend uses in chat URLs.
func NewID() string {
	const digits = 14
	var b [digits]byte
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			// crypto/rand failing is not recoverable in any meaningful way here;
			// fall back to a time-derived digit rather than aborting a request.
			b[i] = byte('0' + (time.Now().UnixNano()/int64(i+1))%10)
			continue
		}
		b[i] = byte('0' + n.Int64())
	}
	return string(b[:])
}

// ListChats returns chats ordered by most recently updated.
func (s *Store) ListChats(ctx context.Context) ([]Chat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, provider, model, created_at, updated_at
		FROM chats
		ORDER BY updated_at DESC, rowid DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: list chats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	chats := []Chat{}
	for rows.Next() {
		var c Chat
		if err := rows.Scan(&c.ID, &c.Title, &c.Provider, &c.Model, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan chat: %w", err)
		}
		chats = append(chats, c)
	}
	return chats, rows.Err()
}

// CreateChat inserts a new chat.
func (s *Store) CreateChat(ctx context.Context, providerID, model string) (Chat, error) {
	now := time.Now().UnixMilli()
	chat := Chat{
		ID:        NewID(),
		Title:     "New Chat",
		Provider:  providerID,
		Model:     model,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO chats (id, title, provider, model, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		chat.ID, chat.Title, chat.Provider, chat.Model, chat.CreatedAt, chat.UpdatedAt)
	if err != nil {
		return Chat{}, fmt.Errorf("store: create chat: %w", err)
	}
	return chat, nil
}

func (s *Store) getChat(ctx context.Context, id string) (Chat, error) {
	var c Chat
	err := s.db.QueryRowContext(ctx, `
		SELECT id, title, provider, model, created_at, updated_at
		FROM chats WHERE id = ?`, id).
		Scan(&c.ID, &c.Title, &c.Provider, &c.Model, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Chat{}, ErrNotFound
	}
	if err != nil {
		return Chat{}, fmt.Errorf("store: get chat: %w", err)
	}
	return c, nil
}

// GetChat returns a chat with all of its messages.
func (s *Store) GetChat(ctx context.Context, id string) (ChatDetail, error) {
	chat, err := s.getChat(ctx, id)
	if err != nil {
		return ChatDetail{}, err
	}
	messages, err := s.ListMessages(ctx, id)
	if err != nil {
		return ChatDetail{}, err
	}
	return ChatDetail{Chat: chat, Messages: messages}, nil
}

// ListMessages returns a chat's messages in chronological order.
func (s *Store) ListMessages(ctx context.Context, chatID string) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, chat_id, role, content, reasoning, thinking_ms, created_at
		FROM messages
		WHERE chat_id = ?
		ORDER BY created_at ASC, rowid ASC`, chatID)
	if err != nil {
		return nil, fmt.Errorf("store: list messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	messages := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(
			&m.ID, &m.ChatID, &m.Role, &m.Content,
			&m.Reasoning, &m.ThinkingMs, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan message: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// AddMessage appends a message and bumps the chat's updated_at. The ID and
// creation time are assigned here.
func (s *Store) AddMessage(ctx context.Context, msg Message) (Message, error) {
	msg.ID = NewID()
	msg.CreatedAt = time.Now().UnixMilli()
	now := msg.CreatedAt

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, fmt.Errorf("store: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO messages (id, chat_id, role, content, reasoning, thinking_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.ChatID, msg.Role, msg.Content, msg.Reasoning, msg.ThinkingMs, msg.CreatedAt,
	); err != nil {
		return Message{}, fmt.Errorf("store: insert message: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE chats SET updated_at = ? WHERE id = ?`, now, msg.ChatID); err != nil {
		return Message{}, fmt.Errorf("store: touch chat: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Message{}, fmt.Errorf("store: commit: %w", err)
	}
	return msg, nil
}

// UpdateChat changes the title and/or the selected model of a chat. Empty
// values are left untouched.
func (s *Store) UpdateChat(ctx context.Context, id, title, providerID, model string) (Chat, error) {
	if _, err := s.getChat(ctx, id); err != nil {
		return Chat{}, err
	}

	set := []string{}
	args := []any{}
	if title != "" {
		set = append(set, "title = ?")
		args = append(args, title)
	}
	if model != "" {
		set = append(set, "model = ?", "provider = ?")
		args = append(args, model, providerID)
	}
	if len(set) == 0 {
		return s.getChat(ctx, id)
	}

	args = append(args, id)
	query := "UPDATE chats SET " + join(set, ", ") + " WHERE id = ?"
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return Chat{}, fmt.Errorf("store: update chat: %w", err)
	}
	return s.getChat(ctx, id)
}

// RenameChat sets a chat's title.
func (s *Store) RenameChat(ctx context.Context, id, title string) (Chat, error) {
	return s.UpdateChat(ctx, id, title, "", "")
}

// DeleteChat removes a chat and its messages.
func (s *Store) DeleteChat(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM chats WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete chat: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete chat: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func join(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}
