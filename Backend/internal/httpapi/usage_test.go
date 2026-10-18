package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/ai"
	"github.com/CodewithMubasher/Calabrass/backend/internal/quota"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

const usageQuotas = `{
  "provider_quotas": {
    "google_gemini": {
      "provider": "Google",
      "tier": "Free Tier",
      "models": [
        {"model_id": "gemini-2.5-flash", "display_name": "Gemini 2.5 Flash",
         "rpm_limit": 15, "rpd_limit": 100, "tpm_limit": 1000, "status": "active"}
      ]
    }
  }
}`

// The usage endpoint answers with today's real totals, the per-provider split,
// the free-tier allowance and a seven-day series that includes empty days.
func TestUsageEndpoint(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	quotaPath := filepath.Join(t.TempDir(), "model_free_tier_quotas.md")
	if err := os.WriteFile(quotaPath, []byte(usageQuotas), 0o644); err != nil {
		t.Fatalf("write quotas: %v", err)
	}
	catalog, err := quota.Load(quotaPath)
	if err != nil {
		t.Fatalf("load quotas: %v", err)
	}

	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	events := []store.UsageEvent{
		{ProviderID: "google", ModelID: "gemini-2.5-flash", InputTokens: 100, OutputTokens: 50, CreatedAt: now.UnixMilli()},
		{ProviderID: "google", ModelID: "gemini-2.5-flash", InputTokens: 200, OutputTokens: 100, CreatedAt: now.UnixMilli()},
		{ProviderID: "groq", ModelID: "gpt-oss-120b", InputTokens: 10, OutputTokens: 5, CreatedAt: yesterday.UnixMilli()},
	}
	for _, event := range events {
		if err := st.RecordUsage(t.Context(), event); err != nil {
			t.Fatalf("record usage: %v", err)
		}
	}

	registry := ai.NewRegistry("", "", nil, nil)
	server := httptest.NewServer(NewServer(Deps{
		Store:    st,
		Registry: registry,
		Quotas:   catalog,
		Logger:   slog.Default(),
	}))
	t.Cleanup(server.Close)

	res, err := server.Client().Get(server.URL + "/api/usage")
	if err != nil {
		t.Fatalf("GET /api/usage: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/usage answered %s", res.Status)
	}

	var payload struct {
		Today  store.UsageTotals `json:"today"`
		Minute store.UsageTotals `json:"minute"`
		Limits struct {
			RPD int `json:"rpd"`
			TPM int `json:"tpm"`
		} `json:"limits"`
		Providers []struct {
			ID string `json:"id"`
			store.UsageTotals
			RPDLimit int `json:"rpdLimit"`
			TPMLimit int `json:"tpmLimit"`
			Daily    []struct {
				Date string `json:"date"`
				store.UsageTotals
			} `json:"daily"`
		} `json:"providers"`
		Daily []struct {
			Date string `json:"date"`
			store.UsageTotals
		} `json:"daily"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode the usage answer: %v", err)
	}

	if payload.Today.Requests != 2 || payload.Today.TotalTokens != 450 {
		t.Errorf("today is %+v, want 2 requests and 450 tokens", payload.Today)
	}
	if payload.Minute.Requests != 2 {
		t.Errorf("minute has %d requests, want 2", payload.Minute.Requests)
	}
	if payload.Limits.RPD != 100 || payload.Limits.TPM != 1000 {
		t.Errorf("limits are %+v, want the google model's 100 RPD and 1000 TPM", payload.Limits)
	}
	if len(payload.Daily) != usageDayCount {
		t.Fatalf("the series has %d days, want %d", len(payload.Daily), usageDayCount)
	}
	if got := payload.Daily[len(payload.Daily)-1]; got.Date != now.Format("2006-01-02") || got.Requests != 2 {
		t.Errorf("the last day is %+v, want today with 2 requests", got)
	}
	if got := payload.Daily[len(payload.Daily)-2]; got.Date != yesterday.Format("2006-01-02") || got.Requests != 1 {
		t.Errorf("the day before is %+v, want yesterday with 1 request", got)
	}

	var google, groq *int
	for index := range payload.Providers {
		provider := &payload.Providers[index]
		switch provider.ID {
		case "google":
			google = &index
			if provider.Requests != 2 || provider.RPDLimit != 100 || provider.TPMLimit != 1000 {
				t.Errorf("google is %+v, want 2 requests and the file's limits", provider)
			}
			if len(provider.Daily) != usageDayCount {
				t.Errorf("google's series has %d days", len(provider.Daily))
			}
		case "groq":
			groq = &index
			if provider.Requests != 0 || provider.RPDLimit != 0 {
				t.Errorf("groq is %+v, want no usage and no quota", provider)
			}
		}
	}
	if google == nil || groq == nil {
		t.Fatalf("both providers should be listed, got %d rows", len(payload.Providers))
	}
}
