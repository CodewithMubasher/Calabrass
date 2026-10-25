// Package settings keeps the TUI's small amount of local state: which backend
// to talk to, the session token, and the last model used.
package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// EnvDir overrides the folder the settings file lives in (used by tests).
const EnvDir = "CALABRASS_TUI_CONFIG_DIR"

// Settings is the content of tui.json.
type Settings struct {
	Addr  string `json:"addr,omitempty"`
	Token string `json:"token,omitempty"`

	// Email and Password are the credentials of the local account the TUI
	// created for itself, when no account was supplied through the
	// environment. They never leave this machine.
	Email    string `json:"email,omitempty"`
	Password string `json:"password,omitempty"`

	// The last model that was picked.
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// Path is where tui.json lives.
func Path() (string, error) {
	dir := os.Getenv(EnvDir)
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "Calabrass")
	}
	return filepath.Join(dir, "tui.json"), nil
}

// Load reads the settings. A missing file is not an error.
func Load() (Settings, error) {
	var s Settings
	path, err := Path()
	if err != nil {
		return s, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}, err
	}
	return s, nil
}

// Save writes the settings, readable by this user only.
func (s Settings) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
