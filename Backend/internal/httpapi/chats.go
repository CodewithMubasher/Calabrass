package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/ai"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// --- health -----------------------------------------------------------------

// handleHealth reports which providers are usable, so the UI can warn early.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	providers := map[string]bool{}
	for _, p := range s.registry.Providers(r.Context()) {
		providers[p.ID] = p.Connected && p.Enabled
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"providers": providers,
	})
}

// --- models -----------------------------------------------------------------

// handleModels lists the models of every usable provider. Providers that are
// turned off, or have no credentials, are left out by the registry itself.
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
		// WorkspaceID binds the chat to one workspace's folder. Empty means the
		// default workspace.
		WorkspaceID string `json:"workspaceId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	workspaceDir := ""
	if strings.TrimSpace(req.WorkspaceID) != "" {
		record, err := s.store.GetWorkspace(r.Context(), req.WorkspaceID)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusBadRequest, errors.New("unknown workspace"))
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		workspaceDir = record.Dir
	}

	providerID, modelID := s.resolveModel(r.Context(), req.Provider, req.Model)

	chat, err := s.store.CreateChat(r.Context(), providerID, modelID, workspaceDir)
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
		// Pinned is a pointer so "leave it alone" is different from false.
		Pinned *bool `json:"pinned"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	chatID := r.PathValue("chatID")
	chat, err := s.store.UpdateChat(r.Context(), chatID, req.Title, req.Provider, req.Model)
	if err == nil && req.Pinned != nil {
		chat, err = s.store.SetChatPinned(r.Context(), chatID, *req.Pinned)
	}
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

// handleDeleteMessage removes a single turn, which is how the UI drops a reply
// it no longer wants.
func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	err := s.store.DeleteMessage(r.Context(), r.PathValue("chatID"), r.PathValue("messageID"))
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

// dropLastReply deletes a trailing assistant message so a regeneration replaces
// it rather than adding a second answer. A chat ending in a user turn is left
// alone, so "retry" still works before anything was answered.
func (s *Server) dropLastReply(ctx context.Context, chatID string, messages []store.Message) ([]store.Message, error) {
	if len(messages) == 0 {
		return messages, nil
	}
	last := messages[len(messages)-1]
	if last.Role != ai.RoleAssistant {
		return messages, nil
	}
	if err := s.store.DeleteMessage(ctx, chatID, last.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		return messages, err
	}
	return messages[:len(messages)-1], nil
}

// --- messages (SSE) ---------------------------------------------------------

type sendRequest struct {
	Content  string `json:"content"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	// Regenerate drops the trailing assistant reply and writes a new one from
	// the same user turn, instead of storing a second user message.
	Regenerate bool `json:"regenerate"`
}

// deltaPayload carries one incremental chunk of either answer text or
// reasoning text.
type deltaPayload struct {
	Text string `json:"text"`
}

// tracePayload carries the running record of a turn: what the model thought,
// said and ran, in order. It is re-sent whole on every change, so a client that
// joins late still ends up with the same picture.
type tracePayload struct {
	Trace ai.Trace `json:"trace"`
}

type donePayload struct {
	Message     store.Message `json:"message"`
	Usage       ai.Usage      `json:"usage"`
	Interrupted bool          `json:"interrupted"`
	// Steps is how many model turns the harness used, and StepsExhausted
	// reports that it stopped because the step budget ran out.
	Steps          int  `json:"steps,omitempty"`
	StepsExhausted bool `json:"stepsExhausted,omitempty"`
}

type errorPayload struct {
	Message string `json:"message"`
}

// workspaceFor picks the folder a chat works in: the workspace it was bound to,
// or the server's default. The folder is opened on demand, so a chat keeps
// working in the directory it remembers even after the workspace row is gone.
// A binding that cannot be opened falls back to the default rather than running
// the turn without tools.
func (s *Server) workspaceFor(chat store.Chat) *agent.Workspace {
	dir := strings.TrimSpace(chat.WorkspaceDir)
	if dir == "" || dir == agent.DefaultWorkspaceName || s.library == nil {
		return s.workspace
	}
	opened, err := s.library.Ensure(dir)
	if err != nil {
		s.log.Warn("could not open the chat's workspace",
			"chat", chat.ID, "workspace", dir, "error", err)
		return s.workspace
	}
	return opened
}

// handleSendMessage persists the user turn, then streams the assistant reply as
// Server-Sent Events: chat, message, reasoning*, token*, done|error.
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
		writeError(w, http.StatusBadRequest, errors.New("no model is available; add a provider key or turn a provider on under Providers"))
		return
	}

	history := chat.Messages

	if req.Regenerate {
		// Replace the previous answer rather than stacking a second one on
		// top: the user turn is already stored, so nothing is added for it.
		history, err = s.dropLastReply(ctx, chatID, history)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}

	var userMessage store.Message
	if !req.Regenerate {
		// Persist the user turn before generating, so it survives a failed
		// reply.
		userMessage, err = s.store.AddMessage(ctx, store.Message{
			ChatID:  chatID,
			Role:    ai.RoleUser,
			Content: req.Content,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}

	// Auto-title from the first user message and remember the chosen model.
	title := ""
	if !req.Regenerate && len(chat.Messages) == 0 {
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
	if !req.Regenerate {
		if err := stream.event("message", userMessage); err != nil {
			return
		}
	}

	turns := make([]ai.Message, 0, len(history)+1)
	for _, m := range history {
		turns = append(turns, ai.Message{Role: m.Role, Content: m.Content, Replay: m.Replay})
	}
	if !req.Regenerate {
		turns = append(turns, ai.Message{Role: ai.RoleUser, Content: req.Content})
	}

	// The harness: the tools for the workspace, and a prompt that explains the
	// project the model is working in. Without a workspace the turn is a plain
	// completion, which is what a fresh checkout without a project looks like.
	workspace := s.workspaceFor(chat.Chat)
	tools := agent.Tools(workspace)
	system := s.system
	if workspace != nil {
		system = agent.SystemPrompt(workspace, s.system)
	}

	started := time.Now()
	var lastTrace ai.Trace
	reply, genErr := s.registry.Stream(ctx, ai.StreamRequest{
		ProviderID: providerID,
		Model:      modelID,
		System:     system,
		Messages:   turns,
		Tools:      tools,
		MaxSteps:   agent.DefaultMaxSteps,
	}, ai.Handlers{
		OnText: func(delta string) error {
			return stream.event("token", deltaPayload{Text: delta})
		},
		OnReasoning: func(delta string) error {
			return stream.event("reasoning", deltaPayload{Text: delta})
		},
		OnTrace: func(trace ai.Trace) error {
			lastTrace = trace
			return stream.event("trace", tracePayload{Trace: trace})
		},
	})
	elapsed := time.Since(started)

	// The visible answer is the model's closing text; narration from earlier
	// steps stays in the trace, where the tool calls it introduces live.
	content := reply.Text
	if answer := reply.Trace.AnswerText(); strings.TrimSpace(answer) != "" {
		content = answer
	}
	var savedTrace json.RawMessage
	if len(lastTrace.Steps) > 0 {
		if data, err := json.Marshal(lastTrace); err == nil {
			savedTrace = data
		}
	}

	// Persist whatever we got, even when the stream failed or the client
	// disconnected mid-generation.
	if strings.TrimSpace(content) != "" || strings.TrimSpace(reply.Reasoning) != "" || len(savedTrace) > 0 {
		// Detach from the request context: a cancelled request must still be
		// able to write the partial reply.
		saveCtx := context.WithoutCancel(ctx)
		assistant, saveErr := s.store.AddMessage(saveCtx, store.Message{
			ChatID:       chatID,
			Role:         ai.RoleAssistant,
			Content:      content,
			Reasoning:    reply.Reasoning,
			ThinkingMs:   reply.ThinkingMs,
			OutputTokens: int64(reply.Usage.OutputTokens),
			DurationMs:   elapsed.Milliseconds(),
			Trace:        savedTrace,
			Replay:       reply.Replay,
		})
		if saveErr != nil {
			s.log.Error("could not persist assistant reply", "chat", chatID, "error", saveErr)
		} else {
			_ = stream.event("done", donePayload{
				Message:        assistant,
				Usage:          reply.Usage,
				Interrupted:    genErr != nil,
				Steps:          reply.Steps,
				StepsExhausted: reply.StepsExhausted,
			})
		}
	}

	if genErr != nil {
		s.log.Warn("generation failed", "chat", chatID, "provider", providerID, "model", modelID, "error", genErr)
		_ = stream.event("error", errorPayload{Message: genErr.Error()})
	}
}

// resolveModel picks the provider/model for a turn, falling back to the first
// provider that has credentials and is turned on.
func (s *Server) resolveModel(ctx context.Context, providerID, modelID string) (string, string) {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)

	if !s.registry.Usable(ctx, providerID) {
		providerID = ""
		for _, id := range s.registry.ProviderOrder(ctx) {
			providerID = id
			break
		}
	}
	if providerID == "" {
		return "", ""
	}

	if modelID == "" || !s.registry.ValidModel(ctx, providerID, modelID) {
		modelID = s.registry.DefaultModel(ctx, providerID)
	}
	return providerID, modelID
}
