package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallQuotasKeepsThePersonsEdits(t *testing.T) {
	dir := t.TempDir()
	path := installQuotas(dir, []byte("shipped v1"))
	if filepath.Base(path) != "model_free_tier_quotas.md" {
		t.Fatalf("the backend must read the person's copy, got %s", path)
	}
	if got, _ := os.ReadFile(path); string(got) != "shipped v1" {
		t.Fatalf("first run: %q", got)
	}

	// The person edits their copy; the next launch (and an update) must not undo it.
	if err := os.WriteFile(path, []byte("my edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	installQuotas(dir, []byte("shipped v2"))
	if got, _ := os.ReadFile(path); string(got) != "my edit" {
		t.Fatalf("a hand edit was overwritten: %q", got)
	}
	// The shipped copy is refreshed, so the new limits are there to compare or copy.
	if got, _ := os.ReadFile(filepath.Join(dir, "model_free_tier_quotas.default.md")); string(got) != "shipped v2" {
		t.Fatalf("the shipped copy was not refreshed: %q", got)
	}
}
