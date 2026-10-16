package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/CodewithMubasher/Calabrass/backend/internal/ai"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// --- health -----------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"providers": map[string]bool{
			ai.ProviderGoogle: s.registry.Available(ai.ProviderGoogle),
			ai.ProviderGroq:   s.registry.Available(ai.ProviderGroq),
		},
	})
}

// --- models -----------------------------------------------------------------

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("refresh") != ""
	writeJSON(w, http.StatusOK, map[string]any{
		"providers": s.registry.Models(r.Context(), force),
	})
}

// --- chats ------------------------------------------------------------------

func (s *Server) handleListChats(w http.ResponseWriter, r *http.Request) {
	chats, err := s.store.ListChats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chats": chats})
}

func (s *Server) handleCreateChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	providerID, modelID := s.resolveModel(r.Context(), req.Provider, req.Model)

	chat, err := s.store.CreateChat(r.Context(), providerID, modelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"chat": chat})
}

func (s *Server) handleGetChat(w http.ResponseWriter, r *http.Request) {
	chat, err := s.store.GetChat(r.Context(), r.PathValue("chatID"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chat": chat})
}

func (s *Server) handleUpdateChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title    string `json:"title"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	chat, err := s.store.UpdateChat(r.Context(), r.PathValue("chatID"), req.Title, req.Provider, req.Model)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chat": chat})
}

func (s *Server) handleDeleteChat(w http.ResponseWriter, r *http.Request) {
	err := s.store.DeleteChat(r.Context(), r.PathValue("chatID"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// --- messages (SSE) ---------------------------------------------------------

type sendRequest struct {
	Content  string `json:"content"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type tokenPayload struct {
	Text string `json:"text"`
}

type donePayload struct {
	Message     store.Message `json:"message"`
	Usage       ai.Usage      `json:"usage"`
	Interrupted bool          `json:"interrupted"`
}

type errorPayload struct {
	Message string `json:"message"`
}

// handleSendMessage persists the user turn, then streams the assistant reply as
// Server-Sent Events: chat, message, token*, done|error.
func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chatID := r.PathValue("chatID")

	var req sendRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, errors.New("content is required"))
		return
	}

	chat, err := s.store.GetChat(ctx, chatID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	providerID, modelID := s.resolveModel(ctx, firstNonEmpty(req.Provider, chat.Provider), firstNonEmpty(req.Model, chat.Model))
	if providerID == "" || modelID == "" {
		writeError(w, http.StatusBadRequest, errors.New("no model is available; check the backend provider keys"))
		return
	}

	// Persist the user turn before generating, so it survives a failed reply.
	userMessage, err := s.store.AddMessage(ctx, chatID, ai.RoleUser, req.Content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Auto-title from the first user message and remember the chosen model.
	title := ""
	if len(chat.Messages) == 0 {
		title = titleFrom(req.Content)
	}
	updated, err := s.store.UpdateChat(ctx, chatID, title, providerID, modelID)
	if err != nil {
		s.log.Warn("could not update chat", "chat", chatID, "error", err)
		updated = chat.Chat
	}

	stream, err := newSSEWriter(w)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if err := stream.event("chat", updated); err != nil {
		return
	}
	if err := stream.event("message", userMessage); err != nil {
		return
	}

	history := make([]ai.Message, 0, len(chat.Messages)+1)
	for _, m := range chat.Messages {
		history = append(history, ai.Message{Role: m.Role, Content: m.Content})
	}
	history = append(history, ai.Message{Role: ai.RoleUser, Content: req.Content})

	reply, genErr := s.registry.Stream(ctx, providerID, modelID, s.system, history, func(delta string) error {
		return stream.event("token", tokenPayload{Text: delta})
	})

	// Persist whatever text we got, even when the stream failed or the client
	// disconnected mid-generation.
	if strings.TrimSpace(reply.Text) != "" {
		// Detach from the request context: a cancelled request must still be
		// able to write the partial reply.
		saveCtx := context.WithoutCancel(ctx)
		assistant, saveErr := s.store.AddMessage(saveCtx, chatID, ai.RoleAssistant, reply.Text)
		if saveErr != nil {
			s.log.Error("could not persist assistant reply", "chat", chatID, "error", saveErr)
		} else {
			_ = stream.event("done", donePayload{
				Message:     assistant,
				Usage:       reply.Usage,
				Interrupted: genErr != nil,
			})
		}
	}

	if genErr != nil {
		s.log.Warn("generation failed", "chat", chatID, "provider", providerID, "model", modelID, "error", genErr)
		_ = stream.event("error", errorPayload{Message: genErr.Error()})
	}
}

// resolveModel picks the provider/model for a turn, falling back to the first
// configured provider and its default model.
func (s *Server) resolveModel(ctx context.Context, providerID, modelID string) (string, string) {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)

	if providerID == "" || !s.registry.Available(providerID) {
		order := s.registry.ProviderOrder()
		if len(order) == 0 {
			return "", ""
		}
		providerID = order[0]
	}
	if !s.registry.Available(providerID) {
		return "", ""
	}

	if modelID == "" || !s.registry.ValidModel(ctx, providerID, modelID) {
		modelID = s.registry.DefaultModel(ctx, providerID)
	}
	return providerID, modelID
}
