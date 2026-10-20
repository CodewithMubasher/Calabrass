package providers

import (
	"strings"
)

// Preset is a pre-configured AI provider template.
// These are seeded at signup so users just need to add their API key.
type Preset struct {
	// ID is the unique identifier (used as provider ID)
	ID string
	// Display name shown in the UI
	Name string
	// Base URL for the provider's OpenAI-compatible API
	BaseURL string
	// Default model to use if none specified
	DefaultModel string
	// Icon key for the frontend (matches PROVIDER_ICONS keys)
	Icon string
	// Whether this provider needs special handling
	Special bool
}

// ProviderPresets are the pre-configured AI providers.
// They are seeded at signup with empty API keys and disabled.
var ProviderPresets = []Preset{
	{
		ID:           "google",
		Name:         "Google Gemini",
		BaseURL:      "https://generativelanguage.googleapis.com/v1beta/openai/",
		DefaultModel: "gemini-3.5-flash",
		Icon:         "google",
		Special:      true, // Uses native Gemini API, not OpenAI-compatible
	},
	{
		ID:           "groq",
		Name:         "Groq",
		BaseURL:      "https://api.groq.com/openai/v1",
		DefaultModel: "llama-3.3-70b-versatile",
		Icon:         "groq",
		Special:      true, // Needs max_completion_tokens handling
	},
	{
		ID:           "openai",
		Name:         "OpenAI",
		BaseURL:      "https://api.openai.com/v1",
		DefaultModel: "gpt-4o",
		Icon:         "openai",
		Special:      false,
	},
	{
		ID:           "openrouter",
		Name:         "OpenRouter",
		BaseURL:      "https://openrouter.ai/api/v1",
		DefaultModel: "anthropic/claude-3.5-sonnet",
		Icon:         "openrouter",
		Special:      false,
	},
	{
		ID:           "mistral",
		Name:         "Mistral",
		BaseURL:      "https://api.mistral.ai/v1",
		DefaultModel: "mistral-large-latest",
		Icon:         "mistral",
		Special:      false,
	},
	{
		ID:           "deepseek",
		Name:         "DeepSeek",
		BaseURL:      "https://api.deepseek.com/v1",
		DefaultModel: "deepseek-chat",
		Icon:         "deepseek",
		Special:      false,
	},
	{
		ID:           "nvidia",
		Name:         "NVIDIA NIM",
		BaseURL:      "https://integrate.api.nvidia.com/v1",
		DefaultModel: "nvidia/nemotron-3-ultra",
		Icon:         "nvidia",
		Special:      false,
	},
	{
		ID:           "cloudflare",
		Name:         "Cloudflare Workers AI",
		BaseURL:      "https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1",
		DefaultModel: "@cf/meta/llama-3.1-8b-instruct",
		Icon:         "cloudflare",
		Special:      true, // Needs account_id substitution (API key format: account_id:api_token)
	},
	{
		ID:           "ollama",
		Name:         "Ollama (Local)",
		BaseURL:      "http://localhost:11434/v1",
		DefaultModel: "llama3.1",
		Icon:         "ollama",
		Special:      false,
	},
	{
		ID:           "together",
		Name:         "Together AI",
		BaseURL:      "https://api.together.xyz/v1",
		DefaultModel: "meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo",
		Icon:         "together",
		Special:      false,
	},
	{
		ID:           "cohere",
		Name:         "Cohere",
		BaseURL:      "https://api.cohere.ai/v1",
		DefaultModel: "command-r-plus",
		Icon:         "cohere",
		Special:      false,
	},
	{
		ID:           "perplexity",
		Name:         "Perplexity",
		BaseURL:      "https://api.perplexity.ai",
		DefaultModel: "llama-3.1-sonar-large-128k-online",
		Icon:         "perplexity",
		Special:      false,
	},
}

// GetPreset returns a preset by ID, or nil if not found.
func GetPreset(id string) *Preset {
	for _, p := range ProviderPresets {
		if strings.EqualFold(p.ID, id) {
			return &p
		}
	}
	return nil
}

// PresetIDs returns all preset IDs in order.
func PresetIDs() []string {
	ids := make([]string, len(ProviderPresets))
	for i, p := range ProviderPresets {
		ids[i] = p.ID
	}
	return ids
}

// IsPresetID checks if an ID is a known preset.
func IsPresetID(id string) bool {
	return GetPreset(id) != nil
}