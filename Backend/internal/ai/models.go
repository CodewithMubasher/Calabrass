package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Roles a stored message can have.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

const (
	modelsCacheTTL = 10 * time.Minute
	modelsTimeout  = 15 * time.Second
)

// ModelInfo is one selectable model.
type ModelInfo struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Provider string `json:"provider"`
}

// ProviderInfo groups the models of a single provider.
type ProviderInfo struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	DefaultModel string      `json:"defaultModel"`
	Models       []ModelInfo `json:"models"`
}

type catalog struct {
	mu        sync.Mutex
	providers []ProviderInfo
	fetchedAt time.Time
}

var (
	httpClient = &http.Client{Timeout: modelsTimeout}
)

// Models returns the live model catalog, using a short-lived cache unless
// force is set. Providers that are turned off or have no endpoint are skipped.
// When no live lookup succeeds, the endpoints known to serve a fixed set of
// models fall back to that set, so a network blip does not empty the picker. An
// account with no providers gets an empty catalog, which is the truth.
func (r *Registry) Models(ctx context.Context, force bool) []ProviderInfo {
	r.catalog.mu.Lock()
	defer r.catalog.mu.Unlock()

	if !force && len(r.catalog.providers) > 0 && time.Since(r.catalog.fetchedAt) < modelsCacheTTL {
		return r.catalog.providers
	}

	providers := []ProviderInfo{}

	for _, p := range r.providers(ctx) {
		if !p.connected() || !p.Enabled || p.fetch == nil {
			continue
		}
		models, err := p.fetch(ctx, p.defaults)
		if err != nil || len(models) == 0 {
			continue
		}
		providers = append(providers, ProviderInfo{
			ID:           p.ID,
			Name:         p.Name,
			DefaultModel: pickDefault(models),
			Models:       models,
		})
	}

	if len(providers) == 0 {
		providers = fallbackCatalog(r, ctx)
	}

	r.catalog.providers = providers
	r.catalog.fetchedAt = time.Now()
	return providers
}

// NormaliseBaseURL trims trailing slashes. GoAI appends "/chat/completions" to
// whatever base URL it is given.
func NormaliseBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

// groqBaseURL is Groq's OpenAI-compatible endpoint.
const groqBaseURL = "https://api.groq.com/openai/v1"

// geminiBaseURL is the Google Generative Language API. It speaks its own
// protocol rather than the OpenAI one, which is why it gets its own adapter.
const geminiBaseURL = "https://generativelanguage.googleapis.com"

// The preferred model order for the vendors whose API is not plain
// OpenAI-compatible. The first id the live catalog actually offers wins, so the
// picker opens on a sensible model without hard-coding the only choice. These
// are also what a fallback shows when the live listing cannot be fetched.
var (
	geminiPreferredModels = []string{
		"gemini-3.8-flash",
		"gemini-flash-latest",
		"gemini-3.5-flash",
		"gemini-2.5-flash",
		"gemini-2.5-pro",
	}
	groqPreferredModels = []string{
		"openai/gpt-oss-120b",
		"qwen/qwen3.8-27b",
		"llama-3.3-70b-versatile",
		"llama-3.1-8b-instant",
	}
	cloudflarePreferredModels = []string{
		"@cf/meta/llama-3.1-8b-instruct",
		"@cf/meta/llama-3.2-11b-vision-instruct",
		"@cf/meta/llama-3.3-70b-instruct-fp8-fast",
		"@cf/mistral/mistral-7b-instruct-v0.2",
	}
)

// DefaultModel returns the default model for a provider, or an empty string.
func (r *Registry) DefaultModel(ctx context.Context, providerID string) string {
	for _, p := range r.Models(ctx, false) {
		if p.ID == providerID {
			return p.DefaultModel
		}
	}
	return ""
}

// ValidModel reports whether providerID/modelID is in the live catalog.
func (r *Registry) ValidModel(ctx context.Context, providerID, modelID string) bool {
	for _, p := range r.Models(ctx, false) {
		if p.ID != providerID {
			continue
		}
		for _, m := range p.Models {
			if m.ID == modelID {
				return true
			}
		}
	}
	return false
}

type geminiModel struct {
	Name                       string   `json:"name"`
	DisplayName                string   `json:"displayName"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}

func fetchGoogleModels(ctx context.Context, providerID, apiKey string, defaults []string) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		geminiBaseURL+"/v1beta/models?pageSize=200", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-goog-api-key", apiKey)

	var payload struct {
		Models []geminiModel `json:"models"`
	}
	if err := doJSON(req, &payload); err != nil {
		return nil, err
	}

	models := []ModelInfo{}
	for _, m := range payload.Models {
		if !hasGenerateContent(m.SupportedGenerationMethods) {
			continue
		}
		id := strings.TrimPrefix(m.Name, "models/")
		if isNonChatModel(id) {
			continue
		}
		label := strings.TrimSpace(m.DisplayName)
		if label == "" {
			label = Prettify(id)
		}
		models = append(models, ModelInfo{ID: id, Label: label, Provider: providerID})
	}
	sortModels(models, defaults)
	return models, nil
}

func fetchGroqModels(ctx context.Context, providerID, apiKey string, defaults []string) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, groqBaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := doJSON(req, &payload); err != nil {
		return nil, err
	}

	models := []ModelInfo{}
	for _, m := range payload.Data {
		if m.ID == "" || isNonChatModel(m.ID) {
			continue
		}
		models = append(models, ModelInfo{ID: m.ID, Label: Prettify(m.ID), Provider: providerID})
	}
	sortModels(models, defaults)
	return models, nil
}

func doJSON(req *http.Request, target any) error {
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("ai: model list request failed with status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func hasGenerateContent(methods []string) bool {
	for _, m := range methods {
		if m == "generateContent" {
			return true
		}
	}
	return false
}

// isNonChatModel filters out audio, image, embedding and safety models that
// show up in provider listings but cannot hold a conversation.
func isNonChatModel(id string) bool {
	lower := strings.ToLower(id)
	for _, token := range []string{
		"embedding", "embed", "whisper", "tts", "aqa", "orpheus",
		"prompt-guard", "image", "veo", "native-audio", "learnlm",
		"gemma-3n", "imagen",
	} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

// sortModels puts the preferred default models first, then sorts the rest
// alphabetically by id.
func sortModels(models []ModelInfo, defaults []string) {
	rank := func(id string) int {
		for i, preferred := range defaults {
			if preferred == id {
				return i
			}
		}
		return len(defaults) + 100
	}
	sort.SliceStable(models, func(i, j int) bool {
		ri, rj := rank(models[i].ID), rank(models[j].ID)
		if ri != rj {
			return ri < rj
		}
		return models[i].ID < models[j].ID
	})
}

// pickDefault returns the highest-ranked model, which is simply the first one
// after sortModels has run.
func pickDefault(models []ModelInfo) string {
	if len(models) == 0 {
		return ""
	}
	return models[0].ID
}

// fallbackCatalog lists the models a recognised endpoint is known to serve, used
// when its live listing cannot be fetched. A plain OpenAI-compatible provider is
// left out: its model ids cannot be guessed.
func fallbackCatalog(r *Registry, ctx context.Context) []ProviderInfo {
	providers := []ProviderInfo{}

	for _, p := range r.providers(ctx) {
		if !p.connected() || !p.Enabled || len(p.defaults) == 0 {
			continue
		}
		models := make([]ModelInfo, 0, len(p.defaults))
		for _, modelID := range p.defaults {
			models = append(models, ModelInfo{ID: modelID, Label: Prettify(modelID), Provider: p.ID})
		}
		if len(models) == 0 {
			continue
		}
		providers = append(providers, ProviderInfo{
			ID:           p.ID,
			Name:         p.Name,
			DefaultModel: models[0].ID,
			Models:       models,
		})
	}
	return providers
}

// fetchCompatModels lists the chat models of a custom, OpenAI-compatible
// provider by calling its /models endpoint.
func fetchCompatModels(
	ctx context.Context,
	providerID, baseURL, apiKey string,
	defaults []string,
) ([]ModelInfo, error) {
	if baseURL == "" {
		return nil, errors.New("ai: base URL is not set")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := doJSON(req, &payload); err != nil {
		return nil, err
	}

	models := []ModelInfo{}
	for _, m := range payload.Data {
		if m.ID == "" || isNonChatModel(m.ID) {
			continue
		}
		models = append(models, ModelInfo{ID: m.ID, Label: Prettify(m.ID), Provider: providerID})
	}
	sortModels(models, defaults)
	return models, nil
}

// fetchCloudflareModels lists the chat models of Cloudflare Workers AI.
// The baseURL should already have the account_id substituted.
func fetchCloudflareModels(
	ctx context.Context,
	providerID, baseURL, apiKey string,
	defaults []string,
) ([]ModelInfo, error) {
	if baseURL == "" {
		return nil, errors.New("ai: base URL is not set")
	}

	// Cloudflare Workers AI models endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	var payload struct {
		Result []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"result"`
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
		Messages []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"messages"`
	}
	if err := doJSON(req, &payload); err != nil {
		return nil, err
	}

	if !payload.Success && len(payload.Errors) > 0 {
		return nil, fmt.Errorf("cloudflare: %s", payload.Errors[0].Message)
	}

	models := []ModelInfo{}
	for _, m := range payload.Result {
		if m.Name == "" || isNonChatModel(m.Name) {
			continue
		}
		label := strings.TrimSpace(m.Description)
		if label == "" {
			label = Prettify(m.Name)
		}
		models = append(models, ModelInfo{ID: m.Name, Label: label, Provider: providerID})
	}
	sortModels(models, defaults)
	return models, nil
}

// Prettify turns a model id into a human label: "gemini-2.5-flash" becomes
// "Gemini 2.5 Flash", "openai/gpt-oss-120b" becomes "GPT OSS 120B".
func Prettify(id string) string {
	base := id
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}

	parts := strings.FieldsFunc(base, func(r rune) bool {
		return r == '-' || r == '_'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		switch strings.ToLower(part) {
		case "gpt", "oss", "ai", "glm", "qwen", "moe", "rag":
			parts[i] = strings.ToUpper(part)
			continue
		}
		if part[0] >= '0' && part[0] <= '9' {
			// Size suffixes such as "120b" or "27b" read better uppercased.
			last := part[len(part)-1]
			if last >= 'a' && last <= 'z' {
				parts[i] = part[:len(part)-1] + strings.ToUpper(string(last))
			}
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}
