package quota

import (
	"os"
	"path/filepath"
	"testing"
)

const sample = `{
  "provider_quotas": {
    "google_gemini": {
      "provider": "Google",
      "tier": "Free Tier",
      "models": [
        {"model_id": "gemini-2.5-flash", "display_name": "Gemini 2.5 Flash",
         "rpm_limit": 15, "rpd_limit": 1500, "tpm_limit": 1000000, "status": "active"},
        {"model_id": "gemini-2.5-pro", "display_name": "Gemini 2.5 Pro",
         "rpm_limit": 2, "rpd_limit": 50, "tpm_limit": 32000, "status": "active"}
      ]
    },
    "groq": {
      "provider": "GroqCloud",
      "tier": "Free Tier",
      "models": [
        {"model_id": "gpt-oss-120b", "display_name": "GPT OSS 120B",
         "rpm_limit": 30, "rpd_limit": 14400, "tpm_limit": 15000, "status": "active"}
      ]
    }
  }
}`

func writeSample(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "model_free_tier_quotas.md")
	if err := os.WriteFile(path, []byte(sample), 0o644); err != nil {
		t.Fatalf("write quotas: %v", err)
	}
	return path
}

// The file names providers the Google/Mistral way; the catalog has to expose
// them under the ids the rest of the app uses.
func TestLoadMapsProviderKeys(t *testing.T) {
	catalog, err := Load(writeSample(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	provider, ok := catalog.Provider("google")
	if !ok {
		t.Fatal("google_gemini did not map to google")
	}
	if provider.Provider != "Google" || len(provider.Models) != 2 {
		t.Fatalf("google came back as %+v", provider)
	}

	rpd, tpm := catalog.Limits("google")
	if rpd != 1550 || tpm != 1032000 {
		t.Errorf("google limits are rpd=%d tpm=%d, want 1550 and 1032000", rpd, tpm)
	}

	if _, ok := catalog.Provider("ollama-cloud"); ok {
		t.Error("a provider the file does not cover reported quotas")
	}
	if rpd, tpm := catalog.Limits("ollama-cloud"); rpd != 0 || tpm != 0 {
		t.Errorf("unknown provider limits are %d/%d, want zero", rpd, tpm)
	}
}

// A nil catalog is what a missing file leaves behind; asking it for limits has
// to answer zero rather than panic.
func TestNilCatalogHasNoLimits(t *testing.T) {
	var catalog *Catalog
	if rpd, tpm := catalog.Limits("google"); rpd != 0 || tpm != 0 {
		t.Errorf("nil catalog limits are %d/%d, want zero", rpd, tpm)
	}
	if _, ok := catalog.Provider("google"); ok {
		t.Error("nil catalog returned a provider")
	}
}

func TestLoadReportsMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "absent.md")); err == nil {
		t.Fatal("a missing file was accepted")
	}
}
