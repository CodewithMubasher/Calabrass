// Package hostfile reads the announcement the Calabrass host leaves in the
// data folder: where it listens, and the secret that lets this terminal sign in
// as the machine's owner. The folder is the same one the desktop app and the
// host use, so all of them find the same host.
package hostfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Info is the part of host.json a terminal needs.
type Info struct {
	Addr   string `json:"addr"`
	PID    int    `json:"pid"`
	Owner  string `json:"owner"`
	Secret string `json:"secret"`
}

// DataDir is Calabrass's per-user data folder. It must agree with the
// backend's config.DataDir, which decides where the host writes host.json.
func DataDir() string {
	if v := strings.TrimSpace(os.Getenv("CALABRASS_DATA_DIR")); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		if dir := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); dir != "" {
			return filepath.Join(dir, "Calabrass")
		}
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "Calabrass")
	}
	return "Calabrass"
}

// Read returns the host's announcement, or an error when there is none.
func Read() (Info, error) {
	raw, err := os.ReadFile(filepath.Join(DataDir(), "host.json"))
	if err != nil {
		return Info{}, err
	}
	var info Info
	if err := json.Unmarshal(raw, &info); err != nil {
		return Info{}, err
	}
	return info, nil
}
