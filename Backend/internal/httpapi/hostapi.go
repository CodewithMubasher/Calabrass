package httpapi

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/auth"
	"github.com/CodewithMubasher/Calabrass/backend/internal/host"
	"github.com/CodewithMubasher/Calabrass/backend/internal/netguard"
)

// HostHeader carries the host secret for calls that are made without a login,
// such as `cal stop`.
const HostHeader = host.SecretHeader

func (s *Server) secretMatches(given string) bool {
	given = strings.TrimSpace(given)
	return s.hostInfo.Secret != "" && given != "" &&
		subtle.ConstantTimeCompare([]byte(given), []byte(s.hostInfo.Secret)) == 1
}

// handleHostInfo says what is running: for the About page, and for a face that
// wants to know whether it is a guest of a host that outlives it.
func (s *Server) handleHostInfo(w http.ResponseWriter, r *http.Request) {
	pub := s.hostInfo.Public()
	writeJSON(w, http.StatusOK, map[string]any{
		"host":       pub,
		"webUI":      s.webUI != nil,
		"keyStorage": s.keyBackend,
		"pairing":    s.pairingOn,
		"time":       time.Now(),
	})
}

// handleLocalSignIn signs a caller in as the machine's owner, with no password,
// when it can show the host secret. The secret lives in a file only the owner
// of this user account can read, so holding it is the proof.
//
// It is how the terminal joins the same account the desktop app and the web page
// use, instead of inventing one of its own.
func (s *Server) handleLocalSignIn(w http.ResponseWriter, r *http.Request) {
	if !netguard.IsLoopbackRequest(r) {
		writeError(w, http.StatusForbidden, errors.New("the machine owner can only sign in from the machine itself"))
		return
	}
	var body struct {
		Secret string `json:"secret"`
		Create bool   `json:"create"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !s.secretMatches(body.Secret) {
		writeError(w, http.StatusForbidden, errors.New("that is not this host's secret"))
		return
	}

	session, created, err := s.auth.SignInAsOwner(r.Context(), body.Create)
	switch {
	case errors.Is(err, auth.ErrNoOwner):
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "No one has an account on this machine yet. Create one in the desktop app or the web page, or let the terminal make one.",
			"code":  "no_owner",
		})
		return
	case err != nil:
		s.writeAuthError(w, err)
		return
	}

	if created {
		s.seedBundledConnectors(r.Context(), session.User.ID)
		s.seedProviderPresets(r.Context(), session.User.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": session.Token, "user": session.User, "created": created,
	})
}

// handleHostShutdown stops the host. It needs the host secret, so only someone
// who can read the owner's data folder can do it, and it answers before it
// stops so the caller learns it worked.
func (s *Server) handleHostShutdown(w http.ResponseWriter, r *http.Request) {
	if !netguard.IsLoopbackRequest(r) {
		writeError(w, http.StatusForbidden, errors.New("the host can only be stopped from the machine itself"))
		return
	}
	if !s.secretMatches(r.Header.Get(HostHeader)) {
		writeError(w, http.StatusForbidden, errors.New("that is not this host's secret"))
		return
	}
	if s.onShutdown == nil {
		writeError(w, http.StatusNotImplemented, errors.New("this host cannot be stopped from here"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stopping": true})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		s.onShutdown()
	}()
}

// hostPublic is what /api/health adds so a face can tell what it found.
func (s *Server) hostPublic() host.Public { return s.hostInfo.Public() }
