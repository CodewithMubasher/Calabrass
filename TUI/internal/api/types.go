// Package api is a small HTTP + SSE client for the Calabrass backend.
//
// It mirrors the JSON the backend already speaks (see Frontend/src/types/chat.ts
// and Backend/internal/httpapi). It has no terminal-UI imports, so it can be
// tested on its own.
package api

import "encoding/json"

// User is the signed-in account.
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Session is what sign-up and sign-in return.
type Session struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// Chat is one conversation, without its messages.
type Chat struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Mode         string `json:"mode"`
	WorkspaceDir string `json:"workspaceDir"`
	AgentID      string `json:"agentId"`
	Pinned       bool   `json:"pinned"`
	CreatedAt    int64  `json:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt"`
}

// ChatDetail is a chat together with its messages.
type ChatDetail struct {
	Chat
	Messages []Message `json:"messages"`
}

// Message is one turn of a conversation.
type Message struct {
	ID           string          `json:"id"`
	ChatID       string          `json:"chatId"`
	Role         string          `json:"role"`
	Content      string          `json:"content"`
	Reasoning    string          `json:"reasoning,omitempty"`
	ThinkingMs   int64           `json:"thinkingMs,omitempty"`
	OutputTokens int64           `json:"outputTokens,omitempty"`
	DurationMs   int64           `json:"durationMs,omitempty"`
	Trace        json.RawMessage `json:"trace,omitempty"`
	Compacted    bool            `json:"compacted,omitempty"`
	CreatedAt    int64           `json:"createdAt"`
}

// ParsedTrace decodes the message's trace. ok is false when the message has
// none, or when it is not a trace (old messages that were compacted).
func (m Message) ParsedTrace() (Trace, bool) {
	if len(m.Trace) == 0 || string(m.Trace) == "null" {
		return Trace{}, false
	}
	var t Trace
	if err := json.Unmarshal(m.Trace, &t); err != nil || len(t.Steps) == 0 {
		return Trace{}, false
	}
	return t, true
}

// Roles used by the backend.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Trace is the ordered record of one assistant turn.
type Trace struct {
	Steps []TraceStep `json:"steps"`
}

// TraceStep is one model call plus the results of the tools it asked for.
type TraceStep struct {
	Number       int         `json:"number"`
	FinishReason string      `json:"finishReason,omitempty"`
	Parts        []TracePart `json:"parts"`
}

// Trace part kinds.
const (
	PartReasoning = "reasoning"
	PartText      = "text"
	PartTool      = "tool"
)

// Tool statuses.
const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusOK      = "ok"
	StatusError   = "error"
	StatusSkipped = "skipped"
)

// TracePart is a stretch of reasoning, a stretch of text, or one tool call.
type TracePart struct {
	Kind       string `json:"kind"`
	Text       string `json:"text,omitempty"`
	CallID     string `json:"callId,omitempty"`
	Name       string `json:"name,omitempty"`
	Server     string `json:"server,omitempty"`
	Input      string `json:"input,omitempty"`
	Status     string `json:"status,omitempty"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
}

// Usage is the token count of a turn.
type Usage struct {
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
	Model        string `json:"model"`
}

// Done is the payload of the final "done" event.
type Done struct {
	Message        Message `json:"message"`
	Usage          Usage   `json:"usage"`
	Interrupted    bool    `json:"interrupted"`
	Steps          int     `json:"steps,omitempty"`
	StepsExhausted bool    `json:"stepsExhausted,omitempty"`
}

// ModelInfo is one selectable model.
type ModelInfo struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Provider string `json:"provider"`
}

// ProviderInfo groups the models of one usable provider (GET /models).
type ProviderInfo struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	DefaultModel string      `json:"defaultModel"`
	Models       []ModelInfo `json:"models"`
}

// ProviderRow is a configured provider (GET /providers), usable or not.
type ProviderRow struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	BaseURL      string `json:"baseUrl"`
	APIKey       string `json:"apiKey"`
	Connected    bool   `json:"connected"`
	Enabled      bool   `json:"enabled"`
	DefaultModel string `json:"defaultModel"`
	ModelCount   int    `json:"modelCount"`
}

// ProviderUpdate changes a provider. A nil field is left alone.
type ProviderUpdate struct {
	Name    *string `json:"name,omitempty"`
	BaseURL *string `json:"baseUrl,omitempty"`
	APIKey  *string `json:"apiKey,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
}

// Connector is an MCP server registered with the backend.
type Connector struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
	Enabled bool   `json:"enabled"`
}

// CreateChatRequest opens a new conversation.
type CreateChatRequest struct {
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model,omitempty"`
	Mode          string `json:"mode,omitempty"`
	WorkspacePath string `json:"workspacePath,omitempty"`
}

// SendRequest is one user turn.
type SendRequest struct {
	Content    string `json:"content"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	Regenerate bool   `json:"regenerate,omitempty"`
	// Unattended says nobody is watching: nothing is put to a person for
	// approval, and the turn runs inside Scope.
	Unattended bool       `json:"unattended,omitempty"`
	Scope      *TaskScope `json:"scope,omitempty"`
}

// TaskScope is what an unattended turn may do, beyond reading and editing files
// in the project. Only a person sets it.
type TaskScope struct {
	MaxTier  int      `json:"maxTier"`
	Commands []string `json:"commands,omitempty"`
}

// ScheduledTask is one instruction the scheduler will run, once, at a time of its own.
type ScheduledTask struct {
	ID        string `json:"id"`
	Prompt    string `json:"prompt"`
	ChatID    string `json:"chatId"`
	RunAt     int64  `json:"runAt"`
	Enabled   bool   `json:"enabled"`
	LastRunAt int64  `json:"lastRunAt"`
	LastStatus string `json:"lastStatus"`
	LastError  string `json:"lastError"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// ScheduledTaskInput is the fields a new task is booked with.
type ScheduledTaskInput struct {
	Prompt string `json:"prompt"`
	RunAt  int64  `json:"runAt"`
}

// ScheduledTaskPatch is what an edit can change about a task.
type ScheduledTaskPatch struct {
	Prompt  *string `json:"prompt,omitempty"`
	RunAt   *int64  `json:"runAt,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
}

// Approval is the agent asking to do something that needs a person's say-so.
type Approval struct {
	ID       string `json:"id"`
	ChatID   string `json:"chatId"`
	Tool     string `json:"tool"`
	Tier     int    `json:"tier"`
	TierName string `json:"tierName"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail"`
	// CanRemember is false for actions that are asked every time.
	CanRemember bool   `json:"canRemember"`
	Pattern     string `json:"pattern"`
}

// Approval answers the backend understands.
const (
	AllowOnce    = "allow_once"
	AllowPattern = "allow_pattern"
	AllowRun     = "allow_run"
	Deny         = "deny"
)
