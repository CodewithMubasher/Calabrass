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

// The operating modes a conversation can be in. Chat is the ordinary one;
// crew is for long and complex work.
const (
	ModeChat = "chat"
	ModeCrew = "crew"
)

// Chat is a conversation without its messages.
type Chat struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	// Mode is the chat's operating mode: "chat" (the ordinary one) or "crew"
	// (the long-horizon one).
	Mode string `json:"mode"`
	// WorkspaceDir is the workspace folder this chat works in. Empty means the
	// default workspace.
	WorkspaceDir string `json:"workspaceDir"`
	// AgentID is the agent this chat is associated with. Empty means no agent.
	AgentID string `json:"agentId"`
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
	// Compacted indicates whether this message's trace/replay has been replaced
	// with a one-line summary to save context space.
	Compacted bool `json:"compacted,omitempty"`
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
	user_id    TEXT NOT NULL DEFAULT '',
	title      TEXT NOT NULL,
	provider   TEXT NOT NULL DEFAULT '',
	model      TEXT NOT NULL DEFAULT '',
	mode       TEXT NOT NULL DEFAULT 'chat',
	workspace_dir TEXT NOT NULL DEFAULT '',
	agent_id   TEXT NOT NULL DEFAULT '',
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

-- One row per provider the person added: its endpoint, its key and whether it
-- is on. A provider exists only because there is a row here, so a fresh database
-- offers none.
CREATE TABLE IF NOT EXISTS providers (
	user_id    TEXT NOT NULL DEFAULT '',
	id         TEXT NOT NULL,
	name       TEXT NOT NULL DEFAULT '',
	base_url   TEXT NOT NULL DEFAULT '',
	api_key    TEXT NOT NULL DEFAULT '',
	enabled    INTEGER NOT NULL DEFAULT 1,
	custom     INTEGER NOT NULL DEFAULT 0,
	updated_at INTEGER NOT NULL,
	PRIMARY KEY (user_id, id)
);

CREATE INDEX IF NOT EXISTS idx_messages_chat ON messages(chat_id, created_at);
CREATE INDEX IF NOT EXISTS idx_chats_updated ON chats(updated_at DESC);

-- One row per workspace: the folder the agent may work in, plus the label the
-- UI shows for it. The folder itself lives inside the workspaces library.
CREATE TABLE IF NOT EXISTS workspaces (
	id          TEXT PRIMARY KEY,
	user_id     TEXT NOT NULL DEFAULT '',
	name        TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	dir         TEXT NOT NULL,
	builtin     INTEGER NOT NULL DEFAULT 0,
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);

-- One row per MCP connector: the server the agent may start, and whether it is
-- allowed to.
CREATE TABLE IF NOT EXISTS connectors (
	id         TEXT PRIMARY KEY,
	user_id    TEXT NOT NULL DEFAULT '',
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
	user_id      TEXT NOT NULL DEFAULT '',
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

-- One row per sub-agent: a named instruction, the mode it runs in, the MCP
-- connector it may use, and the model it prefers with a fallback beside it.
-- system_prompt is the explicit system prompt replacement; when empty, prompt is
-- used. description is the one line that says what the agent is for, which is
-- what the orchestrator reads to decide whether to hand work to it.
--
-- The key is (user_id, id) rather than id alone: the agents the app ships with
-- have the same id for every account, so two accounts must be able to hold
-- their own saved version of the same built-in.
CREATE TABLE IF NOT EXISTS agents (
	id            TEXT NOT NULL,
	user_id       TEXT NOT NULL DEFAULT '',
	name          TEXT NOT NULL,
	prompt        TEXT NOT NULL DEFAULT '',
	system_prompt TEXT NOT NULL DEFAULT '',
	mode          TEXT NOT NULL DEFAULT 'chat',
	description   TEXT NOT NULL DEFAULT '',
	connector_id  TEXT NOT NULL DEFAULT '',
	provider      TEXT NOT NULL DEFAULT '',
	model         TEXT NOT NULL DEFAULT '',
	alt_provider  TEXT NOT NULL DEFAULT '',
	alt_model     TEXT NOT NULL DEFAULT '',
	skills        TEXT NOT NULL DEFAULT '[]',
	created_at    INTEGER NOT NULL,
	updated_at    INTEGER NOT NULL,
	PRIMARY KEY (user_id, id)
);

-- One row per local account. Passwords are stored as bcrypt hashes; the desktop
-- app is single-machine, so there is no email verification or reset flow.
CREATE TABLE IF NOT EXISTS users (
	id           TEXT PRIMARY KEY,
	name         TEXT NOT NULL,
	email        TEXT NOT NULL COLLATE NOCASE UNIQUE,
	password_hash TEXT NOT NULL,
	created_at   INTEGER NOT NULL
);

-- One row per signed-in browser. Only the SHA-256 of the token is stored, so a
-- copy of the database cannot be used to sign in as anyone.
CREATE TABLE IF NOT EXISTS sessions (
	token_hash TEXT PRIMARY KEY,
	user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions(expires_at);

-- One row per model call: where it went and how many tokens it reported. The
-- usage page reads this; nothing else does.
CREATE TABLE IF NOT EXISTS usage_events (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id       TEXT NOT NULL DEFAULT '',
	provider_id   TEXT NOT NULL,
	model_id      TEXT NOT NULL DEFAULT '',
	input_tokens  INTEGER NOT NULL DEFAULT 0,
	output_tokens INTEGER NOT NULL DEFAULT 0,
	created_at    INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_usage_created ON usage_events(created_at);
CREATE INDEX IF NOT EXISTS idx_usage_provider ON usage_events(provider_id, created_at);
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
		{"messages", "compacted", "compacted INTEGER NOT NULL DEFAULT 0"},
		{"providers", "name", "name TEXT NOT NULL DEFAULT ''"},
		{"providers", "base_url", "base_url TEXT NOT NULL DEFAULT ''"},
		{"providers", "api_key", "api_key TEXT NOT NULL DEFAULT ''"},
		{"providers", "custom", "custom INTEGER NOT NULL DEFAULT 0"},
		{"chats", "pinned", "pinned INTEGER NOT NULL DEFAULT 0"},
		{"chats", "workspace_dir", "workspace_dir TEXT NOT NULL DEFAULT ''"},
		{"chats", "agent_id", "agent_id TEXT NOT NULL DEFAULT ''"},
		{"chats", "mode", "mode TEXT NOT NULL DEFAULT 'chat'"},
		{"agents", "system_prompt", "system_prompt TEXT NOT NULL DEFAULT ''"},
		{"agents", "description", "description TEXT NOT NULL DEFAULT ''"},
		{"agents", "skills", "skills TEXT NOT NULL DEFAULT '[]'"},
	}
	for _, col := range added {
		if err := s.ensureColumn(ctx, col.table, col.column, col.ddl); err != nil {
			return err
		}
	}

	// The agents table used to key on the id alone. The agents the app ships
	// with have the same id for every account, so the key has to carry the
	// account with it. A table that already has user_id is already keyed that
	// way, and rebuilding it again would wipe ownership on every startup.
	if ok, err := s.hasColumn(ctx, "agents", "user_id"); err != nil {
		return err
	} else if !ok {
		if err := s.rebuildAgents(ctx); err != nil {
			return err
		}
	}

	// The two modes used to be "build" and "plan". Plan is gone, and build is
	// the ordinary chat, so both old names become "chat": a conversation that
	// was a plan carries on as an ordinary one.
	if err := s.renameModes(ctx); err != nil {
		return err
	}

	// Provider rows became per-account. An older providers table keys only on
	// id, so it cannot hold one account's Groq override beside another's; move
	// its rows into a table keyed by (user_id, id). The rows stay unowned until
	// the admin account exists, and are assigned to it at startup.
	if ok, err := s.hasColumn(ctx, "providers", "user_id"); err != nil {
		return err
	} else if !ok {
		if err := s.rebuildProviders(ctx); err != nil {
			return err
		}
	}

	// Providers used to be built into the backend, with their endpoint implied by
	// the id. They are ordinary rows now, so a row that build left behind is
	// given the name and endpoint it used to imply, and keeps the key it holds.
	if err := s.adoptBuiltinProviders(ctx); err != nil {
		return err
	}

	// Every user-facing record belongs to the account that made it. The empty
	// default is a migration placeholder: existing rows have no owner yet and
	// are assigned to the shipped admin account once it has been seeded.
	owned := []struct{ table, column, ddl string }{
		{"chats", "user_id", "user_id TEXT NOT NULL DEFAULT ''"},
		{"workspaces", "user_id", "user_id TEXT NOT NULL DEFAULT ''"},
		{"connectors", "user_id", "user_id TEXT NOT NULL DEFAULT ''"},
		{"scheduled_tasks", "user_id", "user_id TEXT NOT NULL DEFAULT ''"},
		{"agents", "user_id", "user_id TEXT NOT NULL DEFAULT ''"},
		{"usage_events", "user_id", "user_id TEXT NOT NULL DEFAULT ''"},
	}
	for _, col := range owned {
		if err := s.ensureColumn(ctx, col.table, col.column, col.ddl); err != nil {
			return err
		}
	}

	// The indexes use the columns above, so they are created only after the
	// migration has added them to older databases.
	const indexes = `
CREATE INDEX IF NOT EXISTS idx_chats_user_updated ON chats(user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_usage_user_created ON usage_events(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_usage_user_provider ON usage_events(user_id, provider_id, created_at);
CREATE INDEX IF NOT EXISTS idx_scheduled_user ON scheduled_tasks(user_id, run_at);
CREATE INDEX IF NOT EXISTS idx_connectors_user ON connectors(user_id, name);
CREATE INDEX IF NOT EXISTS idx_agents_user ON agents(user_id, name);
DROP INDEX IF EXISTS idx_workspaces_dir;
CREATE UNIQUE INDEX IF NOT EXISTS idx_workspaces_user_dir ON workspaces(user_id, dir);
`
	if _, err := s.db.ExecContext(ctx, indexes); err != nil {
		return fmt.Errorf("store: migrate indexes: %w", err)
	}
	return nil
}

// hasColumn reports whether a table already has a column.
func (s *Store) hasColumn(ctx context.Context, table, column string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("store: inspect %s.%s: %w", table, column, err)
	}
	return count > 0, nil
}

// rebuildAgents rebuilds the agents table onto the (user_id, id) key without
// losing any saved agents. A row that was left by the build that keyed on the
// id alone becomes an unowned row (user_id = '') and is assigned to the admin
// account once it has been seeded.
func (s *Store) rebuildAgents(ctx context.Context) error {
	const rebuild = `
DROP TABLE IF EXISTS agents_user_keyed;
CREATE TABLE agents_user_keyed (
	user_id       TEXT NOT NULL DEFAULT '',
	id            TEXT NOT NULL,
	name          TEXT NOT NULL,
	prompt        TEXT NOT NULL DEFAULT '',
	system_prompt TEXT NOT NULL DEFAULT '',
	mode          TEXT NOT NULL DEFAULT 'chat',
	description   TEXT NOT NULL DEFAULT '',
	connector_id  TEXT NOT NULL DEFAULT '',
	provider      TEXT NOT NULL DEFAULT '',
	model         TEXT NOT NULL DEFAULT '',
	alt_provider  TEXT NOT NULL DEFAULT '',
	alt_model     TEXT NOT NULL DEFAULT '',
	skills        TEXT NOT NULL DEFAULT '[]',
	created_at    INTEGER NOT NULL,
	updated_at    INTEGER NOT NULL,
	PRIMARY KEY (user_id, id)
);
INSERT INTO agents_user_keyed (user_id, id, name, prompt, system_prompt, mode, description, connector_id, provider, model, alt_provider, alt_model, skills, created_at, updated_at)
SELECT '', id, name, prompt, system_prompt, mode, description, connector_id, provider, model, alt_provider, alt_model, '[]', created_at, updated_at FROM agents;
DROP TABLE agents;
ALTER TABLE agents_user_keyed RENAME TO agents;
`
	if _, err := s.db.ExecContext(ctx, rebuild); err != nil {
		return fmt.Errorf("store: rebuild agents: %w", err)
	}
	return nil
}

// rebuildProviders moves an older single-key providers table onto the
// per-account (user_id, id) key without losing any configured provider.
// The modes used to be "build" and "plan". Plan is gone — a conversation that
// was a plan carries on as an ordinary one — and build is the ordinary chat, so
// both names land on "chat". It is idempotent: a row that already says "chat"
// is left alone.
func (s *Store) renameModes(ctx context.Context) error {
	const rename = `
UPDATE chats SET mode = 'chat' WHERE mode = 'build' OR mode = 'plan';
UPDATE agents SET mode = 'chat' WHERE mode = 'build' OR mode = 'plan';
`
	if _, err := s.db.ExecContext(ctx, rename); err != nil {
		return fmt.Errorf("store: rename modes: %w", err)
	}
	return nil
}

// rebuildProviders moves an older single-key providers table onto the
// per-account (user_id, id) key without losing any configured provider.
func (s *Store) rebuildProviders(ctx context.Context) error {
	const rebuild = `
DROP TABLE IF EXISTS providers_user_keyed;
CREATE TABLE providers_user_keyed (
	user_id    TEXT NOT NULL DEFAULT '',
	id         TEXT NOT NULL,
	name       TEXT NOT NULL DEFAULT '',
	base_url   TEXT NOT NULL DEFAULT '',
	api_key    TEXT NOT NULL DEFAULT '',
	enabled    INTEGER NOT NULL DEFAULT 1,
	custom     INTEGER NOT NULL DEFAULT 0,
	updated_at INTEGER NOT NULL,
	PRIMARY KEY (user_id, id)
);
INSERT INTO providers_user_keyed (user_id, id, name, base_url, api_key, enabled, custom, updated_at)
SELECT '', id, name, base_url, api_key, enabled, custom, updated_at FROM providers;
DROP TABLE providers;
ALTER TABLE providers_user_keyed RENAME TO providers;
`
	if _, err := s.db.ExecContext(ctx, rebuild); err != nil {
		return fmt.Errorf("store: rebuild providers: %w", err)
	}
	return nil
}

// adoptBuiltinProviders turns a row left by the build that had built-in
// providers into an ordinary provider. Such a row carries no name or endpoint
// because the backend supplied both; here it is given what it used to imply.
//
// The URLs are written out rather than taken from the AI package on purpose: a
// migration has to keep meaning what it meant when it was written, whatever the
// code that talks to those vendors does later. It is idempotent — once a row is
// custom it no longer matches.
func (s *Store) adoptBuiltinProviders(ctx context.Context) error {
	const adopt = `
UPDATE providers SET
	custom = 1,
	name = CASE
		WHEN name = '' AND id = 'google' THEN 'Google Gemini'
		WHEN name = '' AND id = 'groq'   THEN 'Groq'
		ELSE name
	END,
	base_url = CASE
		WHEN base_url = '' AND id = 'google' THEN 'https://generativelanguage.googleapis.com'
		WHEN base_url = '' AND id = 'groq'   THEN 'https://api.groq.com/openai/v1'
		ELSE base_url
	END
WHERE custom = 0`
	if _, err := s.db.ExecContext(ctx, adopt); err != nil {
		return fmt.Errorf("store: adopt built-in providers: %w", err)
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
func (s *Store) ListChats(ctx context.Context, userID ...string) ([]Chat, error) {
	owner := ownerOf(userID)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, provider, model, mode, workspace_dir, agent_id, pinned, created_at, updated_at
		FROM chats
		WHERE user_id = ?
		ORDER BY pinned DESC, updated_at DESC, rowid DESC`, owner)
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
		if err := rows.Scan(&c.ID, &c.Title, &c.Provider, &c.Model, &c.Mode, &c.WorkspaceDir, &c.AgentID, &pinned, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan chat: %w", err)
		}
		c.Pinned = pinned != 0
		chats = append(chats, c)
	}
	return chats, rows.Err()
}

// CreateChat inserts a new chat. A workspaceDir binds the chat to one of the
// library's folders; empty means the default workspace. The trailing account id
// is the owner; calls without one are for tests and migration only.
func (s *Store) CreateChat(ctx context.Context, providerID, model, workspaceDir, agentID, mode string, userID ...string) (Chat, error) {
	now := time.Now().UnixMilli()
	if mode == "" {
		mode = ModeChat
	}
	chat := Chat{
		ID:           NewID(),
		Title:        "New Chat",
		Provider:     providerID,
		Model:        model,
		Mode:         mode,
		WorkspaceDir: workspaceDir,
		AgentID:      agentID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO chats (id, user_id, title, provider, model, mode, workspace_dir, agent_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		chat.ID, ownerOf(userID), chat.Title, chat.Provider, chat.Model, chat.Mode, chat.WorkspaceDir, chat.AgentID, chat.CreatedAt, chat.UpdatedAt)
	if err != nil {
		return Chat{}, fmt.Errorf("store: create chat: %w", err)
	}
	return chat, nil
}

func (s *Store) getChat(ctx context.Context, id string, userID ...string) (Chat, error) {
	var (
		c      Chat
		pinned int
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id, title, provider, model, mode, workspace_dir, agent_id, pinned, created_at, updated_at
		FROM chats WHERE id = ? AND user_id = ?`, id, ownerOf(userID)).
		Scan(&c.ID, &c.Title, &c.Provider, &c.Model, &c.Mode, &c.WorkspaceDir, &c.AgentID, &pinned, &c.CreatedAt, &c.UpdatedAt)
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
func (s *Store) GetChat(ctx context.Context, id string, userID ...string) (ChatDetail, error) {
	chat, err := s.getChat(ctx, id, userID...)
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
		       output_tokens, duration_ms, trace, replay, compacted, created_at
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
			m       Message
			trace   string
			replay  string
			compacted int
		)
		if err := rows.Scan(
			&m.ID, &m.ChatID, &m.Role, &m.Content,
			&m.Reasoning, &m.ThinkingMs,
			&m.OutputTokens, &m.DurationMs, &trace, &replay, &compacted, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan message: %w", err)
		}
		m.Trace = rawJSON(trace)
		m.Replay = rawJSON(replay)
		m.Compacted = compacted != 0
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// AddMessage appends a message and bumps the chat's updated_at. The ID and
// creation time are assigned here. The trailing account id checks that the chat
// belongs to the caller before anything is written.
func (s *Store) AddMessage(ctx context.Context, msg Message, userID ...string) (Message, error) {
	msg.ID = NewID()
	msg.CreatedAt = time.Now().UnixMilli()
	now := msg.CreatedAt
	owner := ownerOf(userID)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, fmt.Errorf("store: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM chats WHERE id = ? AND user_id = ?`, msg.ChatID, owner,
	).Scan(&exists); err != nil {
		return Message{}, fmt.Errorf("store: check chat: %w", err)
	}
	if exists == 0 {
		return Message{}, ErrNotFound
	}

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
		`UPDATE chats SET updated_at = ? WHERE id = ? AND user_id = ?`, now, msg.ChatID, owner); err != nil {
		return Message{}, fmt.Errorf("store: touch chat: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Message{}, fmt.Errorf("store: commit: %w", err)
	}
	return msg, nil
}

// CompactMessages replaces the trace/replay of old messages with a one-line
// summary, to reduce context size. It compacts messages that:
// - are not already compacted
// - have a trace with tool calls
// - are older than the most recent keepCount messages
// Returns the number of messages compacted.
func (s *Store) CompactMessages(ctx context.Context, chatID string, keepCount int) (int, error) {
	// First, find the cutoff created_at: the created_at of the keepCount-th most recent message
	var cutoff int64
	err := s.db.QueryRowContext(ctx, `
		SELECT created_at FROM messages
		WHERE chat_id = ? AND compacted = 0
		ORDER BY created_at DESC
		LIMIT 1 OFFSET ?`, chatID, keepCount).Scan(&cutoff)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil // fewer than keepCount messages
		}
		return 0, fmt.Errorf("store: find cutoff: %w", err)
	}

	// Find messages to compact: not compacted, have trace with steps, older than cutoff
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, trace FROM messages
		WHERE chat_id = ? AND compacted = 0 AND created_at < ? AND trace != '' AND trace != '[]'
		ORDER BY created_at ASC`, chatID, cutoff)
	if err != nil {
		return 0, fmt.Errorf("store: find messages to compact: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type msgToCompact struct {
		id    string
		trace string
	}
	var toCompact []msgToCompact
	for rows.Next() {
		var id, trace string
		if err := rows.Scan(&id, &trace); err != nil {
			return 0, fmt.Errorf("store: scan message to compact: %w", err)
		}
		toCompact = append(toCompact, msgToCompact{id: id, trace: trace})
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("store: iterate messages: %w", err)
	}

	if len(toCompact) == 0 {
		return 0, nil
	}

	compacted := 0
	for _, m := range toCompact {
		summary := summarizeTrace(m.trace)
		if summary == "" {
			continue
		}
		_, err := s.db.ExecContext(ctx, `
			UPDATE messages SET trace = ?, replay = ?, compacted = 1 WHERE id = ?`,
			`["summary": "` + summary + `"]`, `["summary": "` + summary + `"]`, m.id)
		if err != nil {
			return compacted, fmt.Errorf("store: compact message %s: %w", m.id, err)
		}
		compacted++
	}
	return compacted, nil
}

type msgToCompact struct {
	id    string
	trace string
}

// summarizeTrace extracts a one-line summary from a trace JSON.
func summarizeTrace(traceJSON string) string {
	var trace struct {
		Steps []struct {
			Parts []struct {
				Kind   string `json:"kind"`
				Name   string `json:"name"`
				Output string `json:"output"`
			} `json:"parts"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(traceJSON), &trace); err != nil {
		return ""
	}
	var parts []string
	for _, step := range trace.Steps {
		for _, part := range step.Parts {
			if part.Kind == "tool" && part.Name != "" {
				parts = append(parts, part.Name)
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts) <= 3 {
		return "Tools: " + strings.Join(parts, ", ")
	}
	return fmt.Sprintf("Tools: %s, and %d more", strings.Join(parts[:3], ", "), len(parts)-3)
}

// DeleteMessage removes one message from a chat the account owns.
func (s *Store) DeleteMessage(ctx context.Context, chatID, messageID string, userID ...string) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM messages
		WHERE id = ? AND chat_id = ?
		  AND EXISTS (SELECT 1 FROM chats WHERE chats.id = messages.chat_id AND chats.user_id = ?)`,
		messageID, chatID, ownerOf(userID))
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
func (s *Store) UpdateChat(ctx context.Context, id, title, providerID, model string, userID ...string) (Chat, error) {
	if _, err := s.getChat(ctx, id, userID...); err != nil {
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
		return s.getChat(ctx, id, userID...)
	}

	args = append(args, id, ownerOf(userID))
	query := "UPDATE chats SET " + join(set, ", ") + " WHERE id = ? AND user_id = ?"
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return Chat{}, fmt.Errorf("store: update chat: %w", err)
	}
	return s.getChat(ctx, id, userID...)
}

// RenameChat sets a chat's title.
func (s *Store) RenameChat(ctx context.Context, id, title string, userID ...string) (Chat, error) {
	return s.UpdateChat(ctx, id, title, "", "", userID...)
}

// SetChatPinned pins or unpins a chat. Pinned chats are listed above the rest.
func (s *Store) SetChatPinned(ctx context.Context, id string, pinned bool, userID ...string) (Chat, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE chats SET pinned = ? WHERE id = ? AND user_id = ?`, boolToInt(pinned), id, ownerOf(userID))
	if err != nil {
		return Chat{}, fmt.Errorf("store: pin chat: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return Chat{}, ErrNotFound
	}
	return s.getChat(ctx, id, userID...)
}

// SetChatMode updates the chat's mode (chat/crew).
func (s *Store) SetChatMode(ctx context.Context, id string, mode string, userID ...string) (Chat, error) {
	if mode != ModeChat && mode != ModeCrew {
		mode = ModeChat
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE chats SET mode = ? WHERE id = ? AND user_id = ?`, mode, id, ownerOf(userID))
	if err != nil {
		return Chat{}, fmt.Errorf("store: set chat mode: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return Chat{}, ErrNotFound
	}
	return s.getChat(ctx, id, userID...)
}

// DeleteChat removes a chat and its messages.
func (s *Store) DeleteChat(ctx context.Context, id string, userID ...string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM chats WHERE id = ? AND user_id = ?`, id, ownerOf(userID))
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

// ListProviders returns every stored provider row for one account.
func (s *Store) ListProviders(ctx context.Context, userID ...string) ([]ProviderRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, base_url, api_key, enabled, custom, updated_at
		FROM providers
		WHERE user_id = ?
		ORDER BY custom ASC, updated_at ASC`, ownerOf(userID))
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

// GetProvider returns one stored provider row for an account.
func (s *Store) GetProvider(ctx context.Context, id string, userID ...string) (ProviderRecord, error) {
	var (
		rec             ProviderRecord
		enabled, custom int
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, base_url, api_key, enabled, custom, updated_at
		FROM providers WHERE id = ? AND user_id = ?`, id, ownerOf(userID)).
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

// UpsertProvider writes a provider row in full for an account.
func (s *Store) UpsertProvider(ctx context.Context, rec ProviderRecord, userID ...string) error {
	owner := ownerOf(userID)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO providers (user_id, id, name, base_url, api_key, enabled, custom, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, id) DO UPDATE SET
			name = excluded.name,
			base_url = excluded.base_url,
			api_key = excluded.api_key,
			enabled = excluded.enabled,
			custom = excluded.custom,
			updated_at = excluded.updated_at`,
		owner, rec.ID, rec.Name, rec.BaseURL, rec.APIKey,
		boolToInt(rec.Enabled), boolToInt(rec.Custom), time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("store: save provider %s: %w", rec.ID, err)
	}
	return nil
}

// DeleteProvider removes a stored provider row for an account.
func (s *Store) DeleteProvider(ctx context.Context, id string, userID ...string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM providers WHERE id = ? AND user_id = ?`, id, ownerOf(userID))
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

// ListWorkspaces returns every workspace for an account, the built-in one
// first.
func (s *Store) ListWorkspaces(ctx context.Context, userID ...string) ([]WorkspaceRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+workspaceColumns+`
		FROM workspaces
		WHERE user_id = ?
		ORDER BY builtin DESC, created_at ASC, rowid ASC`, ownerOf(userID))
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

// GetWorkspace returns one workspace by id for an account.
func (s *Store) GetWorkspace(ctx context.Context, id string, userID ...string) (WorkspaceRecord, error) {
	rec, err := scanWorkspace(s.db.QueryRowContext(ctx,
		`SELECT `+workspaceColumns+` FROM workspaces WHERE id = ? AND user_id = ?`, id, ownerOf(userID)))
	if errors.Is(err, sql.ErrNoRows) {
		return WorkspaceRecord{}, ErrNotFound
	}
	if err != nil {
		return WorkspaceRecord{}, fmt.Errorf("store: get workspace: %w", err)
	}
	return rec, nil
}

// CreateWorkspace stores a new workspace for an account. The ID and timestamps
// are assigned here; the folder itself is the caller's job.
func (s *Store) CreateWorkspace(ctx context.Context, rec WorkspaceRecord, userID ...string) (WorkspaceRecord, error) {
	now := time.Now().UnixMilli()
	rec.ID = NewID()
	rec.CreatedAt, rec.UpdatedAt = now, now

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO workspaces (id, user_id, name, description, dir, builtin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, ownerOf(userID), rec.Name, rec.Description, rec.Dir, boolToInt(rec.Builtin), rec.CreatedAt, rec.UpdatedAt,
	); err != nil {
		return WorkspaceRecord{}, fmt.Errorf("store: create workspace: %w", err)
	}
	return rec, nil
}

// EnsureWorkspace returns the workspace that uses a folder for an account,
// creating its row when there is none. It is how the default workspace is
// seeded on boot and on first use.
func (s *Store) EnsureWorkspace(ctx context.Context, rec WorkspaceRecord, userID ...string) (WorkspaceRecord, error) {
	owner := ownerOf(userID)
	existing, err := scanWorkspace(s.db.QueryRowContext(ctx,
		`SELECT `+workspaceColumns+` FROM workspaces WHERE user_id = ? AND dir = ?`, owner, rec.Dir))
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return WorkspaceRecord{}, fmt.Errorf("store: find workspace %s: %w", rec.Dir, err)
	}
	return s.CreateWorkspace(ctx, rec, owner)
}

// DeleteWorkspace removes a workspace row owned by an account. The built-in one
// is refused. The folder is never touched: a workspace's files are the user's,
// not ours.
func (s *Store) DeleteWorkspace(ctx context.Context, id string, userID ...string) error {
	rec, err := s.GetWorkspace(ctx, id, userID...)
	if err != nil {
		return err
	}
	if rec.Builtin {
		return ErrProtected
	}

	res, err := s.db.ExecContext(ctx,
		`DELETE FROM workspaces WHERE id = ? AND user_id = ?`, id, ownerOf(userID))
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

// ListConnectors returns every connector for an account, alphabetically.
func (s *Store) ListConnectors(ctx context.Context, userID ...string) ([]ConnectorRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+connectorColumns+`
		FROM connectors
		WHERE user_id = ?
		ORDER BY name COLLATE NOCASE ASC, rowid ASC`, ownerOf(userID))
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

// CreateConnector stores a new connector for an account, ready to be turned on.
func (s *Store) CreateConnector(ctx context.Context, name, command string, userID ...string) (ConnectorRecord, error) {
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
		INSERT INTO connectors (id, user_id, name, command, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, ownerOf(userID), rec.Name, rec.Command, boolToInt(rec.Enabled), rec.CreatedAt, rec.UpdatedAt,
	); err != nil {
		return ConnectorRecord{}, fmt.Errorf("store: create connector: %w", err)
	}
	return rec, nil
}

// SetConnectorEnabled turns a connector on or off.
func (s *Store) SetConnectorEnabled(ctx context.Context, id string, enabled bool, userID ...string) (ConnectorRecord, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE connectors SET enabled = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		boolToInt(enabled), time.Now().UnixMilli(), id, ownerOf(userID))
	if err != nil {
		return ConnectorRecord{}, fmt.Errorf("store: update connector: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return ConnectorRecord{}, ErrNotFound
	}
	return s.getConnector(ctx, id, userID...)
}

func (s *Store) getConnector(ctx context.Context, id string, userID ...string) (ConnectorRecord, error) {
	rec, err := scanConnector(s.db.QueryRowContext(ctx,
		`SELECT `+connectorColumns+` FROM connectors WHERE id = ? AND user_id = ?`, id, ownerOf(userID)))
	if errors.Is(err, sql.ErrNoRows) {
		return ConnectorRecord{}, ErrNotFound
	}
	if err != nil {
		return ConnectorRecord{}, fmt.Errorf("store: get connector: %w", err)
	}
	return rec, nil
}

// DeleteConnector removes a connector owned by an account.
func (s *Store) DeleteConnector(ctx context.Context, id string, userID ...string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM connectors WHERE id = ? AND user_id = ?`, id, ownerOf(userID))
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

// Agent records --------------------------------------------------------------

// AgentRecord is one sub-agent: a named instruction the user can hand work to,
// together with the mode, connector and models it should run with.
//
// The model fields name a provider as well as a model, because a bare model id
// is ambiguous across providers. The alternative pair is a fallback the agent
// may be given when its default is not available.
//
// When used as a primary chat agent, the Prompt field REPLACES the global
// system prompt entirely, and the agent's connector/model are used. Its mode is
// the default a chat of its own starts in; the address the conversation lives
// at is what says which mode it is in afterwards.
type AgentRecord struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Prompt       string `json:"prompt"`       // System prompt replacement when used as primary agent
	SystemPrompt string `json:"systemPrompt"` // Explicit system prompt (optional, falls back to Prompt)
	// Mode is "chat" or "crew".
	Mode string `json:"mode"`
	// Description is what the agent is for, in one line. The orchestrator reads it.
	Description string `json:"description"`
	// ConnectorID is the MCP connector the agent may use; empty for none.
	ConnectorID string `json:"connectorId"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	AltProvider string `json:"altProvider"`
	AltModel    string `json:"altModel"`
	// Skills are the folders under ~/.calabrass/skills this worker must read
	// before it starts. Only workers use them; an interactive agent ignores them.
	Skills    []string `json:"skills"`
	CreatedAt int64    `json:"createdAt"`
	UpdatedAt int64    `json:"updatedAt"`
}

const agentColumns = `id, name, prompt, system_prompt, mode, description, connector_id, provider, model, alt_provider, alt_model, skills, created_at, updated_at`

func scanAgent(row interface{ Scan(...any) error }) (AgentRecord, error) {
	var rec AgentRecord
	var skills string
	if err := row.Scan(
		&rec.ID, &rec.Name, &rec.Prompt, &rec.SystemPrompt, &rec.Mode, &rec.Description, &rec.ConnectorID,
		&rec.Provider, &rec.Model, &rec.AltProvider, &rec.AltModel, &skills,
		&rec.CreatedAt, &rec.UpdatedAt,
	); err != nil {
		return AgentRecord{}, err
	}
	rec.Skills = decodeSkills(skills)
	return rec, nil
}

// decodeSkills reads the JSON array a row stores. A malformed or empty value is
// no skills rather than an error: an agent with a broken column must still open.
func decodeSkills(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{}
	}
	var skills []string
	if err := json.Unmarshal([]byte(text), &skills); err != nil {
		return []string{}
	}
	if skills == nil {
		return []string{}
	}
	return skills
}

// encodeSkills writes skills as the JSON array the column holds.
func encodeSkills(skills []string) string {
	clean := make([]string, 0, len(skills))
	for _, skill := range skills {
		if trimmed := strings.TrimSpace(skill); trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	data, err := json.Marshal(clean)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// ListAgents returns every agent for an account, alphabetically by name.
func (s *Store) ListAgents(ctx context.Context, userID ...string) ([]AgentRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+agentColumns+`
		FROM agents
		WHERE user_id = ?
		ORDER BY name COLLATE NOCASE ASC, rowid ASC`, ownerOf(userID))
	if err != nil {
		return nil, fmt.Errorf("store: list agents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	records := []AgentRecord{}
	for rows.Next() {
		rec, err := scanAgent(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan agent: %w", err)
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// GetAgent returns one agent by id for an account.
func (s *Store) GetAgent(ctx context.Context, id string, userID ...string) (AgentRecord, error) {
	rec, err := scanAgent(s.db.QueryRowContext(ctx,
		`SELECT `+agentColumns+` FROM agents WHERE id = ? AND user_id = ?`, id, ownerOf(userID)))
	if errors.Is(err, sql.ErrNoRows) {
		return AgentRecord{}, ErrNotFound
	}
	if err != nil {
		return AgentRecord{}, fmt.Errorf("store: get agent: %w", err)
	}
	return rec, nil
}

// CreateAgent stores a new agent for an account. The ID and timestamps are
// assigned here.
func (s *Store) CreateAgent(ctx context.Context, rec AgentRecord, userID ...string) (AgentRecord, error) {
	now := time.Now().UnixMilli()
	rec.ID = NewID()
	rec.CreatedAt, rec.UpdatedAt = now, now

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO agents (
			id, user_id, name, prompt, system_prompt, mode, description, connector_id, provider, model,
			alt_provider, alt_model, skills, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, ownerOf(userID), rec.Name, rec.Prompt, rec.SystemPrompt, rec.Mode, rec.Description, rec.ConnectorID,
		rec.Provider, rec.Model, rec.AltProvider, rec.AltModel, encodeSkills(rec.Skills),
		rec.CreatedAt, rec.UpdatedAt,
	); err != nil {
		return AgentRecord{}, fmt.Errorf("store: create agent: %w", err)
	}
	return rec, nil
}

// UpdateAgent writes an agent's fields in full, keeping its id and creation time.
func (s *Store) UpdateAgent(ctx context.Context, id string, rec AgentRecord, userID ...string) (AgentRecord, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE agents SET
			name = ?, prompt = ?, system_prompt = ?, mode = ?, description = ?, connector_id = ?,
			provider = ?, model = ?, alt_provider = ?, alt_model = ?, skills = ?,
			updated_at = ?
		WHERE id = ? AND user_id = ?`,
		rec.Name, rec.Prompt, rec.SystemPrompt, rec.Mode, rec.Description, rec.ConnectorID,
		rec.Provider, rec.Model, rec.AltProvider, rec.AltModel, encodeSkills(rec.Skills),
		time.Now().UnixMilli(), id, ownerOf(userID))
	if err != nil {
		return AgentRecord{}, fmt.Errorf("store: update agent: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return AgentRecord{}, ErrNotFound
	}
	return s.GetAgent(ctx, id, userID...)
}

// DeleteAgent removes an agent owned by an account.
func (s *Store) DeleteAgent(ctx context.Context, id string, userID ...string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM agents WHERE id = ? AND user_id = ?`, id, ownerOf(userID))
	if err != nil {
		return fmt.Errorf("store: delete agent: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete agent: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpsertAgent writes an agent with a specific id, whether or not it was
// already in the database. This is how edits to built-ins work: the built-in's
// fields are used until the account saves an override, and this writes that
// override without creating a second agent.
func (s *Store) UpsertAgent(ctx context.Context, id string, rec AgentRecord, userID ...string) (AgentRecord, error) {
	now := time.Now().UnixMilli()
	rec.ID = id
	rec.UpdatedAt = now

	// If it doesn't exist yet, set created_at too.
	_, err := s.GetAgent(ctx, id, userID...)
	if errors.Is(err, ErrNotFound) {
		rec.CreatedAt = now
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO agents (
				id, user_id, name, prompt, system_prompt, mode, description, connector_id, provider, model,
				alt_provider, alt_model, skills, created_at, updated_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, ownerOf(userID), rec.Name, rec.Prompt, rec.SystemPrompt, rec.Mode, rec.Description, rec.ConnectorID,
			rec.Provider, rec.Model, rec.AltProvider, rec.AltModel, encodeSkills(rec.Skills),
			now, now,
		); err != nil {
			return AgentRecord{}, fmt.Errorf("store: upsert agent insert: %w", err)
		}
		return rec, nil
	}
	if err != nil {
		return AgentRecord{}, fmt.Errorf("store: upsert agent check: %w", err)
	}

	// It exists — update it.
	if _, err := s.db.ExecContext(ctx, `
		UPDATE agents SET
			name = ?, prompt = ?, system_prompt = ?, mode = ?, description = ?, connector_id = ?,
			provider = ?, model = ?, alt_provider = ?, alt_model = ?, skills = ?,
			updated_at = ?
		WHERE id = ? AND user_id = ?`,
		rec.Name, rec.Prompt, rec.SystemPrompt, rec.Mode, rec.Description, rec.ConnectorID,
		rec.Provider, rec.Model, rec.AltProvider, rec.AltModel, encodeSkills(rec.Skills),
		now, id, ownerOf(userID)); err != nil {
		return AgentRecord{}, fmt.Errorf("store: upsert agent update: %w", err)
	}
	return rec, nil
}

// ownerOf reads the optional account id at the end of a store method. It is
func ownerOf(userID []string) string {
	if len(userID) == 0 {
		return ""
	}
	return strings.TrimSpace(userID[0])
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
