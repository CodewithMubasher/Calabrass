package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/ai"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
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
	Logger    *slog.Logger
}

// Server holds the HTTP handlers.
type Server struct {
	store     *store.Store
	registry  *ai.Registry
	system    string
	library   *agent.Library
	workspace *agent.Workspace
	log       *slog.Logger
}

// New builds the fully wired HTTP handler.
func New(deps Deps) http.Handler {
	log := deps.Logger
	if log == nil {
		log = slog.Default()
	}

	s := &Server{
		store:     deps.Store,
		registry:  deps.Registry,
		system:    deps.SystemPrompt,
		library:   deps.Library,
		workspace: deps.Workspace,
		log:       log,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/models", s.handleModels)
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
	mux.HandleFunc("GET /api/chats", s.handleListChats)
	mux.HandleFunc("POST /api/chats", s.handleCreateChat)
	mux.HandleFunc("GET /api/chats/{chatID}", s.handleGetChat)
	mux.HandleFunc("PATCH /api/chats/{chatID}", s.handleUpdateChat)
	mux.HandleFunc("DELETE /api/chats/{chatID}", s.handleDeleteChat)
	mux.HandleFunc("POST /api/chats/{chatID}/messages", s.handleSendMessage)
	mux.HandleFunc("DELETE /api/chats/{chatID}/messages/{messageID}", s.handleDeleteMessage)

	return s.withCORS(s.withLogging(mux))
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
