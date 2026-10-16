package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider"
	"github.com/zendev-sh/goai/provider/google"
	"github.com/zendev-sh/goai/provider/groq"
)

// ErrUnsupportedProvider is returned for a provider the backend does not serve.
var ErrUnsupportedProvider = errors.New("ai: unsupported provider")

// Message is a provider-agnostic conversation turn.
type Message struct {
	Role    string
	Content string
}

// Registry resolves provider IDs to GoAI language models.
type Registry struct {
	geminiKey string
	groqKey   string
	priority  []string
}

// NewRegistry builds a registry from the configured provider credentials.
// priority controls the order in which available providers are offered.
func NewRegistry(geminiKey, groqKey string, priority []string) *Registry {
	return &Registry{geminiKey: geminiKey, groqKey: groqKey, priority: priority}
}

// ProviderOrder returns the configured providers that actually have
// credentials, in priority order. Any configured provider missing from the
// priority list is appended so it is never silently dropped.
func (r *Registry) ProviderOrder() []string {
	seen := make(map[string]bool)
	order := make([]string, 0, 2)

	for _, id := range r.priority {
		id = strings.ToLower(strings.TrimSpace(id))
		if id == "" || seen[id] || !r.Available(id) {
			continue
		}
		seen[id] = true
		order = append(order, id)
	}
	for _, id := range []string{ProviderGoogle, ProviderGroq} {
		if !seen[id] && r.Available(id) {
			seen[id] = true
			order = append(order, id)
		}
	}
	return order
}

// Available reports whether a provider has credentials configured.
func (r *Registry) Available(providerID string) bool {
	switch providerID {
	case ProviderGoogle:
		return r.geminiKey != ""
	case ProviderGroq:
		return r.groqKey != ""
	default:
		return false
	}
}

func (r *Registry) resolve(providerID, model string) (provider.LanguageModel, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("ai: empty model id for provider %q", providerID)
	}

	switch providerID {
	case ProviderGoogle:
		if r.geminiKey == "" {
			return nil, fmt.Errorf("ai: %s API key is not configured", ProviderGoogle)
		}
		return google.Chat(model, google.WithAPIKey(r.geminiKey)), nil
	case ProviderGroq:
		if r.groqKey == "" {
			return nil, fmt.Errorf("ai: %s API key is not configured", ProviderGroq)
		}
		return groq.Chat(model, groq.WithAPIKey(r.groqKey)), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedProvider, providerID)
	}
}

// Usage reports token consumption for one completion.
type Usage struct {
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
	Model        string `json:"model"`
}

// Reply is the outcome of a completion. Reasoning is the model's thinking text
// when the provider exposes it, and ThinkingMs is how long it spent producing
// it.
type Reply struct {
	Text       string
	Reasoning  string
	ThinkingMs int64
	Usage      Usage
}

// Handlers receives incremental output from a streaming completion. Both
// callbacks are optional; returning an error aborts the stream.
type Handlers struct {
	OnText      func(delta string) error
	OnReasoning func(delta string) error
}

// Stream runs a single completion, calling the handlers for every text and
// reasoning delta as it arrives. A non-nil error may still come with partial
// text, which callers are expected to persist so a cancelled or failed turn is
// not silently lost.
func (r *Registry) Stream(
	ctx context.Context,
	providerID, model, system string,
	messages []Message,
	handlers Handlers,
) (Reply, error) {
	lm, err := r.resolve(providerID, model)
	if err != nil {
		return Reply{}, err
	}

	opts := make([]goai.Option, 0, 3)
	if strings.TrimSpace(system) != "" {
		opts = append(opts, goai.WithSystem(system))
	}
	opts = append(opts, goai.WithMessages(toGoAIMessages(messages)...))

	stream, err := goai.StreamText(ctx, lm, opts...)
	if err != nil {
		return Reply{}, err
	}

	var (
		text        strings.Builder
		reasoning   strings.Builder
		streamErr   error
		startedAt   = time.Now()
		thinkingEnd time.Time
	)

	// current assembles a reply from whatever has arrived so far, so partial
	// output survives an abort or a provider failure.
	current := func() Reply {
		out := Reply{Text: text.String(), Reasoning: reasoning.String()}
		if out.Reasoning != "" && !thinkingEnd.IsZero() {
			out.ThinkingMs = thinkingEnd.Sub(startedAt).Milliseconds()
		}
		return out
	}

	for chunk := range stream.Stream() {
		switch chunk.Type {
		case provider.ChunkText:
			if chunk.Text == "" {
				continue
			}
			text.WriteString(chunk.Text)
			if handlers.OnText != nil {
				if err := handlers.OnText(chunk.Text); err != nil {
					return current(), err
				}
			}
		case provider.ChunkReasoning:
			if chunk.Text == "" {
				continue
			}
			reasoning.WriteString(chunk.Text)
			thinkingEnd = time.Now()
			if handlers.OnReasoning != nil {
				if err := handlers.OnReasoning(chunk.Text); err != nil {
					return current(), err
				}
			}
		case provider.ChunkError:
			if chunk.Error != nil {
				streamErr = chunk.Error
			}
		}
	}

	if streamErr != nil {
		return current(), streamErr
	}
	if err := stream.Err(); err != nil {
		return current(), err
	}

	reply := current()
	if result := stream.Result(); result != nil {
		// TextResult.Text includes reasoning tokens, so only fall back to it
		// when nothing was accumulated from the text deltas. Steps[n].Text is
		// text-only and is the safer fallback.
		if strings.TrimSpace(reply.Text) == "" {
			var steps strings.Builder
			for _, step := range result.Steps {
				steps.WriteString(step.Text)
			}
			if strings.TrimSpace(steps.String()) != "" {
				reply.Text = steps.String()
			} else if strings.TrimSpace(result.Text) != "" {
				reply.Text = result.Text
			}
		}
		if reply.Reasoning == "" {
			reply.Reasoning = result.Reasoning
			if reply.Reasoning != "" {
				reply.ThinkingMs = time.Since(startedAt).Milliseconds()
			}
		}
		reply.Usage = Usage{
			InputTokens:  result.TotalUsage.InputTokens,
			OutputTokens: result.TotalUsage.OutputTokens,
			Model:        result.Response.Model,
		}
	}
	return reply, nil
}

func toGoAIMessages(messages []Message) []provider.Message {
	out := make([]provider.Message, 0, len(messages))
	for _, m := range messages {
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		if m.Role == RoleAssistant {
			out = append(out, goai.AssistantMessage(content))
			continue
		}
		out = append(out, goai.UserMessage(content))
	}
	return out
}
