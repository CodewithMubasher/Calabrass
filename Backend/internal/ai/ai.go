package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider"
	"github.com/zendev-sh/goai/provider/compat"
	"github.com/zendev-sh/goai/provider/google"
)

// ErrUnsupportedProvider is returned for a provider the backend does not serve.
var ErrUnsupportedProvider = errors.New("ai: unsupported provider")

// maxParallelTools is how many of one step's tool calls may run at once. The
// model is asked to batch independent calls into a single step; this bounds how
// many of those actually overlap — enough to be quick, few enough that a batch
// of file writes does not all contend at once.
const maxParallelTools = 5

// Message is a provider-agnostic conversation turn.
type Message struct {
	Role    string
	Content string
	// Replay is the provider transcript a turn produced, stored as JSON. When
	// set it is sent back verbatim instead of Content, so a turn that ran tools
	// is replayed with its tool calls and results intact.
	Replay json.RawMessage
}

// Registry resolves provider IDs to GoAI language models. Every provider comes
// from the account's stored rows: nothing is built in, so a fresh account has
// none until it adds one.
type Registry struct {
	source ProviderSource
	// userID scopes the stored provider rows to one account. A registry made
	// by ForUser carries that account's id; the base registry carries none.
	userID string
	// catalog is this registry's cached model list, so one account's providers
	// cannot populate or invalidate another's.
	catalog catalog
}

// ProviderRecord is what the registry needs to know about a stored provider.
type ProviderRecord struct {
	ID      string
	Name    string
	BaseURL string
	APIKey  string
	Enabled bool
}

// ProviderSource returns the stored provider rows for one account. A registry
// with no source has no providers at all.
type ProviderSource func(ctx context.Context, userID string) ([]ProviderRecord, error)

// NewRegistry builds a registry over the providers stored in the database.
func NewRegistry(source ProviderSource) *Registry {
	return &Registry{source: source}
}

// ForUser returns a registry scoped to one account. The copy shares the
// provider source but keeps its own model catalog, so changing one account's
// providers does not disturb another's.
func (r *Registry) ForUser(userID string) *Registry {
	if r == nil {
		return nil
	}
	return &Registry{
		source: r.source,
		userID: userID,
	}
}

// resolvedProvider is a provider row joined with the adapter its endpoint calls
// for.
type resolvedProvider struct {
	ID      string
	Name    string
	BaseURL string
	APIKey  string
	Enabled bool

	defaults []string
	fetch    func(ctx context.Context, defaults []string) ([]ModelInfo, error)
}

// connected reports whether the provider has what it needs to be called: an
// endpoint. A key is optional, because a local server may take none.
func (p resolvedProvider) connected() bool {
	return p.BaseURL != ""
}

// ProviderSummary describes one provider, as the providers page needs it.
type ProviderSummary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	// Connected reports whether the provider has an endpoint.
	Connected bool `json:"connected"`
	// Enabled reports whether the provider may be used.
	Enabled bool `json:"enabled"`
	// APIKey is the provider's credential. Never serialised directly; the
	// handlers decide what to expose.
	APIKey string `json:"-"`
}

// providers returns the account's stored providers, each paired with the adapter
// and model listing its endpoint calls for. A read failure is not fatal: it
// leaves no providers, which is also what a fresh account has.
func (r *Registry) providers(ctx context.Context) []resolvedProvider {
	if r.source == nil {
		return nil
	}
	rows, err := r.source(ctx, r.userID)
	if err != nil {
		return nil
	}

	out := make([]resolvedProvider, 0, len(rows))
	for _, row := range rows {
		p := resolvedProvider{
			ID:      strings.ToLower(strings.TrimSpace(row.ID)),
			Name:    strings.TrimSpace(row.Name),
			BaseURL: NormaliseBaseURL(row.BaseURL),
			APIKey:  row.APIKey,
			Enabled: row.Enabled,
		}
		if p.ID == "" {
			continue
		}
		if p.Name == "" {
			p.Name = p.ID
		}
		switch endpointKind(p.BaseURL) {
		case endpointGemini:
			p.defaults = geminiPreferredModels
		case endpointGroq:
			p.defaults = groqPreferredModels
		}
		p.fetch = func(ctx context.Context, defaults []string) ([]ModelInfo, error) {
			switch endpointKind(p.BaseURL) {
			case endpointGemini:
				return fetchGoogleModels(ctx, p.ID, p.APIKey, defaults)
			case endpointGroq:
				return fetchGroqModels(ctx, p.ID, p.APIKey, defaults)
			}
			return fetchCompatModels(ctx, p.ID, p.BaseURL, p.APIKey, defaults)
		}
		out = append(out, p)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// GroqKey is the key of the account's Groq provider, for the audio endpoints,
// which need a transcription credential without having a provider of their own.
func (r *Registry) GroqKey(ctx context.Context) string {
	for _, p := range r.providers(ctx) {
		if endpointKind(p.BaseURL) == endpointGroq && p.APIKey != "" {
			return p.APIKey
		}
	}
	return ""
}

// Providers lists every provider, whether or not it is usable.
func (r *Registry) Providers(ctx context.Context) []ProviderSummary {
	all := r.providers(ctx)
	out := make([]ProviderSummary, 0, len(all))
	for _, p := range all {
		out = append(out, ProviderSummary{
			ID:        p.ID,
			Name:      p.Name,
			BaseURL:   p.BaseURL,
			Connected: p.connected(),
			Enabled:   p.Enabled,
			APIKey:    p.APIKey,
		})
	}
	return out
}

// ProviderOrder returns the providers that can actually be used, in the order
// they are offered.
func (r *Registry) ProviderOrder(ctx context.Context) []string {
	out := []string{}
	for _, p := range r.providers(ctx) {
		if p.connected() && p.Enabled {
			out = append(out, p.ID)
		}
	}
	return out
}

// Usable reports whether a provider exists, has credentials and is turned on.
func (r *Registry) Usable(ctx context.Context, providerID string) bool {
	for _, p := range r.providers(ctx) {
		if p.ID == providerID {
			return p.connected() && p.Enabled
		}
	}
	return false
}

// InvalidateCatalog drops this registry's cached model catalog so the next read
// reflects a provider change.
func (r *Registry) InvalidateCatalog() {
	r.catalog.mu.Lock()
	defer r.catalog.mu.Unlock()
	r.catalog.providers = nil
	r.catalog.fetchedAt = time.Time{}
}

// resolve builds the GoAI language model for a provider, honouring the
// provider's stored settings.
func (r *Registry) resolve(ctx context.Context, providerID, model string) (provider.LanguageModel, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("ai: empty model id for provider %q", providerID)
	}

	for _, p := range r.providers(ctx) {
		if p.ID != providerID {
			continue
		}
		if !p.connected() {
			return nil, fmt.Errorf("ai: %s has no base URL", p.ID)
		}
		if !p.Enabled {
			return nil, fmt.Errorf("ai: %s is turned off", p.ID)
		}

		switch endpointKind(p.BaseURL) {
		case endpointGemini:
			// Gemini speaks its own protocol rather than the OpenAI one.
			return google.Chat(model, google.WithAPIKey(p.APIKey)), nil
		case endpointGroq:
			// Groq goes through the generic OpenAI-compatible model rather than
			// groq.Chat because that one always replays the model's reasoning
			// back as `reasoning_content`, and Groq rejects that field outright
			// for the gpt-oss models. With tools enabled the second step of the
			// loop would fail. Everything else stays the same: Groq's endpoint,
			// and max_completion_tokens, which Groq prefers to max_tokens.
			return compat.Chat(model,
				compat.WithProviderID(p.ID),
				compat.WithBaseURL(groqBaseURL),
				compat.WithAPIKey(p.APIKey),
				compat.WithMaxCompletionTokens(true),
			), nil
		}

		opts := []compat.Option{
			compat.WithBaseURL(p.BaseURL),
			compat.WithProviderID(p.ID),
		}
		// A provider may legitimately take no key, in which case the
		// Authorization header is left off entirely.
		if p.APIKey != "" {
			opts = append(opts, compat.WithAPIKey(p.APIKey))
		}
		return compat.Chat(model, opts...), nil
	}

	return nil, fmt.Errorf("%w: %q", ErrUnsupportedProvider, providerID)
}

// endpoint tells the registry which adapter a provider's base URL needs.
type endpoint int

const (
	endpointCompat endpoint = iota
	endpointGemini
	endpointGroq
)

// endpointKind recognises the two vendors that do not speak the plain OpenAI
// protocol. It matches on the endpoint rather than on a name, because a provider
// exists only because the person added it: where it points decides how to talk
// to it.
func endpointKind(raw string) endpoint {
	u, err := url.Parse(NormaliseBaseURL(raw))
	if err != nil || u.Host == "" {
		return endpointCompat
	}
	switch {
	case strings.EqualFold(u.Host, "generativelanguage.googleapis.com"):
		// Gemini's own API is not OpenAI-compatible. Its /openai path is a
		// translation layer, and pointing there is a request for that layer.
		if strings.Contains(strings.ToLower(u.Path), "/openai") {
			return endpointCompat
		}
		return endpointGemini
	case strings.EqualFold(u.Host, "api.groq.com"):
		return endpointGroq
	}
	return endpointCompat
}

// Usage reports token consumption for one completion.
type Usage struct {
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
	Model        string `json:"model"`
}

// Reply is the outcome of a completion. Reasoning is the model's thinking text
// when the provider exposes it, and ThinkingMs is how long it spent producing
// it. Trace is the ordered record of the turn, and Replay is the provider
// transcript a later turn needs to keep the model's tool context.
type Reply struct {
	Text       string
	Reasoning  string
	ThinkingMs int64
	Usage      Usage
	Trace      Trace
	Replay     json.RawMessage
	// Steps is how many model turns the harness loop used, and StepsExhausted
	// reports that it stopped because MaxSteps ran out mid-task.
	Steps          int
	StepsExhausted bool
}

// StreamRequest is one turn of generation: which model to ask, the conversation
// so far, and the tools the model may call. With no tools it is a plain
// completion.
type StreamRequest struct {
	ProviderID string
	Model      string
	System     string
	Messages   []Message
	Tools      []goai.Tool
	// ToolOrigins maps a tool name to the connector that offers it, so the
	// trace can say which MCP server a call belongs to. Nil without connectors.
	ToolOrigins map[string]string
	// MaxSteps bounds the harness loop. Zero or less means a single step.
	MaxSteps int
}

// Handlers receives incremental output from a streaming completion. All
// callbacks are optional; returning an error aborts the stream. OnTrace is
// called with the whole trace whenever it changes, so the caller can forward a
// snapshot and keep the last one.
type Handlers struct {
	OnText      func(delta string) error
	OnReasoning func(delta string) error
	OnTrace     func(trace Trace) error
}

// Stream runs one turn, calling the handlers for every text and reasoning delta
// as it arrives and for each change to the trace. A non-nil error may still
// come with partial text and a partial trace, which callers are expected to
// persist so a cancelled or failed turn is not silently lost.
func (r *Registry) Stream(ctx context.Context, req StreamRequest, handlers Handlers) (Reply, error) {
	lm, err := r.resolve(ctx, req.ProviderID, req.Model)
	if err != nil {
		return Reply{}, err
	}

	opts := make([]goai.Option, 0, 8)
	if strings.TrimSpace(req.System) != "" {
		opts = append(opts, goai.WithSystem(req.System))
	}
	opts = append(opts, goai.WithMessages(toGoAIMessages(req.Messages)...))

	// Tool results are reported by the harness hook, which runs on the loop's
	// goroutine while this one is blocked reading chunks. They are queued here
	// and folded into the trace between chunks, so every trace mutation and
	// every write to the client happens on one goroutine.
	var (
		resultsMu sync.Mutex
		results   []goai.ToolCallInfo
	)
	if len(req.Tools) > 0 {
		steps := req.MaxSteps
		if steps < 1 {
			steps = 1
		}
		opts = append(opts,
			// Tool calls run in parallel, capped at maxParallelTools at once.
			// The trace still lands in the order the model asked for, because
			// results are matched by call ID rather than by finish order (see
			// traceBuilder.toolResults).
			goai.WithTools(limitedTools(req.Tools, maxParallelTools)...),
			goai.WithMaxSteps(steps),
			goai.WithOnToolCall(func(info goai.ToolCallInfo) {
				resultsMu.Lock()
				results = append(results, info)
				resultsMu.Unlock()
			}),
		)
	}

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
		builder     = traceBuilder{origins: req.ToolOrigins}
	)

	// emit publishes the trace so far. It is called on every change the user
	// can see: a tool call, a finished step, a tool's result.
	emit := func() error {
		if handlers.OnTrace == nil {
			return nil
		}
		return handlers.OnTrace(builder.trace.Clone())
	}

	// drain folds any tool outcome the hook has queued into the trace, and
	// reports whether anything changed.
	drain := func() bool {
		resultsMu.Lock()
		pending := results
		results = nil
		resultsMu.Unlock()
		if len(pending) == 0 {
			return false
		}
		outcomes := make([]ToolOutcome, 0, len(pending))
		for _, info := range pending {
			outcome := ToolOutcome{
				CallID:     info.ToolCallID,
				Output:     info.Output,
				DurationMs: info.Duration.Milliseconds(),
			}
			switch {
			case info.Error != nil:
				outcome.Output = ""
				outcome.Error = info.Error.Error()
			case info.Skipped:
				outcome.Output = ""
				outcome.Status = StatusSkipped
			}
			outcomes = append(outcomes, outcome)
		}
		builder.toolResults(outcomes)
		return true
	}

	// current assembles a reply from whatever has arrived so far, so partial
	// output survives an abort or a provider failure.
	current := func() Reply {
		out := Reply{Text: text.String(), Reasoning: reasoning.String(), Trace: builder.trace.Clone()}
		if out.Reasoning != "" && !thinkingEnd.IsZero() {
			out.ThinkingMs = thinkingEnd.Sub(startedAt).Milliseconds()
		}
		return out
	}

	for chunk := range stream.Stream() {
		// A tool that has just run reports here, before whatever the model
		// does next.
		if drain() {
			if err := emit(); err != nil {
				return current(), err
			}
		}
		switch chunk.Type {
		case provider.ChunkText:
			if chunk.Text == "" {
				continue
			}
			text.WriteString(chunk.Text)
			builder.append(PartText, chunk.Text)
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
			builder.append(PartReasoning, chunk.Text)
			if handlers.OnReasoning != nil {
				if err := handlers.OnReasoning(chunk.Text); err != nil {
					return current(), err
				}
			}
		case provider.ChunkToolCallStreamStart:
			builder.toolStart(chunk.ToolCallID, chunk.ToolName)
			if err := emit(); err != nil {
				return current(), err
			}
		case provider.ChunkToolCall:
			builder.toolInput(chunk.ToolCallID, chunk.ToolName, chunk.ToolInput)
			if err := emit(); err != nil {
				return current(), err
			}
		case provider.ChunkStepFinish:
			// goai tags its own step boundaries "goai" and hands over the
			// step's ordered parts. Tool results do not come through here: the
			// chunk that carries them is consumed by the stream's own reducer,
			// so the harness hook is what reports them (see drain above).
			if stepSource(chunk.Metadata) == "goai" {
				builder.fromContent(chunk.Content)
				builder.stepFinish(string(chunk.FinishReason))
				if err := emit(); err != nil {
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

	// The loop's own record is authoritative, so anything the live stream did
	// not report is folded in before the last snapshot goes out.
	result := stream.Result()
	if result != nil {
		drain()
		builder.applyFinal(result.Steps)
	}
	if err := emit(); err != nil {
		return current(), err
	}

	reply := current()
	if result != nil {
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
		reply.Steps = len(result.Steps)
		reply.StepsExhausted = result.StepsExhausted
		// The replay transcript is only needed when tools were in play: without
		// them the plain role/content history says the same thing.
		if len(req.Tools) > 0 && len(result.ResponseMessages) > 0 {
			if data, err := json.Marshal(result.ResponseMessages); err == nil {
				reply.Replay = data
			}
		}
	}
	return reply, nil
}

// limitedTools wraps each tool so that no more than max of them run at once.
// goai fires a step's tool calls in parallel with no bound of its own, so the
// cap lives here, in one semaphore shared by every tool in the turn.
//
// The tools are copied before wrapping: their definitions are shared between
// turns, and a wrapped Execute must not leak into the next one.
func limitedTools(tools []goai.Tool, max int) []goai.Tool {
	if max < 1 || len(tools) == 0 {
		return tools
	}

	slots := make(chan struct{}, max)
	limited := make([]goai.Tool, len(tools))
	for i, tool := range tools {
		execute := tool.Execute
		if execute == nil {
			limited[i] = tool
			continue
		}
		tool.Execute = func(ctx context.Context, input json.RawMessage) (string, error) {
			slots <- struct{}{}
			defer func() { <-slots }()
			return execute(ctx, input)
		}
		limited[i] = tool
	}
	return limited
}

func toGoAIMessages(messages []Message) []provider.Message {
	out := make([]provider.Message, 0, len(messages))
	for _, m := range messages {
		// A turn that ran tools replays its provider transcript verbatim: tool
		// calls and their results have to survive, and the plain text cannot
		// carry them.
		if len(m.Replay) > 0 {
			var replayed []provider.Message
			if err := json.Unmarshal(m.Replay, &replayed); err == nil && len(replayed) > 0 {
				out = append(out, replayed...)
				continue
			}
		}
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
