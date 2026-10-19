// Package gmail provides a client for Google's Gmail API using OAuth2.
package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// Client wraps the Gmail API service with OAuth2 token management.
type Client struct {
	service *gmailapi.Service
	config  *oauth2.Config
	token   *oauth2.Token
	tokenPath string
	logger *log.Logger
	mu      sync.Mutex
	authed  bool
}

// Message represents a Gmail message in a simplified form.
type Message struct {
	ID        string
	ThreadID  string
	Subject   string
	From      string
	To        string
	Date      string
	Snippet   string
	Body      string
	Labels    []string
}

// Draft represents a Gmail draft.
type Draft struct {
	ID      string
	Message Message
}

// Config holds the OAuth2 configuration for Gmail.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	TokenPath    string
}

// DefaultConfig returns the default OAuth2 config using Google's official endpoints.
func DefaultConfig() *Config {
	return &Config{
		ClientID:     "",
		ClientSecret: "",
		RedirectURL:  "http://localhost:8080/oauth2callback",
		TokenPath:    defaultTokenPath(),
	}
}

// defaultTokenPath returns the default path for storing OAuth tokens.
func defaultTokenPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".calabrass", "gmail_token.json")
}

// NewClient creates a new Gmail client and authenticates immediately.
// Use this for the -auth flag or when you need immediate authentication.
func NewClient(ctx context.Context, config *Config, logger *log.Logger) (*Client, error) {
	if logger == nil {
		logger = log.New(os.Stderr, "gmail: ", 0)
	}

	// Load client ID/secret from environment if not set
	if config.ClientID == "" {
		config.ClientID = os.Getenv("GMAIL_CLIENT_ID")
	}
	if config.ClientSecret == "" {
		config.ClientSecret = os.Getenv("GMAIL_CLIENT_SECRET")
	}

	if config.ClientID == "" || config.ClientSecret == "" {
		if err := loadConfigFromFile(config); err != nil {
			return nil, fmt.Errorf("Gmail credentials not configured. Set GMAIL_CLIENT_ID and GMAIL_CLIENT_SECRET environment variables, or create a config file at ~/.calabrass/gmail_config.json: %w", err)
		}
	}

	logger.Printf("Loaded config: ClientID=%s", mask(config.ClientID))

	oauthConfig := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,
		Scopes: []string{
			gmailapi.GmailReadonlyScope,
			gmailapi.GmailSendScope,
			gmailapi.GmailModifyScope,
			gmailapi.GmailComposeScope,
		},
		Endpoint: google.Endpoint,
	}

	client := &Client{
		config:    oauthConfig,
		tokenPath: config.TokenPath,
		logger:    logger,
	}

	// Load existing token
	if err := client.loadToken(); err != nil {
		logger.Printf("no existing token: %v", err)
	} else {
		logger.Printf("loaded existing token")
	}

	// Authenticate immediately (opens browser)
	if err := client.ensureAuthed(ctx); err != nil {
		return nil, err
	}

	// Create Gmail API service
	httpClient := client.config.Client(ctx, client.token)
	service, err := gmailapi.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gmail service: %w", err)
	}
	client.service = service

	return client, nil
}

// NewLazyClient creates a client that defers OAuth until first tool call.
// This allows the MCP server to start and respond to initialize immediately.
func NewLazyClient(ctx context.Context, config *Config, logger *log.Logger) *Client {
	if logger == nil {
		logger = log.New(os.Stderr, "gmail: ", 0)
	}

	// Load client ID/secret from environment if not set
	if config.ClientID == "" {
		config.ClientID = os.Getenv("GMAIL_CLIENT_ID")
	}
	if config.ClientSecret == "" {
		config.ClientSecret = os.Getenv("GMAIL_CLIENT_SECRET")
	}

	oauthConfig := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,
		Scopes: []string{
			gmailapi.GmailReadonlyScope,
			gmailapi.GmailSendScope,
			gmailapi.GmailModifyScope,
			gmailapi.GmailComposeScope,
		},
		Endpoint: google.Endpoint,
	}

	client := &Client{
		config:    oauthConfig,
		tokenPath: config.TokenPath,
		logger:    logger,
	}

	// Try to load existing token (non-blocking)
	_ = client.loadToken()

	return client
}

// ensureAuthed ensures we have a valid token and service, doing OAuth if needed.
func (c *Client) ensureAuthed(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.authed && c.service != nil && c.token != nil && c.token.Valid() {
		return nil
	}

	// Load token if we don't have one
	if c.token == nil {
		if err := c.loadToken(); err != nil {
			c.logger.Printf("no existing token: %v", err)
		}
	}

	// Try to refresh token if we have one (even if expired, try refresh if we have refresh token)
	if c.token != nil {
		if err := c.refreshTokenIfNeeded(ctx); err != nil {
			c.logger.Printf("token refresh failed: %v", err)
		}
	}

	// If token is valid, create service
	if c.token != nil && c.token.Valid() {
		httpClient := c.config.Client(ctx, c.token)
		service, err := gmailapi.NewService(ctx, option.WithHTTPClient(httpClient))
		if err == nil {
			c.service = service
			c.authed = true
			return nil
		}
		c.logger.Printf("token invalid or service creation failed: %v", err)
	}

	// Need to authenticate - use web flow
	return c.authenticateWebFlow(ctx)
}

// refreshTokenIfNeeded explicitly refreshes the OAuth token if it has a refresh token and is expired
func (c *Client) refreshTokenIfNeeded(ctx context.Context) error {
	if c.token == nil {
		return errors.New("no token to refresh")
	}

	// If token is still valid, no need to refresh
	if c.token.Valid() {
		return nil
	}

	// If no refresh token, we can't refresh
	if c.token.RefreshToken == "" {
		c.logger.Printf("token expired and no refresh token available")
		return errors.New("no refresh token available")
	}

	c.logger.Printf("refreshing expired OAuth token...")
	
	// Create a new token source that will refresh the token
	tokenSource := c.config.TokenSource(ctx, c.token)
	newToken, err := tokenSource.Token()
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	c.token = newToken
	
	// Save the refreshed token
	if err := c.saveToken(); err != nil {
		c.logger.Printf("failed to save refreshed token: %v", err)
	}
	
	c.logger.Printf("OAuth token refreshed successfully")
	return nil
}

// authenticateWebFlow uses the standard OAuth2 Authorization Code flow with local server.
// This is what works with "Desktop app" OAuth client type.
func (c *Client) authenticateWebFlow(ctx context.Context) error {
	// Build auth URL with ALL required parameters
	authURL := c.config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	
	// Debug: print the actual auth URL
	c.logger.Printf("DEBUG: Auth URL: %s", authURL)
	
	c.logger.Printf("Opening browser for Gmail authentication...")
	c.logger.Printf("If browser doesn't open, visit: %s", authURL)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	
	mux := http.NewServeMux()
	server := &http.Server{Addr: ":8080", Handler: mux}
	
	mux.HandleFunc("/oauth2callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback")
			return
		}
		codeCh <- code
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Authentication successful! You can close this window."))
	})
	
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	defer server.Close()

	// Open browser
	cmd := exec.Command("cmd", "/c", "start", "", authURL)
	cmd.Start()

	select {
	case code := <-codeCh:
		token, err := c.config.Exchange(ctx, code)
		if err != nil {
			return fmt.Errorf("failed to exchange code for token: %w", err)
		}
		c.token = token
		if err := c.saveToken(); err != nil {
			return err
		}
		// Create service with new token
		httpClient := c.config.Client(ctx, c.token)
		service, err := gmailapi.NewService(ctx, option.WithHTTPClient(httpClient))
		if err != nil {
			return fmt.Errorf("failed to create Gmail service: %w", err)
		}
		c.service = service
		c.authed = true
		return nil
	case err := <-errCh:
		return fmt.Errorf("OAuth callback error: %w", err)
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Minute):
		return errors.New("authentication timeout (5 minutes)")
	}
}

// getService returns the Gmail service, authenticating if needed.
func (c *Client) getService(ctx context.Context) (*gmailapi.Service, error) {
	if err := c.ensureAuthed(ctx); err != nil {
		return nil, err
	}
	return c.service, nil
}

// doWithRetry executes a function that uses the Gmail service, retrying with token refresh on 401 errors.
func (c *Client) doWithRetry(ctx context.Context, fn func(*gmailapi.Service) error) error {
	for attempts := 0; attempts < 2; attempts++ {
		service, err := c.getService(ctx)
		if err != nil {
			return err
		}
		
		err = fn(service)
		if err == nil {
			return nil
		}
		
		// Check if it's a 401 error (token expired/invalid)
		if isAuthError(err) {
			c.logger.Printf("auth error detected, refreshing token: %v", err)
			c.mu.Lock()
			c.authed = false
			c.service = nil
			c.mu.Unlock()
			
			// Force token refresh
			if err := c.refreshTokenIfNeeded(ctx); err != nil {
				c.logger.Printf("failed to refresh token: %v", err)
			}
			continue // retry
		}
		
		return err
	}
	
	return fmt.Errorf("max retries exceeded")
}

// isAuthError checks if the error is an authentication/authorization error (401)
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "401") || 
		strings.Contains(errStr, "invalid authentication credentials") ||
		strings.Contains(errStr, "invalid_grant") ||
		strings.Contains(errStr, "unauthorized")
}

// loadToken loads the saved OAuth2 token from disk.
func (c *Client) loadToken() error {
	data, err := os.ReadFile(c.tokenPath)
	if err != nil {
		return err
	}
	
	// Parse with custom handling for empty expiry
	type tokenJSON struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		Expiry       string `json:"expiry"`
	}
	
	var tj tokenJSON
	if err := json.Unmarshal(data, &tj); err != nil {
		return err
	}
	
	c.token = &oauth2.Token{
		AccessToken:  tj.AccessToken,
		RefreshToken: tj.RefreshToken,
		TokenType:    tj.TokenType,
	}
	
	if tj.Expiry != "" {
		expiry, err := time.Parse(time.RFC3339, tj.Expiry)
		if err != nil {
			// Try alternative format (without timezone)
			expiry, err = time.Parse("2006-01-02T15:04:05", tj.Expiry)
			if err != nil {
				// Try with Z suffix
				expiry, err = time.Parse(time.RFC3339, tj.Expiry+"Z")
				if err != nil {
					c.logger.Printf("could not parse expiry: %v", err)
				} else {
					c.token.Expiry = expiry
				}
			} else {
				c.token.Expiry = expiry
			}
		} else {
			c.token.Expiry = expiry
		}
	}
	
	return nil
}

// saveToken saves the OAuth2 token to disk.
func (c *Client) saveToken() error {
	data, err := json.Marshal(c.token)
	if err != nil {
		return err
	}
	dir := filepath.Dir(c.tokenPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(c.tokenPath, data, 0600)
}

// loadConfigFromFile loads OAuth config from a JSON file.
func loadConfigFromFile(config *Config) error {
	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, ".calabrass", "gmail_config.json")
	
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("could not read config file at %s: %w", configPath, err)
	}
	
	type fileConfig struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	
	var fc fileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		return err
	}
	
	config.ClientID = fc.ClientID
	config.ClientSecret = fc.ClientSecret
	return nil
}

// ListMessages retrieves messages matching the query.
func (c *Client) ListMessages(ctx context.Context, query string, maxResults int64) ([]Message, error) {
	var messages []Message
	err := c.doWithRetry(ctx, func(service *gmailapi.Service) error {
		call := service.Users.Messages.List("me").Q(query).MaxResults(maxResults)
		resp, err := call.Do()
		if err != nil {
			return fmt.Errorf("failed to list messages: %w", err)
		}
		
		for _, msg := range resp.Messages {
			m, err := c.GetMessage(ctx, msg.Id)
			if err != nil {
				c.logger.Printf("failed to get message %s: %v", msg.Id, err)
				continue
			}
			messages = append(messages, *m)
		}
		
		return nil
	})
	return messages, err
}

// GetMessage retrieves a single message by ID.
func (c *Client) GetMessage(ctx context.Context, id string) (*Message, error) {
	var msg *Message
	err := c.doWithRetry(ctx, func(service *gmailapi.Service) error {
		call := service.Users.Messages.Get("me", id).Format("full")
		resp, err := call.Do()
		if err != nil {
			return fmt.Errorf("failed to get message: %w", err)
		}
		
		msg = c.parseMessage(resp)
		return nil
	})
	return msg, err
}

// parseMessage converts a Gmail API message to our simplified Message.
func (c *Client) parseMessage(msg *gmailapi.Message) *Message {
	m := &Message{
		ID:       msg.Id,
		ThreadID: msg.ThreadId,
		Labels:   msg.LabelIds,
		Snippet:  msg.Snippet,
	}
	
	for _, header := range msg.Payload.Headers {
		switch strings.ToLower(header.Name) {
		case "subject":
			m.Subject = header.Value
		case "from":
			m.From = header.Value
		case "to":
			m.To = header.Value
		case "date":
			m.Date = header.Value
		}
	}
	
	m.Body = c.extractBody(msg.Payload)
	return m
}

// extractBody extracts the text body from a message payload.
func (c *Client) extractBody(payload *gmailapi.MessagePart) string {
	if payload.MimeType == "text/plain" && payload.Body.Data != "" {
		data, _ := base64URLDecode(payload.Body.Data)
		return string(data)
	}
	
	if payload.MimeType == "text/html" && payload.Body.Data != "" {
		data, _ := base64URLDecode(payload.Body.Data)
		return stripHTML(string(data))
	}
	
	for _, part := range payload.Parts {
		body := c.extractBody(part)
		if body != "" {
			return body
		}
	}
	
	return ""
}

// SendMessage sends an email.
func (c *Client) SendMessage(ctx context.Context, to, subject, body string) error {
	return c.doWithRetry(ctx, func(service *gmailapi.Service) error {
		message := fmt.Sprintf("To: %s\r\nSubject: %s\r\n\r\n%s", to, subject, body)
		encoded := base64URLEncode([]byte(message))
		
		gmailMsg := &gmailapi.Message{Raw: encoded}
		
		_, err := service.Users.Messages.Send("me", gmailMsg).Do()
		if err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}
		return nil
	})
}

// CreateDraft creates a draft email.
func (c *Client) CreateDraft(ctx context.Context, to, subject, body string) (*Draft, error) {
	var draft *Draft
	err := c.doWithRetry(ctx, func(service *gmailapi.Service) error {
		message := fmt.Sprintf("To: %s\r\nSubject: %s\r\n\r\n%s", to, subject, body)
		encoded := base64URLEncode([]byte(message))
		
		d := &gmailapi.Draft{
			Message: &gmailapi.Message{Raw: encoded},
		}
		
		resp, err := service.Users.Drafts.Create("me", d).Do()
		if err != nil {
			return fmt.Errorf("failed to create draft: %w", err)
		}
		
		draft = &Draft{ID: resp.Id}
		return nil
	})
	return draft, err
}

// ListDrafts lists all drafts.
func (c *Client) ListDrafts(ctx context.Context, maxResults int64) ([]Draft, error) {
	var drafts []Draft
	err := c.doWithRetry(ctx, func(service *gmailapi.Service) error {
		call := service.Users.Drafts.List("me").MaxResults(maxResults)
		resp, err := call.Do()
		if err != nil {
			return fmt.Errorf("failed to list drafts: %w", err)
		}
		
		for _, d := range resp.Drafts {
			drafts = append(drafts, Draft{ID: d.Id})
		}
		return nil
	})
	return drafts, err
}

// ModifyLabels adds/removes labels on a message.
func (c *Client) ModifyLabels(ctx context.Context, id string, addLabels, removeLabels []string) error {
	return c.doWithRetry(ctx, func(service *gmailapi.Service) error {
		mod := &gmailapi.ModifyMessageRequest{
			AddLabelIds:    addLabels,
			RemoveLabelIds: removeLabels,
		}
		
		_, err := service.Users.Messages.Modify("me", id, mod).Do()
		if err != nil {
			return fmt.Errorf("failed to modify labels: %w", err)
		}
		return nil
	})
}

// ListLabels lists all labels.
func (c *Client) ListLabels(ctx context.Context) ([]*gmailapi.Label, error) {
	var labels []*gmailapi.Label
	err := c.doWithRetry(ctx, func(service *gmailapi.Service) error {
		call := service.Users.Labels.List("me")
		resp, err := call.Do()
		if err != nil {
			return fmt.Errorf("failed to list labels: %w", err)
		}
		
		labels = resp.Labels
		return nil
	})
	return labels, err
}

// base64URLEncode encodes data using base64 URL encoding.
func base64URLEncode(data []byte) string {
	return base64.URLEncoding.EncodeToString(data)
}

// base64URLDecode decodes base64 URL encoded data.
func base64URLDecode(s string) ([]byte, error) {
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	return base64.URLEncoding.DecodeString(s)
}

// stripHTML removes HTML tags from a string.
func stripHTML(s string) string {
	re := regexp.MustCompile(`(?s)<(script|style)[^>]*>.*?</$1>`)
	s = re.ReplaceAllString(s, "")
	re = regexp.MustCompile(`<[^>]+>`)
	s = re.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "<", "<")
	s = strings.ReplaceAll(s, ">", ">")
	s = strings.ReplaceAll(s, "&", "&")
	s = strings.ReplaceAll(s, "\u0022", "\"")
	re = regexp.MustCompile(`\s+`)
	s = re.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// mask returns a masked version of a string for logging.
func mask(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}