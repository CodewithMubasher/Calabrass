package secrets

import (
	"path/filepath"
	"strings"
	"testing"
)

func newTestSealer(t *testing.T) (*Sealer, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "k")
	s, err := NewWith(FileKeyStore(path))
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}

func TestSealOpenRoundTrip(t *testing.T) {
	s, _ := newTestSealer(t)
	sealed, err := s.Seal("sk-live-abcdef123456")
	if err != nil {
		t.Fatal(err)
	}
	if !IsSealed(sealed) || strings.Contains(sealed, "abcdef") {
		t.Fatalf("not sealed: %q", sealed)
	}
	plain, err := s.Open(sealed)
	if err != nil || plain != "sk-live-abcdef123456" {
		t.Fatalf("open = %q, %v", plain, err)
	}
	// Sealing twice gives different text (fresh nonce) and still opens.
	again, _ := s.Seal("sk-live-abcdef123456")
	if again == sealed {
		t.Fatal("two seals of the same value were identical")
	}
}

func TestEmptyAndLegacyValues(t *testing.T) {
	s, _ := newTestSealer(t)
	if v, _ := s.Seal(""); v != "" {
		t.Fatalf("empty must stay empty, got %q", v)
	}
	// An old plaintext row opens as itself, so existing databases keep working.
	if v, err := s.Open("sk-old-plaintext"); err != nil || v != "sk-old-plaintext" {
		t.Fatalf("legacy value: %q, %v", v, err)
	}
	// Sealing an already sealed value must not double-wrap it.
	sealed, _ := s.Seal("k")
	if again, _ := s.Seal(sealed); again != sealed {
		t.Fatal("a sealed value was sealed again")
	}
}

func TestKeyPersistsAcrossRestarts(t *testing.T) {
	s1, path := newTestSealer(t)
	sealed, _ := s1.Seal("secret")
	s2, err := NewWith(FileKeyStore(path))
	if err != nil {
		t.Fatal(err)
	}
	if v, err := s2.Open(sealed); err != nil || v != "secret" {
		t.Fatalf("a restarted sealer must open earlier values: %q, %v", v, err)
	}
}

func TestWrongKeyAndTampering(t *testing.T) {
	a, _ := newTestSealer(t)
	b, _ := newTestSealer(t) // a different key
	sealed, _ := a.Seal("secret")
	if _, err := b.Open(sealed); err == nil {
		t.Fatal("a different master key opened the value")
	}
	if _, err := a.Open(sealed[:len(sealed)-4] + "AAAA"); err == nil {
		t.Fatal("a tampered value was accepted")
	}
	if _, err := a.Open("cal1:!!!not-base64"); err == nil {
		t.Fatal("garbage was accepted")
	}
}
