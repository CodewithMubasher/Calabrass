package httpapi

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/ai"
	"github.com/CodewithMubasher/Calabrass/backend/internal/connectors"
	"github.com/CodewithMubasher/Calabrass/backend/internal/quota"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
	"github.com/CodewithMubasher/Calabrass/backend/internal/voice"
)

// Deps are the collaborators the HTTP layer needs.
type Deps struct {
	Store        *store.Store
	Registry     *ai.Registry
	SystemPrompt string
	// Library holds the workspace folders. When it is nil the workspace
	// endpoints report unavailable.
	Library *agent.Library
	// Workspace is the project the agent may read and change — the workspace a
	// chat uses until it picks one of its own. When it is nil the chat runs
	// without tools.
	Workspace *agent.Workspace
	// MistralKey is the credential Cal Live listens with. Empty falls back to
	// Groq's Whisper.
	MistralKey string
	// VoiceProvider and VoiceModel let the voice agent think with a faster model
	// than the one that does its work. Empty means "use the chat model".
	VoiceProvider string
	VoiceModel    string
	// Connectors holds the MCP servers the agent may start. When it is nil one
	// is built here, which is what tests get.
	Connectors *connectors.Pool
	// Quotas holds the free-tier limits read from model_free_tier_quotas.md.
	// Nil means no limits are known, which is how a missing file is handled.
	Quotas *quota.Catalog
	Logger *slog.Logger
}

// Server holds the HTTP handlers.
type Server struct {
	store         *store.Store
	registry      *ai.Registry
	system        string
	library       *agent.Library
	workspace     *agent.Workspace
	mistralKey    string
	speaker       voice.Speaker
	voiceProvider string
	voiceModelID  string
	connectors    *connectors.Pool
	quotas        *quota.Catalog
	log           *slog.Logger
	// handler is the routed and wrapped mux, kept so the server itself can be
	// handed to http.Server.
	handler http.Handler
}

// New builds the fully wired HTTP handler.
func New(deps Deps) http.Handler {
	return NewServer(deps)
}

// NewServer builds the server and its routes. main holds the result so it can
// start the scheduler on the same wiring and hand the same value to
// http.Server.
func NewServer(deps Deps) *Server {
	log := deps.Logger
	if log == nil {
		log = slog.Default()
	}

	pool := deps.Connectors
	if pool == nil {
		pool = connectors.New(deps.Store, "", log)
	}

	s := &Server{
		store:         deps.Store,
		registry:      deps.Registry,
		system:        deps.SystemPrompt,
		library:       deps.Library,
		workspace:     deps.Workspace,
		mistralKey:    deps.MistralKey,
		speaker:       voice.NewSpeaker(deps.MistralKey),
		voiceProvider: deps.VoiceProvider,
		voiceModelID:  deps.VoiceModel,
		connectors:    pool,
		quotas:        deps.Quotas,
		log:           log,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/models", s.handleModels)
	mux.HandleFunc("GET /api/usage", s.handleUsage)
	mux.HandleFunc("GET /api/providers", s.handleListProviders)
	mux.HandleFunc("POST /api/providers", s.handleCreateProvider)
	mux.HandleFunc("PUT /api/providers/{providerID}", s.handleUpdateProvider)
	mux.HandleFunc("DELETE /api/providers/{providerID}", s.handleDeleteProvider)
	mux.HandleFunc("GET /api/workspaces", s.handleListWorkspaces)
	mux.HandleFunc("POST /api/workspaces", s.handleCreateWorkspace)
	mux.HandleFunc("DELETE /api/workspaces/{workspaceID}", s.handleDeleteWorkspace)
	mux.HandleFunc("GET /api/connectors", s.handleListConnectors)
	mux.HandleFunc("POST /api/connectors", s.handleCreateConnector)
	mux.HandleFunc("PATCH /api/connectors/{connectorID}", s.handleUpdateConnector)
	mux.HandleFunc("DELETE /api/connectors/{connectorID}", s.handleDeleteConnector)
	mux.HandleFunc("GET /api/connectors/{connectorID}/tools", s.handleConnectorTools)
	mux.HandleFunc("GET /api/agents", s.handleListAgents)
	mux.HandleFunc("POST /api/agents", s.handleCreateAgent)
	mux.HandleFunc("PUT /api/agents/{agentID}", s.handleUpdateAgent)
	mux.HandleFunc("DELETE /api/agents/{agentID}", s.handleDeleteAgent)
	mux.HandleFunc("GET /api/voice", s.handleVoiceStream)
	mux.HandleFunc("GET /api/artifacts/stream", s.handleArtifactStream)
	mux.HandleFunc("GET /api/artifacts/content", s.handleArtifactContent)
	mux.HandleFunc("GET /api/artifacts/file", s.handleArtifactDownload)
	mux.HandleFunc("GET /api/artifacts/archive", s.handleArtifactArchive)
	mux.HandleFunc("GET /api/chats", s.handleListChats)
	mux.HandleFunc("POST /api/chats", s.handleCreateChat)
	mux.HandleFunc("GET /api/chats/{chatID}", s.handleGetChat)
	mux.HandleFunc("PATCH /api/chats/{chatID}", s.handleUpdateChat)
	mux.HandleFunc("DELETE /api/chats/{chatID}", s.handleDeleteChat)
	mux.HandleFunc("POST /api/chats/{chatID}/messages", s.handleSendMessage)
	mux.HandleFunc("DELETE /api/chats/{chatID}/messages/{messageID}", s.handleDeleteMessage)
	mux.HandleFunc("GET /api/schedules", s.handleListSchedules)
	mux.HandleFunc("POST /api/schedules", s.handleCreateSchedule)
	mux.HandleFunc("GET /api/schedules/stream", s.handleScheduleStream)
	mux.HandleFunc("PATCH /api/schedules/{taskID}", s.handleUpdateSchedule)
	mux.HandleFunc("DELETE /api/schedules/{taskID}", s.handleDeleteSchedule)

	s.handler = s.withCORS(s.withLogging(mux))
	return s
}

// ServeHTTP is what lets the server be handed to http.Server directly, so main
// can keep the same value it started the scheduler with.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// --- middleware -------------------------------------------------------------

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack lets a handler take the connection over. The voice socket needs it, and
// without this the recorder hides the interface from the upgrade check — which
// is what turned a WebSocket handshake into a 501.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start).Round(time.Millisecond).String(),
		)
	})
}

// withCORS permits the local dev server (any localhost port) to call the API.
func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if isLocalOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLocalOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	for _, prefix := range []string{
		"http://localhost:", "http://127.0.0.1:", "http://[::1]:",
	} {
		if strings.HasPrefix(origin, prefix) {
			return true
		}
	}
	return false
}

// --- helpers ----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("write response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func decodeJSON(r *http.Request, target any) error {
	defer func() { _, _ = io.Copy(io.Discard, r.Body) }()

	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return nil // empty body is fine; all fields are optional
		}
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

const titleMaxLength = 48

func titleFrom(text string) string {
	clean := strings.Join(strings.Fields(text), " ")
	if clean == "" {
		return "New Chat"
	}
	runes := []rune(clean)
	if len(runes) > titleMaxLength {
		return string(runes[:titleMaxLength]) + "..."
	}
	return clean
}

// --- SSE --------------------------------------------------------------------

type sseWriter struct {
	w  http.ResponseWriter
	fl http.Flusher
}

// newSSEWriter starts a text/event-stream response.
func newSSEWriter(w http.ResponseWriter) (*sseWriter, error) {
	fl, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming is not supported by this server")
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fl.Flush()

	return &sseWriter{w: w, fl: fl}, nil
}

func (s *sseWriter) event(name string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, data); err != nil {
		return err
	}
	s.fl.Flush()
	return nil
}
