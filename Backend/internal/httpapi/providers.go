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
// The API key is included so the detail dialog can show and edit it. This is a
// local, single-user app whose API is bound to the loopback interface, so the
// key never leaves the machine; the CORS middleware still refuses non-local
// origins.
type providerPayload struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	BaseURL      string `json:"baseUrl"`
	APIKey       string `json:"apiKey"`
	Connected    bool   `json:"connected"`
	Enabled      bool   `json:"enabled"`
	Custom       bool   `json:"custom"`
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

// providerRows joins the registry's providers with the live model catalog.
func (s *Server) providerRows(ctx context.Context) ([]providerPayload, error) {
	catalog := map[string]ai.ProviderInfo{}
	for _, p := range s.registry.Models(ctx, false) {
		catalog[p.ID] = p
	}

	summaries := s.registry.Providers(ctx)
	rows := make([]providerPayload, 0, len(summaries))
	for _, p := range summaries {
		row := providerPayload{
			ID:        p.ID,
			Name:      p.Name,
			BaseURL:   p.BaseURL,
			APIKey:    p.APIKey,
			Connected: p.Connected,
			Enabled:   p.Enabled,
			Custom:    p.Custom,
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
	rows, err := s.providerRows(r.Context())
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

	id, err := s.newProviderID(r.Context(), name)
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
	if err := s.store.UpsertProvider(r.Context(), rec); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.registry.InvalidateCatalog()

	s.respondWithProviders(w, r)
}

// handleUpdateProvider edits a provider, or just turns it on or off. Built-in
// providers only accept a key and the toggle: their endpoint and name are part
// of the backend.
func (s *Server) handleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("providerID")
	if !s.registry.Known(r.Context(), providerID) {
		writeError(w, http.StatusNotFound, ErrUnknownProvider)
		return
	}

	var req providerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	rec, err := s.store.GetProvider(r.Context(), providerID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		// A built-in with nothing stored yet: start from its defaults and let
		// the submitted fields create the override.
		rec = store.ProviderRecord{ID: providerID, Enabled: true}
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if rec.Custom {
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
	} else {
		// Built-in: the name and endpoint belong to the backend.
		rec.Name = ""
		rec.BaseURL = ""
	}

	if req.APIKey != nil {
		rec.APIKey = strings.TrimSpace(*req.APIKey)
	}
	if req.Enabled != nil {
		rec.Enabled = *req.Enabled
	}

	if err := s.store.UpsertProvider(r.Context(), rec); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.registry.InvalidateCatalog()

	s.respondWithProviders(w, r)
}

// handleDeleteProvider removes a custom provider. Built-ins cannot be removed,
// only turned off.
func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("providerID")
	if !s.registry.Known(r.Context(), providerID) {
		writeError(w, http.StatusNotFound, ErrUnknownProvider)
		return
	}

	rec, err := s.store.GetProvider(r.Context(), providerID)
	if errors.Is(err, store.ErrNotFound) {
		// A built-in that has never been edited has no row, and cannot go.
		writeError(w, http.StatusBadRequest,
			errors.New("built-in providers cannot be removed; turn it off instead"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !rec.Custom {
		writeError(w, http.StatusBadRequest,
			errors.New("built-in providers cannot be removed; turn it off instead"))
		return
	}

	if err := s.store.DeleteProvider(r.Context(), providerID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.registry.InvalidateCatalog()

	s.respondWithProviders(w, r)
}

// newProviderID builds a readable id from the provider name, with a numeric
// suffix when the slug is already taken.
func (s *Server) newProviderID(ctx context.Context, name string) (string, error) {
	used := map[string]bool{}
	for _, p := range s.registry.Providers(ctx) {
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
