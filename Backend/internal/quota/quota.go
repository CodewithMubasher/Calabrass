// Package quota reads the free-tier limits from model_free_tier_quotas.md.
//
// The file is JSON, despite its .md name, so it can be edited by hand and read
// without a parser of its own. Each provider key maps onto the provider id the
// rest of the app uses (google_gemini -> google, for example).
package quota

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Model is one model's free-tier allowance.
type Model struct {
	ID       string `json:"model_id"`
	Name     string `json:"display_name"`
	RPMLimit int    `json:"rpm_limit"`
	RPDLimit int    `json:"rpd_limit"`
	TPMLimit int    `json:"tpm_limit"`
	Status   string `json:"status"`
}

// Provider is one provider's block: its tier and the models it covers.
type Provider struct {
	Provider string  `json:"provider"`
	Tier     string  `json:"tier"`
	Models   []Model `json:"models"`
}

type file struct {
	ProviderQuotas map[string]Provider `json:"provider_quotas"`
}

// Catalog holds the quotas keyed by the app's provider id.
type Catalog struct {
	providers map[string]Provider
}

// providerID turns a key from the file into the id the registry knows.
func providerID(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "google_gemini":
		return "google"
	case "mistral_ai":
		return "mistral"
	default:
		return strings.ToLower(strings.TrimSpace(key))
	}
}

// Load reads the quota file at path.
func Load(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("quota: read %s: %w", path, err)
	}

	var parsed file
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("quota: parse %s: %w", path, err)
	}

	catalog := &Catalog{providers: make(map[string]Provider, len(parsed.ProviderQuotas))}
	for key, provider := range parsed.ProviderQuotas {
		catalog.providers[providerID(key)] = provider
	}
	return catalog, nil
}

// Provider returns the quota block for a provider.
func (c *Catalog) Provider(providerID string) (Provider, bool) {
	if c == nil {
		return Provider{}, false
	}
	provider, ok := c.providers[providerID]
	return provider, ok
}

// Limits returns the provider's total daily request allowance and total
// per-minute token allowance, summed across its models. A provider the file
// does not cover has no limits.
func (c *Catalog) Limits(providerID string) (rpd, tpm int) {
	provider, ok := c.Provider(providerID)
	if !ok {
		return 0, 0
	}
	for _, model := range provider.Models {
		rpd += model.RPDLimit
		tpm += model.TPMLimit
	}
	return rpd, tpm
}
