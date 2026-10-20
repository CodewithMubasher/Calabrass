package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/CodewithMubasher/Calabrass/backend/internal/auth"
	"github.com/CodewithMubasher/Calabrass/backend/internal/providers"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// handleSignUp creates an account and returns its first session.
func (s *Server) handleSignUp(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	session, err := s.auth.SignUp(r.Context(), body.Name, body.Email, body.Password)
	if err != nil {
		s.writeAuthError(w, err)
		return
	}

	s.seedBundledConnectors(r.Context(), session.User.ID)
	s.seedProviderPresets(r.Context(), session.User.ID)
	writeJSON(w, http.StatusCreated, session)
}

// seedBundledConnectors gives a brand new account the MCP servers that shipped
// with the app, so they are already there to switch on rather than something to
// be typed in by path. A server the installer left out was never found, and so
// is not registered. Seeding only happens once, at sign-up: a server the person
// then removes stays removed.
//
// They are registered switched off. A server that drives Excel, the keyboard
// and other people's windows is a decision, not a default: it is listed and
// waiting, and does nothing at all until the person turns it on.
func (s *Server) seedBundledConnectors(ctx context.Context, userID string) {
	for _, bundled := range s.bundled {
		record, err := s.store.CreateConnector(ctx, bundled.Name, bundled.Command, userID)
		if err != nil {
			// The account exists and is signed in; a connector that could not be
			// written down must not turn that into a failed sign-up.
			s.log.Warn("could not register a bundled MCP server",
				"connector", bundled.Name, "error", err)
			continue
		}
		if _, err := s.store.SetConnectorEnabled(ctx, record.ID, false, userID); err != nil {
			s.log.Warn("could not leave a bundled MCP server switched off",
				"connector", bundled.Name, "error", err)
		}
	}
}

// seedProviderPresets gives a brand new account the pre-configured AI provider
// templates, so they are already there to configure rather than something to
// be typed in from scratch. A preset the installer left out was never found,
// and so is not registered. Seeding only happens once, at sign-up: a preset
// the person then removes stays removed.
//
// They are registered switched off with empty API keys. Adding a key is a
// deliberate choice: the provider is listed and waiting, and does nothing at
// all until the person turns it on.
func (s *Server) seedProviderPresets(ctx context.Context, userID string) {
	for _, preset := range providers.ProviderPresets {
		// Use UpsertProvider to create or update the preset
		err := s.store.UpsertProvider(ctx, store.ProviderRecord{
			ID:        preset.ID,
			Name:      preset.Name,
			BaseURL:   preset.BaseURL,
			APIKey:    "", // Empty - user must add their own key
			Enabled:   false, // Disabled by default
			Custom:    false, // These are built-in presets
		}, userID)
		if err != nil {
			s.log.Warn("could not register a provider preset",
				"provider", preset.ID, "error", err)
			continue
		}
		// Note: We intentionally don't update the default model here.
		// The provider's default model is handled by the registry when listing models.
	}
}

// handleSignIn checks a password and returns a session.
func (s *Server) handleSignIn(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	session, err := s.auth.SignIn(r.Context(), body.Email, body.Password)
	if err != nil {
		s.writeAuthError(w, err)
		return
	}

	// Seed provider presets if this is the first sign-in (user has no providers yet)
	providers, err := s.store.ListProviders(r.Context(), session.User.ID)
	if err != nil {
		s.log.Warn("could not check existing providers", "user", session.User.ID, "error", err)
	} else if len(providers) == 0 {
		s.seedProviderPresets(r.Context(), session.User.ID)
	}

	writeJSON(w, http.StatusOK, session)
}

// handleMe resolves the bearer token to the account it belongs to.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r.Context())
	if user.ID == "" {
		s.writeAuthError(w, auth.ErrUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]auth.User{"user": user})
}

// handleSignOut revokes the bearer token. Signing out twice is harmless.
func (s *Server) handleSignOut(w http.ResponseWriter, r *http.Request) {
	if err := s.auth.SignOut(r.Context(), requestToken(r)); err != nil {
		s.writeAuthError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeAuthError maps the auth service's errors to status codes, and keeps
// anything unexpected out of the response body.
func (s *Server) writeAuthError(w http.ResponseWriter, err error) {
	var invalid *auth.ValidationError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, auth.ErrEmailTaken):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, err)
	default:
		s.log.Error("auth request failed", "error", err)
		writeError(w, http.StatusInternalServerError, errors.New("the account request failed; try again"))
	}
}

// bearerToken reads "Authorization: Bearer <token>", returning "" when there is
// no usable header.
func bearerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(header) < 7 || !strings.EqualFold(header[:7], "bearer ") {
		return ""
	}
	return strings.TrimSpace(header[7:])
}

// requestToken accepts the header, or the query parameter the streaming
// endpoints use because EventSource and WebSocket cannot set headers.
func requestToken(r *http.Request) string {
	if token := bearerToken(r); token != "" {
		return token
	}
	return strings.TrimSpace(r.URL.Query().Get("access_token"))
}
