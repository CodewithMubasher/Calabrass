package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CodewithMubasher/Calabrass/backend/internal/secrets"
)

func rawKey(t *testing.T, s *Store, id string) string {
	t.Helper()
	var key string
	if err := s.db.QueryRow(`SELECT api_key FROM providers WHERE id = ?`, id).Scan(&key); err != nil {
		t.Fatal(err)
	}
	return key
}

func newSealer(t *testing.T) Sealer {
	t.Helper()
	sealer, err := secrets.NewWith(secrets.FileKeyStore(filepath.Join(t.TempDir(), "k")))
	if err != nil {
		t.Fatal(err)
	}
	return sealer
}

func TestProviderKeysAreCiphertextOnDisk(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	s.SetSealer(newSealer(t))

	const key = "sk-live-1234567890abcdef"
	if err := s.UpsertProvider(ctx, ProviderRecord{ID: "p", Name: "P", BaseURL: "https://x", APIKey: key, Enabled: true}, "u"); err != nil {
		t.Fatal(err)
	}
	if stored := rawKey(t, s, "p"); strings.Contains(stored, "1234567890") || !strings.HasPrefix(stored, "cal1:") {
		t.Fatalf("the key is readable in the database: %q", stored)
	}
	got, err := s.GetProvider(ctx, "p", "u")
	if err != nil || got.APIKey != key {
		t.Fatalf("the key does not read back: %q %v", got.APIKey, err)
	}
	list, _ := s.ListProviders(ctx, "u")
	if len(list) != 1 || list[0].APIKey != key {
		t.Fatalf("the list does not read back: %+v", list)
	}
}

func TestAnOldPlaintextDatabaseIsMigrated(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Written by a build that did not seal anything.
	if err := s.UpsertProvider(ctx, ProviderRecord{ID: "old", Name: "Old", BaseURL: "https://x", APIKey: "sk-plain-key-0000", Enabled: true}, "u"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertProvider(ctx, ProviderRecord{ID: "empty", Name: "Empty", BaseURL: "https://x", Enabled: false}, "u"); err != nil {
		t.Fatal(err)
	}

	s.SetSealer(newSealer(t))
	// Before migrating, the old key still reads, because Open passes plain text through.
	if got, _ := s.GetProvider(ctx, "old", "u"); got.APIKey != "sk-plain-key-0000" {
		t.Fatalf("a plaintext key stopped working: %q", got.APIKey)
	}
	n, err := s.SealLegacySecrets(ctx)
	if err != nil || n != 1 {
		t.Fatalf("migrated %d, %v; want 1", n, err)
	}
	if stored := rawKey(t, s, "old"); strings.Contains(stored, "plain-key") {
		t.Fatalf("still plaintext after migration: %q", stored)
	}
	if got, _ := s.GetProvider(ctx, "old", "u"); got.APIKey != "sk-plain-key-0000" {
		t.Fatalf("the key changed in migration: %q", got.APIKey)
	}
	if rawKey(t, s, "empty") != "" {
		t.Fatal("an empty key must stay empty")
	}
	// Running it again finds nothing to do.
	if n, _ := s.SealLegacySecrets(ctx); n != 0 {
		t.Fatalf("a second migration touched %d rows", n)
	}
}

func TestALostMasterKeyReadsAsNoKeyNotAnError(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	s.SetSealer(newSealer(t))
	_ = s.UpsertProvider(ctx, ProviderRecord{ID: "p", Name: "P", BaseURL: "https://x", APIKey: "sk-secret-secret-1", Enabled: true}, "u")

	s.SetSealer(newSealer(t)) // a different master key
	list, err := s.ListProviders(ctx, "u")
	if err != nil || len(list) != 1 || list[0].APIKey != "" {
		t.Fatalf("a key that cannot be opened must read as empty, not fail: %+v %v", list, err)
	}
}

func TestOwnerUserIsTheFirstAccount(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if _, err := s.OwnerUser(ctx); err != ErrNotFound {
		t.Fatalf("an empty database has no owner, got %v", err)
	}
	_, _ = s.CreateUser(ctx, UserRecord{ID: "first", Name: "A", Email: "a@x.com", PasswordHash: "h", CreatedAt: 100})
	_, _ = s.CreateUser(ctx, UserRecord{ID: "second", Name: "B", Email: "b@x.com", PasswordHash: "h", CreatedAt: 200})
	if owner, _ := s.OwnerUser(ctx); owner.ID != "first" {
		t.Fatalf("the owner is %q, want the first account", owner.ID)
	}
}
