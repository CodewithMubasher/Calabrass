//go:build darwin

package secrets

import (
	"encoding/hex"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

// platformKeyStore keeps the master key in the login Keychain.
func platformKeyStore(dataDir string) KeyStore {
	if _, err := exec.LookPath("security"); err != nil {
		return fileKeyStore{path: filepath.Join(dataDir, "calabrass.key")}
	}
	return keychainStore{}
}

type keychainStore struct{}

func (keychainStore) Name() string { return "macOS Keychain" }

func (keychainStore) Get() ([]byte, error) {
	out, err := exec.Command("security", "find-generic-password",
		"-s", "Calabrass", "-a", "master-key", "-w").Output()
	if err != nil {
		// Exit status 44 is "item could not be found".
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 44 {
			return nil, ErrNoKey
		}
		return nil, err
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(out)))
	if err != nil || len(key) == 0 {
		return nil, ErrNoKey
	}
	return key, nil
}

func (keychainStore) Put(key []byte) error {
	// `security` takes the secret as an argument, so it is visible in the
	// process list for the instant it runs. That happens once, when the key is
	// created, on the user's own machine.
	out, err := exec.Command("security", "add-generic-password",
		"-s", "Calabrass", "-a", "master-key", "-U", "-w", hex.EncodeToString(key)).CombinedOutput()
	if err != nil {
		return errors.New(strings.TrimSpace(string(out)))
	}
	return nil
}
