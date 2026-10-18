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

// Provider identifiers used across the backend and the API.
const (
	ProviderGoogle = "google"
	ProviderGroq   = "groq"

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
// force is set. Providers that are turned off or have no credentials are
// skipped; if every live lookup fails the static fallback list is returned so
// the UI is never empty.
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

// providerSpec describes how to talk to one provider's model listing.
type providerSpec struct {
	id       string
	name     string
	defaults []string
	fetch    func(ctx context.Context, defaults []string) ([]ModelInfo, error)
}

func (r *Registry) providerSpecs() map[string]providerSpec {
	return map[string]providerSpec{
		ProviderGoogle: {
			id:   ProviderGoogle,
			name: "Google Gemini",
			// Preference order: the first id present in the live catalog wins.
			defaults: []string{
				"gemini-3.8-flash",
				"gemini-flash-latest",
				"gemini-3.5-flash",
				"gemini-2.5-flash",
				"gemini-2.5-pro",
			},
			fetch: r.fetchGoogleModels,
		},
		ProviderGroq: {
			id:   ProviderGroq,
			name: "Groq",
			// Not every Groq key exposes llama-3.3-70b-versatile.
			defaults: []string{
				"openai/gpt-oss-120b",
				"qwen/qwen3.8-27b",
				"llama-3.3-70b-versatile",
				"llama-3.1-8b-instant",
			},
			fetch: r.fetchGroqModels,
		},
	}
}

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

func (r *Registry) fetchGoogleModels(ctx context.Context, defaults []string) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://generativelanguage.googleapis.com/v1beta/models?pageSize=200", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-goog-api-key", r.geminiKey)

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
		models = append(models, ModelInfo{ID: id, Label: label, Provider: ProviderGoogle})
	}
	sortModels(models, defaults)
	return models, nil
}

func (r *Registry) fetchGroqModels(ctx context.Context, defaults []string) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, groqBaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.groqKey)

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
		models = append(models, ModelInfo{ID: m.ID, Label: Prettify(m.ID), Provider: ProviderGroq})
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

// fallbackCatalog lists each usable built-in provider's preferred models, used
// when every live lookup fails. Custom providers are left out, since their
// model ids cannot be guessed.
func fallbackCatalog(r *Registry, ctx context.Context) []ProviderInfo {
	providers := []ProviderInfo{}

	for _, p := range r.providers(ctx) {
		if p.Custom || !p.connected() || !p.Enabled || len(p.defaults) == 0 {
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
