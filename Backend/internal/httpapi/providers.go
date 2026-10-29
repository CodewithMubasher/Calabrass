package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/CodewithMubasher/Calabrass/backend/internal/ai"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// ErrUnknownProvider is returned for a provider the backend cannot serve.
var ErrUnknownProvider = errors.New("unknown provider")

// providerPayload is one row of the providers page.
//
// The API key is never sent back. A key is write-only: the page learns whether
// one is saved, and the last few characters so a person can tell which one it
// is, and can replace it, but cannot read it. A key shown in a page is a key an
// extension, a screenshot or a script on that page can take.
type providerPayload struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	// HasKey is true when a key is saved. KeyHint is its last four characters,
	// and only for a key long enough that four characters give nothing away.
	HasKey       bool   `json:"hasKey"`
	KeyHint      string `json:"keyHint,omitempty"`
	Connected    bool   `json:"connected"`
	Enabled      bool   `json:"enabled"`
	DefaultModel string `json:"defaultModel"`
	ModelCount   int    `json:"modelCount"`
}

// providerRequest is the body of a create or update. Every field is optional so
// a toggle can send only `enabled`.
type providerRequest struct {
	Name    *string `json:"name"`
	BaseURL *string `json:"baseUrl"`
	APIKey  *string `json:"apiKey"`
	Enabled *bool   `json:"enabled"`
}

// providerRows joins the account's registry providers with the live model
// catalog.
func (s *Server) providerRows(ctx context.Context, userID string) ([]providerPayload, error) {
	reg := s.registryFor(userID)
	catalog := map[string]ai.ProviderInfo{}
	for _, p := range reg.Models(ctx, false) {
		catalog[p.ID] = p
	}

	summaries := reg.Providers(ctx)
	rows := make([]providerPayload, 0, len(summaries))
	for _, p := range summaries {
		row := providerPayload{
			ID:        p.ID,
			Name:      p.Name,
			BaseURL:   p.BaseURL,
			HasKey:    strings.TrimSpace(p.APIKey) != "",
			KeyHint:   keyHint(p.APIKey),
			Connected: p.Connected,
			Enabled:   p.Enabled,
		}
		if info, ok := catalog[p.ID]; ok {
			row.DefaultModel = info.DefaultModel
			row.ModelCount = len(info.Models)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// respondWithProviders writes the whole list, so the client stays in step with
// the server after a change.
func (s *Server) respondWithProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := s.providerRows(r.Context(), currentUser(r.Context()).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": rows})
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	s.respondWithProviders(w, r)
}

// handleCreateProvider adds a custom, OpenAI-compatible provider.
func (s *Server) handleCreateProvider(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	var req providerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	name := strings.TrimSpace(deref(req.Name))
	baseURL := ai.NormaliseBaseURL(deref(req.BaseURL))
	if name == "" {
		writeError(w, http.StatusBadRequest, errors.New("name is required"))
		return
	}
	if err := validateBaseURL(baseURL); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	id, err := s.newProviderID(r.Context(), user.ID, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	rec := store.ProviderRecord{
		ID:      id,
		Name:    name,
		BaseURL: baseURL,
		APIKey:  strings.TrimSpace(deref(req.APIKey)),
		Enabled: enabled,
		Custom:  true,
	}
	if err := s.store.UpsertProvider(r.Context(), rec, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.registryFor(user.ID).InvalidateCatalog()

	s.respondWithProviders(w, r)
}

// handleUpdateProvider edits a provider, or just turns it on or off. Every field
// is optional, so a toggle can send only `enabled`.
func (s *Server) handleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	reg := s.registryFor(user.ID)

	var req providerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	rec, err := s.store.GetProvider(r.Context(), r.PathValue("providerID"), user.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, ErrUnknownProvider)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if req.Name != nil {
		rec.Name = strings.TrimSpace(*req.Name)
	}
	if req.BaseURL != nil {
		rec.BaseURL = ai.NormaliseBaseURL(*req.BaseURL)
	}
	if rec.Name == "" {
		writeError(w, http.StatusBadRequest, errors.New("name is required"))
		return
	}
	if err := validateBaseURL(rec.BaseURL); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.APIKey != nil {
		rec.APIKey = strings.TrimSpace(*req.APIKey)
	}
	if req.Enabled != nil {
		rec.Enabled = *req.Enabled
	}

	if err := s.store.UpsertProvider(r.Context(), rec, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	reg.InvalidateCatalog()

	s.respondWithProviders(w, r)
}

// handleDeleteProvider removes a provider, and with it its endpoint and key.
func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	reg := s.registryFor(user.ID)

	if err := s.store.DeleteProvider(r.Context(), r.PathValue("providerID"), user.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, ErrUnknownProvider)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	reg.InvalidateCatalog()

	s.respondWithProviders(w, r)
}

// newProviderID builds a readable id from the provider name, with a numeric
// suffix when the slug is already taken on that account.
func (s *Server) newProviderID(ctx context.Context, userID, name string) (string, error) {
	used := map[string]bool{}
	for _, p := range s.registryFor(userID).Providers(ctx) {
		used[p.ID] = true
	}

	base := slugify(name)
	if base == "" {
		base = "provider"
	}
	if !used[base] {
		return base, nil
	}
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !used[candidate] {
			return candidate, nil
		}
	}
	return "", errors.New("could not find a free provider id")
}

// slugify reduces a name to lowercase letters, digits and dashes.
func slugify(name string) string {
	var b strings.Builder
	dashed := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dashed = false
		default:
			if !dashed && b.Len() > 0 {
				b.WriteByte('-')
				dashed = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func validateBaseURL(raw string) error {
	if raw == "" {
		return errors.New("base URL is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("base URL is not valid: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("base URL must start with http:// or https://")
	}
	if u.Host == "" {
		return errors.New("base URL needs a host")
	}
	return nil
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// keyHint is the tail of a key, for telling two saved keys apart. Short keys
// show nothing: four characters of a short key is a large part of it.
func keyHint(key string) string {
	key = strings.TrimSpace(key)
	if len(key) < 16 {
		return ""
	}
	return key[len(key)-4:]
}
