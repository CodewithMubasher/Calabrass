package config

import (
	"bufio"
	"os"
	"strings"
)

const defaultSystemPrompt = "You are Calabrass, a helpful AI assistant. Answer clearly and use Markdown when it helps."

// Config holds everything the server needs to boot.
type Config struct {
	Addr         string
	DatabasePath string
	GeminiKey    string
	GroqKey      string
	// MistralKey is the credential for the realtime transcription endpoint that
	// Cal Live listens with. Empty means "fall back to Groq Whisper".
	MistralKey   string
	SystemPrompt string
	// VoiceProvider and VoiceModel let the voice agent think with a faster model
	// than the one that does its work. Empty means "use the chat model".
	VoiceProvider string
	VoiceModel    string
	// WorkspacesDir is the folder that holds every workspace the agent may
	// read and change — <project>/workspace by default. Empty means "discover
	// the project from the working directory".
	WorkspacesDir string
	// ProviderPriority is the order in which providers are offered and picked
	// as a fallback, e.g. ["groq", "google"].
	ProviderPriority []string
	// QuotasPath points at the free-tier limits file. Empty means "use the one
	// in the project root". The desktop shell sets it to the copy it carries.
	QuotasPath string
}

// Load reads configuration from the environment, after hydrating it from a
// local .env file when present. Real environment variables always win.
func Load() Config {
	return LoadFrom(".env")
}

// LoadFrom is Load with an explicit .env path, which the desktop shell uses to
// read Backend/.env without changing the process working directory.
func LoadFrom(envPath string) Config {
	loadDotEnv(envPath)

	return Config{
		Addr:          envOr("CALABRASS_ADDR", "127.0.0.1:8787"),
		DatabasePath:  envOr("CALABRASS_DB", "data/calabrass.db"),
		GeminiKey:     firstNonEmpty(os.Getenv("GOOGLE_GENERATIVE_AI_API_KEY"), os.Getenv("GEMINI_API_KEY"), os.Getenv("GOOGLE_API_KEY")),
		GroqKey:       os.Getenv("GROQ_API_KEY"),
		MistralKey:    os.Getenv("MISTRAL_API_KEY"),
		VoiceProvider: os.Getenv("CALABRASS_VOICE_PROVIDER"),
		VoiceModel:    os.Getenv("CALABRASS_VOICE_MODEL"),
		SystemPrompt:  envOr("CALABRASS_SYSTEM_PROMPT", defaultSystemPrompt),
		// Empty means the workspaces folder is discovered from the working
		// directory. CALABRASS_WORKSPACE is the older name for it.
		WorkspacesDir: firstNonEmpty(os.Getenv("CALABRASS_WORKSPACES_DIR"), os.Getenv("CALABRASS_WORKSPACE")),
		// Groq first: it is the provider that is verified working by default.
		ProviderPriority: splitList(envOr("CALABRASS_PROVIDER_PRIORITY", "groq,google")),
		QuotasPath:       strings.TrimSpace(os.Getenv("CALABRASS_QUOTAS")),
	}
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// loadDotEnv is a deliberately tiny .env reader: KEY=VALUE per line, optional
// "export" prefix, # comments, surrounding quotes stripped. It never overrides
// a variable that is already set in the process environment.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)

		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, value)
	}
}
