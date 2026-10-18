package main

import (
	"bytes"
	"os"
	"testing"
)

// The desktop binary carries a copy of the quota file so an installed app still
// has limits. The root file is the source of truth, so the copy must match it.
func TestEmbeddedQuotasMatchRoot(t *testing.T) {
	root, err := os.ReadFile("../../model_free_tier_quotas.md")
	if err != nil {
		t.Skipf("quota source is unavailable: %v", err)
	}
	if !bytes.Equal(root, embeddedQuotas) {
		t.Fatal("desktop/assets/model_free_tier_quotas.md is stale; copy ../../model_free_tier_quotas.md over it")
	}
}
