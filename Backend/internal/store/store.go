package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a chat does not exist.
var ErrNotFound = errors.New("store: not found")

// ErrProtected is returned when something built in — the default workspace —
// is asked to be removed.
var ErrProtected = errors.New("store: this item is built in and cannot be removed")

// Chat is a conversation without its messages.
type Chat struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	// WorkspaceDir is the workspace folder this chat works in. Empty means the
	// default workspace.
	WorkspaceDir string `json:"workspaceDir"`
	// Pinned chats are listed above the rest, in the order they were last used.
	Pinned    bool  `json:"pinned"`
	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

// Message is a single turn in a conversation. Reasoning holds the model's
// thinking text when the provider exposes it, and ThinkingMs is how long it
// spent producing it.
type Message struct {
	ID        string `json:"id"`
	ChatID    string `json:"chatId"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Reasoning string `json:"reasoning,omitempty"`
	// ThinkingMs is how long the model spent thinking, when the provider
	// reports reasoning.
	ThinkingMs int64 `json:"thinkingMs,omitempty"`
	// OutputTokens and DurationMs describe the finished reply, and give the
	// UI its tokens-per-second figure.
	OutputTokens int64 `json:"outputTokens,omitempty"`
	DurationMs   int64 `json:"durationMs,omitempty"`
	// Trace is the ordered record of the turn — reasoning, narration and tool
	// calls — as JSON, and is what the UI renders inline.
	Trace json.RawMessage `json:"trace,omitempty"`
	// Replay is the provider transcript for the turn, kept so a later turn can
	// replay its tool calls. It is an implementation detail and never sent to
	// the frontend.
	Replay    json.RawMessage `json:"-"`
	CreatedAt int64           `json:"createdAt"`
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
	workspace_dir TEXT NOT NULL DEFAULT '',
	pinned     INTEGER NOT NULL DEFAULT 0,
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
	output_tokens INTEGER NOT NULL DEFAULT 0,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	trace      TEXT NOT NULL DEFAULT '',
	replay     TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);

-- One row per provider that has been configured or toggled. A provider with no
-- row uses its built-in defaults (credentials from .env, on). Custom providers
-- always have a row and carry their own endpoint and key.
CREATE TABLE IF NOT EXISTS providers (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL DEFAULT '',
	base_url   TEXT NOT NULL DEFAULT '',
	api_key    TEXT NOT NULL DEFAULT '',
	enabled    INTEGER NOT NULL DEFAULT 1,
	custom     INTEGER NOT NULL DEFAULT 0,
	updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_messages_chat ON messages(chat_id, created_at);
CREATE INDEX IF NOT EXISTS idx_chats_updated ON chats(updated_at DESC);

-- One row per workspace: the folder the agent may work in, plus the label the
-- UI shows for it. The folder itself lives inside the workspaces library.
CREATE TABLE IF NOT EXISTS workspaces (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	dir         TEXT NOT NULL,
	builtin     INTEGER NOT NULL DEFAULT 0,
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_workspaces_dir ON workspaces(dir);

-- One row per MCP connector: the server the agent may start, and whether it is
-- allowed to.
CREATE TABLE IF NOT EXISTS connectors (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	command    TEXT NOT NULL,
	enabled    INTEGER NOT NULL DEFAULT 1,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);

-- One row per scheduled task: an instruction, the conversation it speaks in, and
-- the time it is due. A timer rather than a cron — last_run_at is what stops a
-- task from coming due twice.
CREATE TABLE IF NOT EXISTS scheduled_tasks (
	id           TEXT PRIMARY KEY,
	prompt       TEXT NOT NULL,
	chat_id      TEXT NOT NULL DEFAULT '',
	run_at       INTEGER NOT NULL,
	enabled      INTEGER NOT NULL DEFAULT 1,
	last_run_at  INTEGER NOT NULL DEFAULT 0,
	last_status  TEXT NOT NULL DEFAULT '',
	last_error   TEXT NOT NULL DEFAULT '',
	created_at   INTEGER NOT NULL,
	updated_at   INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_scheduled_due ON scheduled_tasks(enabled, last_run_at, run_at);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}

	// Columns added after the first release, for databases created earlier.
	added := []struct{ table, column, ddl string }{
		{"messages", "reasoning", "reasoning TEXT NOT NULL DEFAULT ''"},
		{"messages", "thinking_ms", "thinking_ms INTEGER NOT NULL DEFAULT 0"},
		{"messages", "output_tokens", "output_tokens INTEGER NOT NULL DEFAULT 0"},
		{"messages", "duration_ms", "duration_ms INTEGER NOT NULL DEFAULT 0"},
		{"messages", "trace", "trace TEXT NOT NULL DEFAULT ''"},
		{"messages", "replay", "replay TEXT NOT NULL DEFAULT ''"},
		{"providers", "name", "name TEXT NOT NULL DEFAULT ''"},
		{"providers", "base_url", "base_url TEXT NOT NULL DEFAULT ''"},
		{"providers", "api_key", "api_key TEXT NOT NULL DEFAULT ''"},
		{"providers", "custom", "custom INTEGER NOT NULL DEFAULT 0"},
		{"chats", "pinned", "pinned INTEGER NOT NULL DEFAULT 0"},
		{"chats", "workspace_dir", "workspace_dir TEXT NOT NULL DEFAULT ''"},
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

// jsonText stores a JSON blob as text, keeping NULL and empty apart from an
// empty document.
func jsonText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	return string(raw)
}

// rawJSON turns a stored column back into JSON, treating an empty column as
// "nothing stored" rather than as an invalid document.
func rawJSON(text string) json.RawMessage {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return json.RawMessage(text)
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

// ListChats returns chats ordered by pinned first, then most recently updated.
func (s *Store) ListChats(ctx context.Context) ([]Chat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, provider, model, workspace_dir, pinned, created_at, updated_at
		FROM chats
		ORDER BY pinned DESC, updated_at DESC, rowid DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: list chats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	chats := []Chat{}
	for rows.Next() {
		var (
			c      Chat
			pinned int
		)
		if err := rows.Scan(&c.ID, &c.Title, &c.Provider, &c.Model, &c.WorkspaceDir, &pinned, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan chat: %w", err)
		}
		c.Pinned = pinned != 0
		chats = append(chats, c)
	}
	return chats, rows.Err()
}

// CreateChat inserts a new chat. A workspaceDir binds the chat to one of the
// library's folders; empty means the default workspace.
func (s *Store) CreateChat(ctx context.Context, providerID, model, workspaceDir string) (Chat, error) {
	now := time.Now().UnixMilli()
	chat := Chat{
		ID:           NewID(),
		Title:        "New Chat",
		Provider:     providerID,
		Model:        model,
		WorkspaceDir: workspaceDir,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO chats (id, title, provider, model, workspace_dir, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		chat.ID, chat.Title, chat.Provider, chat.Model, chat.WorkspaceDir, chat.CreatedAt, chat.UpdatedAt)
	if err != nil {
		return Chat{}, fmt.Errorf("store: create chat: %w", err)
	}
	return chat, nil
}

func (s *Store) getChat(ctx context.Context, id string) (Chat, error) {
	var (
		c      Chat
		pinned int
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id, title, provider, model, workspace_dir, pinned, created_at, updated_at
		FROM chats WHERE id = ?`, id).
		Scan(&c.ID, &c.Title, &c.Provider, &c.Model, &c.WorkspaceDir, &pinned, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Chat{}, ErrNotFound
	}
	if err != nil {
		return Chat{}, fmt.Errorf("store: get chat: %w", err)
	}
	c.Pinned = pinned != 0
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
		SELECT id, chat_id, role, content, reasoning, thinking_ms,
		       output_tokens, duration_ms, trace, replay, created_at
		FROM messages
		WHERE chat_id = ?
		ORDER BY created_at ASC, rowid ASC`, chatID)
	if err != nil {
		return nil, fmt.Errorf("store: list messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	messages := []Message{}
	for rows.Next() {
		var (
			m      Message
			trace  string
			replay string
		)
		if err := rows.Scan(
			&m.ID, &m.ChatID, &m.Role, &m.Content,
			&m.Reasoning, &m.ThinkingMs,
			&m.OutputTokens, &m.DurationMs, &trace, &replay, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan message: %w", err)
		}
		m.Trace = rawJSON(trace)
		m.Replay = rawJSON(replay)
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
		INSERT INTO messages (
			id, chat_id, role, content, reasoning, thinking_ms,
			output_tokens, duration_ms, trace, replay, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.ChatID, msg.Role, msg.Content, msg.Reasoning, msg.ThinkingMs,
		msg.OutputTokens, msg.DurationMs, jsonText(msg.Trace), jsonText(msg.Replay), msg.CreatedAt,
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

// DeleteMessage removes one message from a chat.
func (s *Store) DeleteMessage(ctx context.Context, chatID, messageID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM messages WHERE id = ? AND chat_id = ?`, messageID, chatID)
	if err != nil {
		return fmt.Errorf("store: delete message: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete message: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
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

// SetChatPinned pins or unpins a chat. Pinned chats are listed above the rest.
func (s *Store) SetChatPinned(ctx context.Context, id string, pinned bool) (Chat, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE chats SET pinned = ? WHERE id = ?`, boolToInt(pinned), id)
	if err != nil {
		return Chat{}, fmt.Errorf("store: pin chat: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return Chat{}, ErrNotFound
	}
	return s.getChat(ctx, id)
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

// Provider records -----------------------------------------------------------

// ProviderRecord is a stored provider: its on/off state and, for a provider
// the user added, its own endpoint and credential.
type ProviderRecord struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BaseURL   string `json:"baseUrl"`
	APIKey    string `json:"apiKey"`
	Enabled   bool   `json:"enabled"`
	Custom    bool   `json:"custom"`
	UpdatedAt int64  `json:"updatedAt"`
}

// ListProviders returns every stored provider row.
func (s *Store) ListProviders(ctx context.Context) ([]ProviderRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, base_url, api_key, enabled, custom, updated_at
		FROM providers
		ORDER BY custom ASC, updated_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: list providers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	records := []ProviderRecord{}
	for rows.Next() {
		var (
			rec             ProviderRecord
			enabled, custom int
		)
		if err := rows.Scan(
			&rec.ID, &rec.Name, &rec.BaseURL, &rec.APIKey,
			&enabled, &custom, &rec.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan provider: %w", err)
		}
		rec.Enabled = enabled != 0
		rec.Custom = custom != 0
		records = append(records, rec)
	}
	return records, rows.Err()
}

// GetProvider returns one stored provider row.
func (s *Store) GetProvider(ctx context.Context, id string) (ProviderRecord, error) {
	var (
		rec             ProviderRecord
		enabled, custom int
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, base_url, api_key, enabled, custom, updated_at
		FROM providers WHERE id = ?`, id).
		Scan(&rec.ID, &rec.Name, &rec.BaseURL, &rec.APIKey, &enabled, &custom, &rec.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ProviderRecord{}, ErrNotFound
	}
	if err != nil {
		return ProviderRecord{}, fmt.Errorf("store: get provider: %w", err)
	}
	rec.Enabled = enabled != 0
	rec.Custom = custom != 0
	return rec, nil
}

// UpsertProvider writes a provider row in full.
func (s *Store) UpsertProvider(ctx context.Context, rec ProviderRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO providers (id, name, base_url, api_key, enabled, custom, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			base_url = excluded.base_url,
			api_key = excluded.api_key,
			enabled = excluded.enabled,
			custom = excluded.custom,
			updated_at = excluded.updated_at`,
		rec.ID, rec.Name, rec.BaseURL, rec.APIKey,
		boolToInt(rec.Enabled), boolToInt(rec.Custom), time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("store: save provider %s: %w", rec.ID, err)
	}
	return nil
}

// DeleteProvider removes a stored provider row.
func (s *Store) DeleteProvider(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM providers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete provider: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete provider: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// Workspace records ----------------------------------------------------------

// WorkspaceRecord is a workspace: the folder the agent works in, and the label
// and description the UI shows for it.
type WorkspaceRecord struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Dir is the folder name inside the workspaces library.
	Dir string `json:"dir"`
	// Builtin marks the default workspace, which cannot be deleted.
	Builtin   bool  `json:"builtin"`
	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

const workspaceColumns = `id, name, description, dir, builtin, created_at, updated_at`

func scanWorkspace(row interface{ Scan(...any) error }) (WorkspaceRecord, error) {
	var (
		rec     WorkspaceRecord
		builtin int
	)
	if err := row.Scan(&rec.ID, &rec.Name, &rec.Description, &rec.Dir, &builtin, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return WorkspaceRecord{}, err
	}
	rec.Builtin = builtin != 0
	return rec, nil
}

// ListWorkspaces returns every workspace, the built-in one first.
func (s *Store) ListWorkspaces(ctx context.Context) ([]WorkspaceRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+workspaceColumns+`
		FROM workspaces
		ORDER BY builtin DESC, created_at ASC, rowid ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: list workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()

	records := []WorkspaceRecord{}
	for rows.Next() {
		rec, err := scanWorkspace(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan workspace: %w", err)
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// GetWorkspace returns one workspace by id.
func (s *Store) GetWorkspace(ctx context.Context, id string) (WorkspaceRecord, error) {
	rec, err := scanWorkspace(s.db.QueryRowContext(ctx,
		`SELECT `+workspaceColumns+` FROM workspaces WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return WorkspaceRecord{}, ErrNotFound
	}
	if err != nil {
		return WorkspaceRecord{}, fmt.Errorf("store: get workspace: %w", err)
	}
	return rec, nil
}

// CreateWorkspace stores a new workspace. The ID and timestamps are assigned
// here; the folder itself is the caller's job.
func (s *Store) CreateWorkspace(ctx context.Context, rec WorkspaceRecord) (WorkspaceRecord, error) {
	now := time.Now().UnixMilli()
	rec.ID = NewID()
	rec.CreatedAt, rec.UpdatedAt = now, now

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO workspaces (id, name, description, dir, builtin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.Name, rec.Description, rec.Dir, boolToInt(rec.Builtin), rec.CreatedAt, rec.UpdatedAt,
	); err != nil {
		return WorkspaceRecord{}, fmt.Errorf("store: create workspace: %w", err)
	}
	return rec, nil
}

// EnsureWorkspace returns the workspace that uses a folder, creating its row
// when there is none. It is how the default workspace is seeded on boot.
func (s *Store) EnsureWorkspace(ctx context.Context, rec WorkspaceRecord) (WorkspaceRecord, error) {
	existing, err := scanWorkspace(s.db.QueryRowContext(ctx,
		`SELECT `+workspaceColumns+` FROM workspaces WHERE dir = ?`, rec.Dir))
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return WorkspaceRecord{}, fmt.Errorf("store: find workspace %s: %w", rec.Dir, err)
	}
	return s.CreateWorkspace(ctx, rec)
}

// DeleteWorkspace removes a workspace row. The built-in one is refused. The
// folder is never touched: a workspace's files are the user's, not ours.
func (s *Store) DeleteWorkspace(ctx context.Context, id string) error {
	rec, err := s.GetWorkspace(ctx, id)
	if err != nil {
		return err
	}
	if rec.Builtin {
		return ErrProtected
	}

	res, err := s.db.ExecContext(ctx, `DELETE FROM workspaces WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete workspace: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete workspace: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// Connector records ----------------------------------------------------------

// ConnectorRecord is one MCP server the agent may start: the name it is shown
// under, and the command that runs it.
type ConnectorRecord struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
	// Enabled is whether the agent may start the connector and use its tools.
	Enabled   bool  `json:"enabled"`
	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

const connectorColumns = `id, name, command, enabled, created_at, updated_at`

func scanConnector(row interface{ Scan(...any) error }) (ConnectorRecord, error) {
	var (
		rec     ConnectorRecord
		enabled int
	)
	if err := row.Scan(&rec.ID, &rec.Name, &rec.Command, &enabled, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return ConnectorRecord{}, err
	}
	rec.Enabled = enabled != 0
	return rec, nil
}

// ListConnectors returns every connector, alphabetically.
func (s *Store) ListConnectors(ctx context.Context) ([]ConnectorRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+connectorColumns+`
		FROM connectors
		ORDER BY name COLLATE NOCASE ASC, rowid ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: list connectors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	records := []ConnectorRecord{}
	for rows.Next() {
		rec, err := scanConnector(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan connector: %w", err)
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// CreateConnector stores a new connector, ready to be turned on.
func (s *Store) CreateConnector(ctx context.Context, name, command string) (ConnectorRecord, error) {
	now := time.Now().UnixMilli()
	rec := ConnectorRecord{
		ID:        NewID(),
		Name:      name,
		Command:   command,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO connectors (id, name, command, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.Name, rec.Command, boolToInt(rec.Enabled), rec.CreatedAt, rec.UpdatedAt,
	); err != nil {
		return ConnectorRecord{}, fmt.Errorf("store: create connector: %w", err)
	}
	return rec, nil
}

// SetConnectorEnabled turns a connector on or off.
func (s *Store) SetConnectorEnabled(ctx context.Context, id string, enabled bool) (ConnectorRecord, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE connectors SET enabled = ?, updated_at = ? WHERE id = ?`,
		boolToInt(enabled), time.Now().UnixMilli(), id)
	if err != nil {
		return ConnectorRecord{}, fmt.Errorf("store: update connector: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return ConnectorRecord{}, ErrNotFound
	}
	return s.getConnector(ctx, id)
}

func (s *Store) getConnector(ctx context.Context, id string) (ConnectorRecord, error) {
	rec, err := scanConnector(s.db.QueryRowContext(ctx,
		`SELECT `+connectorColumns+` FROM connectors WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return ConnectorRecord{}, ErrNotFound
	}
	if err != nil {
		return ConnectorRecord{}, fmt.Errorf("store: get connector: %w", err)
	}
	return rec, nil
}

// DeleteConnector removes a connector.
func (s *Store) DeleteConnector(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM connectors WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete connector: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete connector: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
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
