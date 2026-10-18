package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/CodewithMubasher/Calabrass/backend/internal/auth"
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
	writeJSON(w, http.StatusCreated, session)
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
