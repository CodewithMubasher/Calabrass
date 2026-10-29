package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/ai"
	"github.com/CodewithMubasher/Calabrass/backend/internal/crew"
	"github.com/CodewithMubasher/Calabrass/backend/internal/permission"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// --- health -----------------------------------------------------------------

// handleHealth reports that the API is up. It is deliberately public: the
// desktop shell probes it before a person has signed in, and it says nothing
// about any account's providers.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// A face uses this to decide whether to attach to what it found, so it says
	// what is running and who owns it. Nothing here is secret.
	pub := s.hostPublic()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"version":    pub.Version,
		"owner":      pub.Owner,
		"background": pub.Background,
		"pid":        pub.PID,
	})
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
		// Mode is the chat's operating mode: "chat" (the ordinary one) or
		// "crew" (the long-horizon one).
		Mode string `json:"mode"`
		// WorkspaceID binds the chat to one workspace's folder. Empty means the
		// default workspace.
		WorkspaceID string `json:"workspaceId"`
		// WorkspacePath binds the chat straight to a folder on this machine.
		// The TUI uses it so a chat works in the directory the app was opened
		// from; WorkspaceID wins when both are sent.
		WorkspacePath string `json:"workspacePath"`
		// AgentID binds the chat to an agent's configuration. When set, the agent's
		// provider, model, connector and system prompt are used.
		AgentID string `json:"agentId"`
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
	} else if strings.TrimSpace(req.WorkspacePath) != "" {
		if !localRequest(r) {
			writeError(w, http.StatusForbidden,
				errors.New("workspacePath is only available to clients on this machine"))
			return
		}
		dir, err := chatWorkspacePath(req.WorkspacePath)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		workspaceDir = dir
	}

	// If agentId is provided, load the agent and use its configuration
	agentID := strings.TrimSpace(req.AgentID)
	providerID := strings.TrimSpace(req.Provider)
	modelID := strings.TrimSpace(req.Model)
	// The mode the request asked for. Empty means the caller did not say, and
	// the agent's own mode — or the ordinary one — stands in.
	mode := agent.NormalMode(req.Mode)
	if mode == "" && strings.TrimSpace(req.Mode) != "" {
		writeError(w, http.StatusBadRequest, errors.New(`mode must be "chat" or "crew"`))
		return
	}

	if agentID != "" {
		agentRec, _, err := s.resolveAgent(r.Context(), user.ID, agentID)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusBadRequest, errors.New("unknown agent"))
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		// Use agent's provider/model if not explicitly provided
		if providerID == "" {
			providerID = agentRec.Provider
		}
		if modelID == "" {
			modelID = agentRec.Model
		}
		// The agent's mode is the default a chat of its own starts in.
		if mode == "" {
			mode = agent.NormalMode(agentRec.Mode)
		}
		// The Orchestrator is the crew, so a chat with it is a crew chat even
		// when the composer left its mode on chat.
		if crew.IsOrchestrator(agentID) {
			mode = agent.ModeCrew
		}
		// If still no provider/model, resolve from registry
		if providerID == "" || modelID == "" {
			providerID, modelID = s.resolveModelFor(r.Context(), user.ID, providerID, modelID)
		}
	} else {
		providerID, modelID = s.resolveModelFor(r.Context(), user.ID, providerID, modelID)
	}

	if mode == "" {
		mode = agent.ModeChat
	}

	// A crew chat that named no agent belongs to the Orchestrator. Crew mode is
	// how the crew is reached; without this the ordinary coding harness answers
	// and does the work itself instead of writing a todo and delegating.
	if agentID == "" && mode == agent.ModeCrew {
		agentID = crew.BuiltinOrchestratorID
	}

	if providerID == "" || modelID == "" {
		writeError(w, http.StatusBadRequest, errors.New("no model is available; add a provider key or turn a provider on under Providers"))
		return
	}

	chat, err := s.store.CreateChat(r.Context(), providerID, modelID, workspaceDir, agentID, mode, user.ID)
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
		// Mode is the chat's operating mode: "chat" or "crew".
		Mode string `json:"mode"`
		// Pinned is a pointer so "leave it alone" is different from false.
		Pinned *bool `json:"pinned"`
		// WorkspaceID moves the chat to another workspace. A pointer so an
		// absent field leaves the binding alone; an empty value resets it to
		// the default workspace.
		WorkspaceID *string `json:"workspaceId"`
		// WorkspacePath moves the chat to a folder on this machine. WorkspaceID
		// wins when both are sent.
		WorkspacePath *string `json:"workspacePath"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	chatID := r.PathValue("chatID")

	// Read the current binding before changing anything. A mode switch has to
	// know which agent the chat is attached to: the Orchestrator only makes
	// sense in crew mode, and crew mode with no agent means the Orchestrator.
	current, err := s.store.GetChatRecord(r.Context(), chatID, user.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Validate the mode before writing anything, so a bad request cannot leave
	// half of an update behind.
	if req.Mode != "" && !agent.ValidMode(req.Mode) {
		writeError(w, http.StatusBadRequest, errors.New(`mode must be "chat" or "crew"`))
		return
	}

	// Resolve the workspace before writing anything, so an unknown id changes
	// nothing else about the chat.
	workspaceDir := ""
	if req.WorkspaceID != nil {
		id := strings.TrimSpace(*req.WorkspaceID)
		if id != "" {
			record, err := s.store.GetWorkspace(r.Context(), id, user.ID)
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
	} else if req.WorkspacePath != nil && strings.TrimSpace(*req.WorkspacePath) != "" {
		if !localRequest(r) {
			writeError(w, http.StatusForbidden,
				errors.New("workspacePath is only available to clients on this machine"))
			return
		}
		dir, err := chatWorkspacePath(*req.WorkspacePath)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		workspaceDir = dir
	}

	chat, err := s.store.UpdateChat(r.Context(), chatID, req.Title, req.Provider, req.Model, user.ID)
	if err == nil && (req.WorkspaceID != nil || req.WorkspacePath != nil) {
		chat, err = s.store.SetChatWorkspace(r.Context(), chatID, workspaceDir, user.ID)
	}
	if err == nil && req.Pinned != nil {
		chat, err = s.store.SetChatPinned(r.Context(), chatID, *req.Pinned, user.ID)
	}
	if err == nil && req.Mode != "" {
		mode := agent.NormalMode(req.Mode)
		chat, err = s.store.SetChatMode(r.Context(), chatID, mode, user.ID)
		if err == nil {
			// Keep the agent binding consistent with the mode. A chat moved to
			// crew with no agent gets the Orchestrator; a chat moved out of crew
			// drops an Orchestrator binding, because the orchestrator prompt is
			// delegate-only and would leave the chat unable to do ordinary work.
			desiredAgent := current.AgentID
			switch {
			case mode == agent.ModeCrew && strings.TrimSpace(desiredAgent) == "":
				desiredAgent = crew.BuiltinOrchestratorID
			case mode == agent.ModeChat && crew.IsOrchestrator(desiredAgent):
				desiredAgent = ""
			}
			if desiredAgent != current.AgentID {
				chat, err = s.store.SetChatAgent(r.Context(), chatID, desiredAgent, user.ID)
			}
		}
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

// maxHistoryMessages bounds the conversation replay. It is large enough to hold
// the recent work and small enough that uncompacted tool replays do not fill the
// window on their own; compaction keeps the same number of messages.
const maxHistoryMessages = 20

// historyWindow returns the first user turn plus the most recent messages. The
// first turn is the one place the original objective and constraints live, so it
// is pinned rather than allowed to slide out of the window. If it is already in
// the tail it is not duplicated.
func historyWindow(history []store.Message, max int) []store.Message {
	if max <= 0 {
		return nil
	}
	if len(history) <= max {
		return history
	}
	tail := history[len(history)-max:]
	firstUser := -1
	for i := 0; i < len(history)-max; i++ {
		if history[i].Role == ai.RoleUser {
			firstUser = i
			break
		}
	}
	if firstUser < 0 {
		return tail
	}
	window := make([]store.Message, 0, max+1)
	window = append(window, history[firstUser])
	window = append(window, tail...)
	return window
}

// runLimitNote turns a harness stop into one honest sentence for the saved
// answer. The model may have written a confident summary before a limit cut it
// off; the note is what keeps that summary from reading as a finished task.
func runLimitNote(reply ai.Reply, state *agent.RunState) string {
	if state != nil {
		if reason, stopped := state.StopReason(); stopped {
			switch {
			case strings.Contains(reason, "tool budget"):
				return "_Run note: the run reached its tool budget before finishing, so this answer may be incomplete._"
			case strings.Contains(reason, "failed in a row"):
				return "_Run note: the run stopped after repeated tool failures, so this answer may be incomplete._"
			default:
				return "_Run note: the run was stopped by a harness limit before finishing, so this answer may be incomplete._"
			}
		}
	}
	if reply.StepsExhausted {
		return fmt.Sprintf("_Run note: the run reached its step limit (%d steps) before finishing, so this answer may be incomplete._", reply.Steps)
	}
	return ""
}

// workspaceFor picks the folder a chat works in for the shipped admin account.
// Handlers use workspaceForUser with the signed-in account.
func (s *Server) workspaceFor(chat store.Chat) *agent.Workspace {
	return s.workspaceForUser("", chat)
}

// localRequest reports whether the request came from this machine. Choosing a
// folder by its path only makes sense for a client that shares the backend's
// filesystem, and keeps a remotely reachable server from being told to work in
// an arbitrary directory.
func localRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// chatWorkspacePath checks a folder a local client asked a chat to work in.
// It must be an absolute, existing directory: a relative value would otherwise
// silently mean a folder inside the account's workspace library.
func chatWorkspacePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if !filepath.IsAbs(path) {
		return "", errors.New("workspacePath must be an absolute folder")
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("workspacePath: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspacePath %s is not a folder", path)
	}
	return path, nil
}

// workspaceForUser picks the folder a chat works in: the workspace it was bound
// to, or the account's default. The folder is opened on demand, so a chat keeps
// working in the directory it remembers even after the workspace row is gone.
// A binding that cannot be opened falls back to the default rather than running
// the turn without tools.
func (s *Server) workspaceForUser(userID string, chat store.Chat) *agent.Workspace {
	dir := strings.TrimSpace(chat.WorkspaceDir)
	// A local client (the TUI) binds a chat to the folder it was opened in, so
	// the path is absolute and sits outside the account's workspace library.
	if filepath.IsAbs(dir) {
		opened, err := agent.NewWorkspace(dir)
		if err != nil {
			s.log.Warn("could not open the chat's workspace",
				"chat", chat.ID, "workspace", dir, "error", err)
			return s.workspaceForUser(userID, store.Chat{ID: chat.ID})
		}
		return opened
	}

	lib := s.libraryFor(userID)
	if lib == nil {
		return nil
	}

	if dir == "" || dir == agent.DefaultWorkspaceName {
		if userID == "" {
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
	// Tools may still be finishing when this handler returns; they must not
	// write to a response that has gone.
	defer stream.close()

	if err := stream.event("chat", updated); err != nil {
		return
	}
	if !req.Regenerate {
		if err := stream.event("message", userMessage); err != nil {
			return
		}
	}

	turns := make([]ai.Message, 0, maxHistoryMessages+1)
	// The first user turn carries the objective and the constraints; losing it
	// to a window makes the model re-ask or drift. Keep it pinned in front of
	// the most recent messages, which carry the current state of the work.
	window := historyWindow(history, maxHistoryMessages)
	for _, m := range window {
		turns = append(turns, ai.Message{Role: m.Role, Content: m.Content, Replay: m.Replay})
	}
	if !req.Regenerate {
		turns = append(turns, ai.Message{Role: ai.RoleUser, Content: req.Content})
	}

	// The harness: the prompt that explains what the model is working with — the
	// project, and the machine the connectors reach — and the tools it may call.
	// A person is watching this turn, so what needs their say-so is put to them
	// on this very stream, and can be answered from any other face as well.
	turnCtx := permission.WithSession(ctx, s.interactiveSession(user.ID, chatID, "", stream))
	setup := s.setupTurnFor(turnCtx, user.ID, s.workspaceForUser(user.ID, chat.Chat), chatID, chat.AgentID)

	started := time.Now()
	var lastTrace ai.Trace
	reply, genErr := s.registryFor(user.ID).Stream(ctx, ai.StreamRequest{
		ProviderID:  providerID,
		Model:       modelID,
		System:      setup.system,
		Messages:    turns,
		Tools:       setup.tools,
		ToolOrigins: setup.origins,
		MaxSteps:    agent.MaxSteps(setup.mode),
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
		OnStepFinish: func(step int, usage ai.Usage) error {
			// Record usage for each step as it completes
			if usage.InputTokens+usage.OutputTokens > 0 {
				s.recordUsageFor(ctx, user.ID, providerID, modelID, usage)
			}
			return nil
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
	// The saved answer has to say when a limit cut the run short. The tokens
	// already streamed cannot be changed, but the final message can carry the
	// truth, and it is the final message that a reload or a later turn sees.
	if note := runLimitNote(reply, setup.runState); note != "" && !strings.Contains(content, note) {
		if strings.TrimSpace(content) == "" {
			content = note
		} else {
			content = strings.TrimSpace(content) + "\n\n" + note
		}
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

			// Compact old messages to keep context size bounded. Keep the same
			// number of messages the history window can show uncompressed, so
			// the window never consists mostly of stale summaries.
			if _, err := s.store.CompactMessages(ctx, chatID, maxHistoryMessages); err != nil {
				s.log.Warn("could not compact messages", "chat", chatID, "error", err)
			}
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
