// Package secrets keeps credentials, such as a provider's API key, from sitting
// in the database as readable text.
//
// A value is sealed with AES-256-GCM under one master key, and the master key is
// held by the operating system rather than by Calabrass:
//
//	Windows  DPAPI: the key file can only be opened by the same Windows user
//	macOS    the login Keychain
//	Linux    the Secret Service (GNOME Keyring, KWallet) through secret-tool
//	other    a key file readable only by the owner (weaker; Backend() says so)
//
// The database then holds only sealed text. Copying calabrass.db to another
// machine, a backup, or a sync folder does not copy the keys with it.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// prefix marks a sealed value. A value without it is legacy plaintext, which
// Open returns unchanged so an existing database keeps working until it is
// migrated.
const prefix = "cal1:"

// ErrNoKey means the key store holds no master key yet.
var ErrNoKey = errors.New("secrets: no master key yet")

// KeyStore holds the one master key.
type KeyStore interface {
	// Name says where the key lives, for the About page and the log.
	Name() string
	// Get returns the key, or ErrNoKey when none has been stored.
	Get() ([]byte, error)
	// Put stores the key.
	Put(key []byte) error
}

// Sealer seals and opens values under the master key.
type Sealer struct {
	aead    cipher.AEAD
	backend string
}

// IsSealed reports whether a stored value is sealed text.
func IsSealed(v string) bool { return strings.HasPrefix(v, prefix) }

// New returns a Sealer whose master key lives in the best store this operating
// system offers, creating the key on first use. dataDir is where a fallback key
// file goes.
func New(dataDir string) (*Sealer, error) {
	return NewWith(platformKeyStore(dataDir))
}

// NewWith is New with an explicit key store.
func NewWith(ks KeyStore) (*Sealer, error) {
	key, err := ks.Get()
	if errors.Is(err, ErrNoKey) {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("secrets: make a master key: %w", err)
		}
		if err := ks.Put(key); err != nil {
			return nil, fmt.Errorf("secrets: store the master key in %s: %w", ks.Name(), err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("secrets: read the master key from %s: %w", ks.Name(), err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("secrets: the master key in %s is the wrong size", ks.Name())
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead, backend: ks.Name()}, nil
}

// Backend names where the master key is kept.
func (s *Sealer) Backend() string { return s.backend }

// Seal returns the sealed form of v. An empty value stays empty: there is
// nothing to protect, and it lets "no key set" be told apart from "a key".
func (s *Sealer) Seal(v string) (string, error) {
	if v == "" || IsSealed(v) {
		return v, nil
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("secrets: nonce: %w", err)
	}
	sealed := s.aead.Seal(nonce, nonce, []byte(v), nil)
	return prefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

// Open returns the plain value. Legacy plaintext passes through.
func (s *Sealer) Open(v string) (string, error) {
	if !IsSealed(v) {
		return v, nil
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(v, prefix))
	if err != nil {
		return "", fmt.Errorf("secrets: damaged sealed value: %w", err)
	}
	size := s.aead.NonceSize()
	if len(raw) < size {
		return "", errors.New("secrets: damaged sealed value")
	}
	plain, err := s.aead.Open(nil, raw[:size], raw[size:], nil)
	if err != nil {
		return "", errors.New("secrets: could not open a sealed value; the master key is not the one that sealed it")
	}
	return string(plain), nil
}

// --- the key file fallback ---------------------------------------------------

// fileKeyStore keeps the key in a file only its owner can read. It is the
// fallback where the OS offers nothing better, and it says so by name.
type fileKeyStore struct{ path string }

func (f fileKeyStore) Name() string { return "key file (owner-only)" }

func (f fileKeyStore) Get() ([]byte, error) {
	raw, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoKey
	}
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("the key file is damaged: %w", err)
	}
	return key, nil
}

func (f fileKeyStore) Put(key []byte) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(f.path, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0o600)
}

// FileKeyStore is the owner-only key file, exported for tests and for hosts
// that want it explicitly.
func FileKeyStore(path string) KeyStore { return fileKeyStore{path: path} }
