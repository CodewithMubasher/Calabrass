package config

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/CodewithMubasher/Calabrass/backend/internal/netguard"
)

const defaultSystemPrompt = "You are Calabrass, a helpful AI assistant. Answer clearly and use Markdown when it helps."

// appName is the folder this program keeps user data in.
const appName = "Calabrass"

// DataDir is the one folder this program keeps user data in: the database, the
// workspaces the agent works in, the log, and the .env a user's keys are read
// from. It belongs to the user, not to the project, so a checkout, a clean or an
// update can move or delete the source tree without taking someone's chats and
// files with it.
func DataDir() string {
	// An explicit folder wins everywhere, which lets tests, portable installs and
	// a second profile keep their own host apart from the everyday one.
	if v := strings.TrimSpace(os.Getenv("CALABRASS_DATA_DIR")); v != "" {
		return v
	}
	// %LOCALAPPDATA% is where a Windows program's own data belongs. It is not
	// %APPDATA%: that one roams, and a database of workspaces should stay put.
	if runtime.GOOS == "windows" {
		if dir := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); dir != "" {
			return filepath.Join(dir, appName)
		}
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, appName)
	}
	return appName
}

// Config holds everything the server needs to boot.
type Config struct {
	Addr         string
	DatabasePath string
	// MistralKey is the credential for the realtime transcription endpoint that
	// Cal Live listens with. Empty means "fall back to Groq Whisper".
	MistralKey   string
	SystemPrompt string
	// VoiceProvider and VoiceModel let the voice agent think with a faster model
	// than the one that does its work. Empty means "use the chat model".
	VoiceProvider string
	VoiceModel    string
	// WorkspacesDir is the folder that holds every workspace the agent may read
	// and change. It lives in the data folder with everything else.
	WorkspacesDir string
	// QuotasPath points at the free-tier limits file. Empty means "use the one
	// in the project root". The desktop shell sets it to the copy it carries.
	QuotasPath string
	// PairingCode is the shared secret that lets another device use this host.
	// Empty (the default) means nothing but this machine may connect, and the
	// host will refuse to listen on anything but loopback.
	PairingCode string
}

// ValidateBind refuses a listen address that would put the agent, which can run
// commands, in reach of the network without a pairing code.
func (c Config) ValidateBind() error {
	return netguard.ValidateBind(c.Addr, c.PairingCode)
}

// Load reads configuration from the environment, after hydrating it from the
// .env in the data folder when present. Real environment variables always win.
func Load() Config {
	return LoadFrom(filepath.Join(DataDir(), ".env"))
}

// LoadFrom is Load with an explicit .env path.
func LoadFrom(envPath string) Config {
	loadDotEnv(envPath)

	dataDir := DataDir()

	return Config{
		Addr:          envOr("CALABRASS_ADDR", "127.0.0.1:8787"),
		DatabasePath:  inDataDir(dataDir, envOr("CALABRASS_DB", "calabrass.db")),
		MistralKey:    os.Getenv("MISTRAL_API_KEY"),
		VoiceProvider: os.Getenv("CALABRASS_VOICE_PROVIDER"),
		VoiceModel:    os.Getenv("CALABRASS_VOICE_MODEL"),
		SystemPrompt:  envOr("CALABRASS_SYSTEM_PROMPT", defaultSystemPrompt),
		// CALABRASS_WORKSPACE is the older name for it.
		WorkspacesDir: inDataDir(dataDir, firstNonEmpty(
			os.Getenv("CALABRASS_WORKSPACES_DIR"),
			os.Getenv("CALABRASS_WORKSPACE"),
			"workspace",
		)),
		QuotasPath:  strings.TrimSpace(os.Getenv("CALABRASS_QUOTAS")),
		PairingCode: strings.TrimSpace(os.Getenv("CALABRASS_PAIRING_CODE")),
	}
}

// inDataDir resolves a configured path against the data folder. A relative value
// can then only mean "inside the data folder", never "inside whatever directory
// the process happens to have been started from" — which is how a stale .env
// used to put a user's database back inside the checkout.
func inDataDir(dir, path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(dir, path)
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
