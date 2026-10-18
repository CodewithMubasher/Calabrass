package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zendev-sh/goai"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/ai"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// voice.go is the agent half of Cal Live: one heard utterance in, one answer out.
// The audio half — the socket, the listener, the speaker — is in voice_ws.go.

// voiceChat finds the chat this conversation belongs to, or opens a new one. A
// chat that has gone missing is replaced rather than failing the turn.
func (s *Server) voiceChat(ctx context.Context, userID, chatID string) (store.ChatDetail, error) {
	if chatID != "" {
		if chat, err := s.store.GetChat(ctx, chatID, userID); err == nil {
			return chat, nil
		}
	}

	providerID, modelID := s.resolveModelFor(ctx, userID, "", "")
	chat, err := s.store.CreateChat(ctx, providerID, modelID, "", userID)
	if err != nil {
		return store.ChatDetail{}, err
	}
	return store.ChatDetail{Chat: chat}, nil
}

// voiceTurn runs one heard message through the voice agent: a tiny prompt and
// the single delegate tool, so it decides quickly and the coding agent does the
// work. The reply's tokens are handed to onText as they arrive, so the answer
// can be read aloud while it is still being written.
func (s *Server) voiceTurn(ctx context.Context, userID string, chat store.ChatDetail, transcript string, onText func(string)) (string, ai.Trace, error) {
	providerID, modelID := s.voiceModel(ctx, userID, chat)
	if providerID == "" || modelID == "" {
		return "", ai.Trace{}, errors.New("no model is available; add a provider key or turn a provider on under Providers")
	}

	if len(chat.Messages) == 0 {
		if updated, err := s.store.UpdateChat(ctx, chat.ID, titleFrom(transcript), "", "", userID); err == nil {
			chat.Chat = updated
		}
	}

	// One tool, and a prompt small enough to keep a spoken answer quick: the
	// voice agent decides, and the coding agent does.
	tools := []goai.Tool{agent.DelegateTool(func(ctx context.Context, args agent.DelegateArgs) (string, error) {
		return s.delegateWork(ctx, userID, chat, args)
	})}

	turns := make([]ai.Message, 0, len(chat.Messages)+1)
	for _, m := range chat.Messages {
		turns = append(turns, ai.Message{Role: m.Role, Content: m.Content, Replay: m.Replay})
	}
	turns = append(turns, ai.Message{Role: ai.RoleUser, Content: transcript})

	started := time.Now()
	var lastTrace ai.Trace
	reply, genErr := s.registryFor(userID).Stream(ctx, ai.StreamRequest{
		ProviderID: providerID,
		Model:      modelID,
		System:     agent.VoiceSystemPrompt,
		Messages:   turns,
		Tools:      tools,
		MaxSteps:   agent.DefaultMaxSteps,
	}, ai.Handlers{
		OnText:      func(delta string) error { onText(delta); return nil },
		OnReasoning: func(string) error { return nil },
		OnTrace:     func(trace ai.Trace) error { lastTrace = trace; return nil },
	})
	elapsed := time.Since(started)
	if genErr == nil || reply.Usage.InputTokens+reply.Usage.OutputTokens > 0 {
		s.recordUsageFor(ctx, userID, providerID, modelID, reply.Usage)
	}
	if genErr != nil {
		return "", ai.Trace{}, fmt.Errorf("the reply failed: %w", genErr)
	}

	// The answer is the model's closing text; narration from earlier steps stays
	// in the trace, where the tool calls it introduces live.
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

	// Detach from the request: a client that stops listening must not lose the
	// turn it just had.
	saveCtx := context.WithoutCancel(ctx)
	if _, err := s.store.AddMessage(saveCtx, store.Message{
		ChatID:  chat.ID,
		Role:    ai.RoleUser,
		Content: transcript,
	}, userID); err != nil {
		s.log.Error("could not persist the spoken turn", "chat", chat.ID, "error", err)
	}
	if _, err := s.store.AddMessage(saveCtx, store.Message{
		ChatID:       chat.ID,
		Role:         ai.RoleAssistant,
		Content:      content,
		Reasoning:    reply.Reasoning,
		ThinkingMs:   reply.ThinkingMs,
		OutputTokens: int64(reply.Usage.OutputTokens),
		DurationMs:   elapsed.Milliseconds(),
		Trace:        savedTrace,
		Replay:       reply.Replay,
	}, userID); err != nil {
		s.log.Error("could not persist the spoken reply", "chat", chat.ID, "error", err)
	}

	return content, lastTrace, nil
}

// delegateWork is what the delegate tool hands over: the full coding agent —
// every tool, the big model, the whole project prompt — run on the task in the
// same workspace, reporting what it did.
func (s *Server) delegateWork(ctx context.Context, userID string, chat store.ChatDetail, args agent.DelegateArgs) (string, error) {
	providerID, modelID := s.resolveModelFor(ctx, userID, "", "")
	if providerID == "" || modelID == "" {
		return "", errors.New("no model is available to do that work")
	}

	workspace := s.workspaceForUser(userID, chat.Chat)
	setup := s.setupTurnFor(ctx, userID, workspace, chat.ID)

	// The worker continues the conversation, so it knows what has been said
	// already, and the task is its instruction.
	turns := make([]ai.Message, 0, len(chat.Messages)+1)
	for _, m := range chat.Messages {
		turns = append(turns, ai.Message{Role: m.Role, Content: m.Content, Replay: m.Replay})
	}
	task := args.Task
	if strings.TrimSpace(args.Context) != "" {
		task += "\n\nContext from the conversation: " + args.Context
	}
	turns = append(turns, ai.Message{Role: ai.RoleUser, Content: task})

	reply, err := s.registryFor(userID).Stream(ctx, ai.StreamRequest{
		ProviderID:  providerID,
		Model:       modelID,
		System:      setup.system,
		Messages:    turns,
		Tools:       setup.tools,
		ToolOrigins: setup.origins,
		MaxSteps:    agent.DefaultMaxSteps,
	}, ai.Handlers{
		OnText:      func(string) error { return nil },
		OnReasoning: func(string) error { return nil },
	})
	if err != nil {
		return "", fmt.Errorf("the work failed: %w", err)
	}
	s.recordUsageFor(ctx, userID, providerID, modelID, reply.Usage)

	answer := reply.Text
	if final := reply.Trace.AnswerText(); strings.TrimSpace(final) != "" {
		answer = final
	}
	if strings.TrimSpace(answer) == "" {
		answer = "Done."
	}
	return answer, nil
}

// voiceModel is what the voice agent itself thinks with. The work it hands over
// runs on the chat model, so this one only has to decide what to say. Set
// CALABRASS_VOICE_PROVIDER and CALABRASS_VOICE_MODEL to give it a faster model.
func (s *Server) voiceModel(ctx context.Context, userID string, chat store.ChatDetail) (string, string) {
	if strings.TrimSpace(s.voiceProvider) != "" || strings.TrimSpace(s.voiceModelID) != "" {
		if providerID, modelID := s.resolveModelFor(ctx, userID, s.voiceProvider, s.voiceModelID); providerID != "" {
			return providerID, modelID
		}
	}
	return s.resolveModelFor(ctx, userID, chat.Provider, chat.Model)
}

// groqKey is the key the audio endpoints use: the one stored for the account's
// Groq provider if there is one, otherwise the key from the environment.
func (s *Server) groqKey(ctx context.Context, userID string) string {
	if record, err := s.store.GetProvider(ctx, "groq", userID); err == nil && record.APIKey != "" {
		return record.APIKey
	}
	if reg := s.registryFor(userID); reg != nil {
		return reg.GroqKey()
	}
	return ""
}
