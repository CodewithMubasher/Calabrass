package httpapi

import (
	"bytes"
	"context"
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

	registry := ai.NewRegistry(func(ctx context.Context, userID string) ([]ai.ProviderRecord, error) {
		rows, err := st.ListProviders(ctx, userID)
		if err != nil {
			return nil, err
		}
		records := make([]ai.ProviderRecord, 0, len(rows))
		for _, row := range rows {
			records = append(records, ai.ProviderRecord{
				ID:      row.ID,
				Name:    row.Name,
				BaseURL: row.BaseURL,
				APIKey:  row.APIKey,
				Enabled: row.Enabled,
			})
		}
		return records, nil
	})

	server := httptest.NewServer(NewServer(Deps{
		Store:    st,
		Registry: registry,
		Quotas:   catalog,
		Logger:   slog.Default(),
	}))
	t.Cleanup(server.Close)

	// Sign up a test user and get a token for authenticated requests
	// Also get the user ID to add providers for that user
	type signupResponse struct {
		Token string `json:"token"`
		User  struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	var signupResp signupResponse
	{
		signUp := map[string]string{
			"name":     "Test User",
			"email":    "test@example.com",
			"password": "a long enough password",
		}
		var body bytes.Buffer
		if err := json.NewEncoder(&body).Encode(signUp); err != nil {
			t.Fatalf("encode sign up: %v", err)
		}
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/auth/signup", &body)
		if err != nil {
			t.Fatalf("build signup request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("signup request: %v", err)
		}
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("signup status = %d, want 201", res.StatusCode)
		}
		if err := json.NewDecoder(res.Body).Decode(&signupResp); err != nil {
			t.Fatalf("decode signup response: %v", err)
		}
	}
	token := signupResp.Token
	userID := signupResp.User.ID

	// Add providers to the database for the signed-up user
	_ = st.UpsertProvider(t.Context(), store.ProviderRecord{
		ID:      "google",
		Name:    "Google",
		BaseURL: "https://generativelanguage.googleapis.com",
		APIKey:  "test-key",
		Enabled: true,
	}, userID)
	_ = st.UpsertProvider(t.Context(), store.ProviderRecord{
		ID:      "groq",
		Name:    "Groq",
		BaseURL: "https://api.groq.com/openai/v1",
		APIKey:  "test-key",
		Enabled: true,
	}, userID)

	// Record usage events for the signed-up user
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	events := []store.UsageEvent{
		{UserID: userID, ProviderID: "google", ModelID: "gemini-2.5-flash", InputTokens: 100, OutputTokens: 50, CreatedAt: now.UnixMilli()},
		{UserID: userID, ProviderID: "google", ModelID: "gemini-2.5-flash", InputTokens: 200, OutputTokens: 100, CreatedAt: now.UnixMilli()},
		{UserID: userID, ProviderID: "groq", ModelID: "gpt-oss-120b", InputTokens: 10, OutputTokens: 5, CreatedAt: yesterday.UnixMilli()},
	}
	for _, event := range events {
		if err := st.RecordUsage(t.Context(), event); err != nil {
			t.Fatalf("record usage: %v", err)
		}
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/usage", nil)
	if err != nil {
		t.Fatalf("build GET /api/usage: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := http.DefaultClient.Do(req)
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