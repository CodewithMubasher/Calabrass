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

// handleHealth reports that the API is up. It is deliberately public: the
// desktop shell probes it before a person has signed in, and it says nothing
// about any account's providers.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// --- models -----------------------------------------------------------------

// handleModels lists the models of every usable provider. Providers that are
// turned off, or have no credentials, are left out by the registry itself.
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("refresh") != ""
	reg := s.registryFor(currentUser(r.Context()).ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"providers": reg.Models(r.Context(), force),
	})
}

// --- chats ------------------------------------------------------------------

func (s *Server) handleListChats(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	chats, err := s.store.ListChats(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chats": chats})
}

func (s *Server) handleCreateChat(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
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
		record, err := s.store.GetWorkspace(r.Context(), req.WorkspaceID, user.ID)
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

	providerID, modelID := s.resolveModelFor(r.Context(), user.ID, req.Provider, req.Model)

	chat, err := s.store.CreateChat(r.Context(), providerID, modelID, workspaceDir, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"chat": chat})
}

func (s *Server) handleGetChat(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	chat, err := s.store.GetChat(r.Context(), r.PathValue("chatID"), user.ID)
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
	user := currentUser(r.Context())
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
	chat, err := s.store.UpdateChat(r.Context(), chatID, req.Title, req.Provider, req.Model, user.ID)
	if err == nil && req.Pinned != nil {
		chat, err = s.store.SetChatPinned(r.Context(), chatID, *req.Pinned, user.ID)
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
	user := currentUser(r.Context())
	err := s.store.DeleteChat(r.Context(), r.PathValue("chatID"), user.ID)
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
	user := currentUser(r.Context())
	err := s.store.DeleteMessage(r.Context(), r.PathValue("chatID"), r.PathValue("messageID"), user.ID)
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
func (s *Server) dropLastReply(ctx context.Context, userID, chatID string, messages []store.Message) ([]store.Message, error) {
	if len(messages) == 0 {
		return messages, nil
	}
	last := messages[len(messages)-1]
	if last.Role != ai.RoleAssistant {
		return messages, nil
	}
	if err := s.store.DeleteMessage(ctx, chatID, last.ID, userID); err != nil && !errors.Is(err, store.ErrNotFound) {
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

// workspaceFor picks the folder a chat works in for the shipped admin account.
// Handlers use workspaceForUser with the signed-in account.
func (s *Server) workspaceFor(chat store.Chat) *agent.Workspace {
	return s.workspaceForUser("", chat)
}

// workspaceForUser picks the folder a chat works in: the workspace it was bound
// to, or the account's default. The folder is opened on demand, so a chat keeps
// working in the directory it remembers even after the workspace row is gone.
// A binding that cannot be opened falls back to the default rather than running
// the turn without tools.
func (s *Server) workspaceForUser(userID string, chat store.Chat) *agent.Workspace {
	lib := s.libraryFor(userID)
	if lib == nil {
		return nil
	}

	dir := strings.TrimSpace(chat.WorkspaceDir)
	if dir == "" || dir == agent.DefaultWorkspaceName {
		if userID == "" || userID == s.adminID {
			if s.workspace != nil {
				return s.workspace
			}
		}
		opened, err := lib.Ensure(agent.DefaultWorkspaceName)
		if err != nil {
			s.log.Warn("could not open the default workspace", "account", userID, "error", err)
			return nil
		}
		return opened
	}
	opened, err := lib.Ensure(dir)
	if err != nil {
		s.log.Warn("could not open the chat's workspace",
			"chat", chat.ID, "workspace", dir, "error", err)
		return s.workspaceForUser(userID, store.Chat{ID: chat.ID})
	}
	return opened
}

// handleSendMessage persists the user turn, then streams the assistant reply as
// Server-Sent Events: chat, message, reasoning*, token*, done|error.
func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := currentUser(ctx)
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

	chat, err := s.store.GetChat(ctx, chatID, user.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	providerID, modelID := s.resolveModelFor(ctx, user.ID, firstNonEmpty(req.Provider, chat.Provider), firstNonEmpty(req.Model, chat.Model))
	if providerID == "" || modelID == "" {
		writeError(w, http.StatusBadRequest, errors.New("no model is available; add a provider key or turn a provider on under Providers"))
		return
	}

	history := chat.Messages

	if req.Regenerate {
		// Replace the previous answer rather than stacking a second one on
		// top: the user turn is already stored, so nothing is added for it.
		history, err = s.dropLastReply(ctx, user.ID, chatID, history)
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
		}, user.ID)
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
	updated, err := s.store.UpdateChat(ctx, chatID, title, providerID, modelID, user.ID)
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

	// The harness: the prompt that explains what the model is working with — the
	// project, and the machine the connectors reach — and the tools it may call.
	setup := s.setupTurnFor(ctx, user.ID, s.workspaceForUser(user.ID, chat.Chat), chatID)

	started := time.Now()
	var lastTrace ai.Trace
	reply, genErr := s.registryFor(user.ID).Stream(ctx, ai.StreamRequest{
		ProviderID:  providerID,
		Model:       modelID,
		System:      setup.system,
		Messages:    turns,
		Tools:       setup.tools,
		ToolOrigins: setup.origins,
		MaxSteps:    agent.DefaultMaxSteps,
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

	// The call counts against the provider's allowance once it reached the
	// model: a failure before that leaves no usage behind.
	if genErr == nil || reply.Usage.InputTokens+reply.Usage.OutputTokens > 0 {
		s.recordUsageFor(ctx, user.ID, providerID, modelID, reply.Usage)
	}

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
		}, user.ID)
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

// resolveModel picks the provider/model for the shipped admin account. Handlers
// use resolveModelFor with the signed-in account.
func (s *Server) resolveModel(ctx context.Context, providerID, modelID string) (string, string) {
	return s.resolveModelFor(ctx, "", providerID, modelID)
}

// resolveModelFor picks the provider/model for a turn within one account,
// falling back to the first provider that has credentials and is turned on.
func (s *Server) resolveModelFor(ctx context.Context, userID, providerID, modelID string) (string, string) {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	reg := s.registryFor(userID)
	if reg == nil {
		return "", ""
	}

	if !reg.Usable(ctx, providerID) {
		providerID = ""
		for _, id := range reg.ProviderOrder(ctx) {
			providerID = id
			break
		}
	}
	if providerID == "" {
		return "", ""
	}

	if modelID == "" || !reg.ValidModel(ctx, providerID, modelID) {
		modelID = reg.DefaultModel(ctx, providerID)
	}
	return providerID, modelID
}
