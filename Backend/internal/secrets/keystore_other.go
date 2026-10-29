//go:build !windows && !darwin

package secrets

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// platformKeyStore prefers the Secret Service when secret-tool is installed and
// a keyring is unlocked; otherwise it falls back to an owner-only key file.
func platformKeyStore(dataDir string) KeyStore {
	fallback := fileKeyStore{path: filepath.Join(dataDir, "calabrass.key")}
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return fallback
	}
	ss := secretServiceStore{}
	// Probe: a lookup that fails for any reason other than "not found" means no
	// usable keyring (headless server, no D-Bus session).
	if _, err := ss.Get(); err != nil && !errors.Is(err, ErrNoKey) {
		return fallback
	}
	return ss
}

type secretServiceStore struct{}

func (secretServiceStore) Name() string { return "Secret Service (system keyring)" }

func (secretServiceStore) Get() ([]byte, error) {
	cmd := exec.Command("secret-tool", "lookup", "service", "Calabrass", "account", "master-key")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		if err != nil {
			// secret-tool exits 1 with no output when nothing matches.
			if out.Len() == 0 && errb.Len() == 0 {
				return nil, ErrNoKey
			}
			return nil, errors.New(strings.TrimSpace(errb.String()))
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		return nil, errors.New("the keyring did not answer")
	}
	key, err := hex.DecodeString(strings.TrimSpace(out.String()))
	if err != nil || len(key) == 0 {
		return nil, ErrNoKey
	}
	return key, nil
}

func (secretServiceStore) Put(key []byte) error {
	// The key goes in on stdin, never on the command line.
	cmd := exec.Command("secret-tool", "store", "--label=Calabrass master key",
		"service", "Calabrass", "account", "master-key")
	cmd.Stdin = strings.NewReader(hex.EncodeToString(key))
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.New(strings.TrimSpace(string(out)))
	}
	return nil
}
