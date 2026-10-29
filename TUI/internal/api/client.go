package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// DefaultAddr is where the backend listens unless told otherwise.
const DefaultAddr = "127.0.0.1:8787"

// Client talks to one backend.
type Client struct {
	Base  string // e.g. "http://127.0.0.1:8787"
	Token string

	http   *http.Client // short calls
	stream *http.Client // no overall timeout: a reply can take minutes
}

// New returns a client for addr, which may be "host:port" or a full URL.
func New(addr string) *Client {
	return &Client{
		Base:   BaseURL(addr),
		http:   &http.Client{Timeout: 30 * time.Second},
		stream: &http.Client{},
	}
}

// BaseURL turns "host:port" into "http://host:port" and trims a trailing slash.
func BaseURL(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = DefaultAddr
	}
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	return strings.TrimRight(addr, "/")
}

// APIError is an error answer from the backend ({"error": "..."}).
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("backend answered %d", e.Status)
	}
	return e.Message
}

// IsUnauthorized reports a 401: the token is missing, expired or wrong.
func IsUnauthorized(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusUnauthorized
}

// IsAPIError reports whether the backend answered (as opposed to being
// unreachable).
func IsAPIError(err error) bool {
	var ae *APIError
	return errors.As(err, &ae)
}

// do sends a JSON request and decodes a JSON answer into out (when non-nil).
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.authorize(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return readAPIError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) authorize(req *http.Request) {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	// A host on another machine only answers paired devices. The code is read
	// from the environment, so it is never written to this terminal's settings.
	if code := strings.TrimSpace(os.Getenv("CALABRASS_PAIRING_CODE")); code != "" {
		req.Header.Set("X-Calabrass-Pairing", code)
	}
}

func readAPIError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var payload struct {
		Error string `json:"error"`
	}
	msg := ""
	if json.Unmarshal(data, &payload) == nil {
		msg = payload.Error
	}
	if msg == "" {
		msg = strings.TrimSpace(string(data))
	}
	return &APIError{Status: resp.StatusCode, Message: msg}
}

// --- health + auth --------------------------------------------------------

// Health succeeds when the backend is up.
func (c *Client) Health(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/api/health", nil, nil)
}

// SignUp creates an account and returns its first session.
func (c *Client) SignUp(ctx context.Context, name, email, password string) (Session, error) {
	var s Session
	err := c.do(ctx, http.MethodPost, "/api/auth/signup",
		map[string]string{"name": name, "email": email, "password": password}, &s)
	return s, err
}

// Login signs in with a password.
func (c *Client) Login(ctx context.Context, email, password string) (Session, error) {
	var s Session
	err := c.do(ctx, http.MethodPost, "/api/auth/login",
		map[string]string{"email": email, "password": password}, &s)
	return s, err
}

// LocalSignIn signs in as the machine's owner, proving it with the secret the
// host wrote to the data folder. create makes the owner account when nobody has
// one yet.
func (c *Client) LocalSignIn(ctx context.Context, secret string, create bool) (Session, error) {
	var s Session
	err := c.do(ctx, http.MethodPost, "/api/auth/local",
		map[string]any{"secret": secret, "create": create}, &s)
	return s, err
}

// DecideApproval answers an approval the agent is waiting on.
func (c *Client) DecideApproval(ctx context.Context, id, decision string) error {
	return c.do(ctx, http.MethodPost, "/api/approvals/"+url.PathEscape(id),
		map[string]string{"decision": decision}, nil)
}

// Me resolves the token to its account.
func (c *Client) Me(ctx context.Context) (User, error) {
	var out struct {
		User User `json:"user"`
	}
	err := c.do(ctx, http.MethodGet, "/api/auth/me", nil, &out)
	return out.User, err
}

// --- models + providers ---------------------------------------------------

// Models lists the models of every usable provider. refresh asks the backend
// to re-read the provider catalogs instead of using its cache.
func (c *Client) Models(ctx context.Context, refresh bool) ([]ProviderInfo, error) {
	path := "/api/models"
	if refresh {
		path += "?refresh=1"
	}
	var out struct {
		Providers []ProviderInfo `json:"providers"`
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out.Providers, err
}

// Providers lists every configured provider, usable or not.
func (c *Client) Providers(ctx context.Context) ([]ProviderRow, error) {
	var out struct {
		Providers []ProviderRow `json:"providers"`
	}
	err := c.do(ctx, http.MethodGet, "/api/providers", nil, &out)
	return out.Providers, err
}

// UpdateProvider changes a provider and returns the new list.
func (c *Client) UpdateProvider(ctx context.Context, id string, u ProviderUpdate) ([]ProviderRow, error) {
	var out struct {
		Providers []ProviderRow `json:"providers"`
	}
	err := c.do(ctx, http.MethodPut, "/api/providers/"+url.PathEscape(id), u, &out)
	return out.Providers, err
}

// CreateProvider adds a custom provider and returns the new list.
func (c *Client) CreateProvider(ctx context.Context, u ProviderUpdate) ([]ProviderRow, error) {
	var out struct {
		Providers []ProviderRow `json:"providers"`
	}
	err := c.do(ctx, http.MethodPost, "/api/providers", u, &out)
	return out.Providers, err
}

// --- connectors (MCP) -----------------------------------------------------

// Connectors lists the MCP servers.
func (c *Client) Connectors(ctx context.Context) ([]Connector, error) {
	var out struct {
		Connectors []Connector `json:"connectors"`
	}
	err := c.do(ctx, http.MethodGet, "/api/connectors", nil, &out)
	return out.Connectors, err
}

// SetConnectorEnabled turns an MCP server on or off and returns the new list.
func (c *Client) SetConnectorEnabled(ctx context.Context, id string, enabled bool) ([]Connector, error) {
	var out struct {
		Connectors []Connector `json:"connectors"`
	}
	err := c.do(ctx, http.MethodPatch, "/api/connectors/"+url.PathEscape(id),
		map[string]bool{"enabled": enabled}, &out)
	return out.Connectors, err
}

// --- chats ----------------------------------------------------------------

// Chats lists the conversations, newest activity first as the backend orders them.
func (c *Client) Chats(ctx context.Context) ([]Chat, error) {
	var out struct {
		Chats []Chat `json:"chats"`
	}
	err := c.do(ctx, http.MethodGet, "/api/chats", nil, &out)
	return out.Chats, err
}

// CreateChat opens a new conversation.
func (c *Client) CreateChat(ctx context.Context, req CreateChatRequest) (Chat, error) {
	var out struct {
		Chat Chat `json:"chat"`
	}
	err := c.do(ctx, http.MethodPost, "/api/chats", req, &out)
	return out.Chat, err
}

// GetChat returns a conversation with all its messages.
func (c *Client) GetChat(ctx context.Context, id string) (ChatDetail, error) {
	var out struct {
		Chat ChatDetail `json:"chat"`
	}
	err := c.do(ctx, http.MethodGet, "/api/chats/"+url.PathEscape(id), nil, &out)
	return out.Chat, err
}

// RenameChat sets a conversation's title.
func (c *Client) RenameChat(ctx context.Context, id, title string) (Chat, error) {
	var out struct {
		Chat Chat `json:"chat"`
	}
	err := c.do(ctx, http.MethodPatch, "/api/chats/"+url.PathEscape(id),
		map[string]string{"title": title}, &out)
	return out.Chat, err
}

// SetChatWorkspace binds a conversation to a folder on disk.
func (c *Client) SetChatWorkspace(ctx context.Context, id, path string) (Chat, error) {
	var out struct {
		Chat Chat `json:"chat"`
	}
	err := c.do(ctx, http.MethodPatch, "/api/chats/"+url.PathEscape(id),
		map[string]string{"workspacePath": path}, &out)
	return out.Chat, err
}

// DeleteChat removes a conversation.
func (c *Client) DeleteChat(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/chats/"+url.PathEscape(id), nil, nil)
}

// --- schedules ------------------------------------------------------------

// ListSchedules returns all scheduled tasks for the current user.
func (c *Client) ListSchedules(ctx context.Context) ([]ScheduledTask, error) {
	var out struct {
		Tasks []ScheduledTask `json:"tasks"`
	}
	err := c.do(ctx, http.MethodGet, "/api/schedules", nil, &out)
	return out.Tasks, err
}

// CreateSchedule books a new scheduled task.
func (c *Client) CreateSchedule(ctx context.Context, input ScheduledTaskInput) ([]ScheduledTask, error) {
	var out struct {
		Tasks []ScheduledTask `json:"tasks"`
	}
	err := c.do(ctx, http.MethodPost, "/api/schedules", input, &out)
	return out.Tasks, err
}

// UpdateSchedule changes a task's prompt, time, or enabled state.
func (c *Client) UpdateSchedule(ctx context.Context, id string, patch ScheduledTaskPatch) ([]ScheduledTask, error) {
	var out struct {
		Tasks []ScheduledTask `json:"tasks"`
	}
	err := c.do(ctx, http.MethodPatch, "/api/schedules/"+url.PathEscape(id), patch, &out)
	return out.Tasks, err
}

// DeleteSchedule removes a scheduled task.
func (c *Client) DeleteSchedule(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/schedules/"+url.PathEscape(id), nil, nil)
}
