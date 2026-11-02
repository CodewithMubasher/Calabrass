package httpapi

import (
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/auth"
	"github.com/CodewithMubasher/Calabrass/backend/internal/netguard"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// Pairing is how another machine gets to use this host: the owner asks for a
// one-time code here, gives it on the other machine, and that machine becomes a
// device the owner can see and remove. See auth/pairing.go for the design.

// handleStartPairing makes a one-time code. It is for the machine the host runs
// on only: whoever can ask for a code decides who else gets in.
func (s *Server) handleStartPairing(w http.ResponseWriter, r *http.Request) {
	if !netguard.IsLoopbackRequest(r) {
		writeError(w, http.StatusForbidden, errors.New("a pairing code can only be made on the machine Calabrass runs on"))
		return
	}
	if !s.remoteOn {
		// A code that no other machine can use is worse than none: say so, and how.
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "Other devices cannot connect to this host yet. Start it with CALABRASS_ALLOW_REMOTE=1 " +
				"and a listen address other machines can reach (for example CALABRASS_ADDR=0.0.0.0:8787), then ask again.",
			"code": "remote_off",
		})
		return
	}
	user := currentUser(r.Context())
	pairing, err := s.auth.StartPairing(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code":      pairing.Code,
		"expiresAt": pairing.ExpiresAt,
		"addr":      s.hostInfo.Addr,
	})
}

// handleClaimPairing trades a code for a device session. It is the one endpoint
// a machine that is not paired yet can reach, so it answers to a rate limit and
// says only "not valid" for every kind of wrong.
func (s *Server) handleClaimPairing(w http.ResponseWriter, r *http.Request) {
	if !s.remoteOn {
		http.NotFound(w, r)
		return
	}
	ip := clientIP(r)
	if !s.claimByIP.Allow(ip) || !s.claimAll.Allow("*") {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, errors.New("too many pairing attempts; wait a minute and try again"))
		return
	}

	var body struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	paired, err := s.auth.ClaimPairing(r.Context(), body.Code, body.Name)
	switch {
	case errors.Is(err, auth.ErrBadPairingCode):
		s.log.Warn("pairing code refused", "from", ip)
		writeError(w, http.StatusForbidden, err)
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.log.Info("device paired", "device", paired.Device.Name, "from", ip)
	s.writeLedger(store.LedgerEntry{
		UserID: paired.Session.User.ID, Kind: "device", Tool: "pairing", Outcome: "paired",
		Summary: "Paired a device: " + paired.Device.Name + " (from " + ip + ")",
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"token":  paired.Session.Token,
		"user":   paired.Session.User,
		"device": map[string]any{"id": paired.Device.ID, "name": paired.Device.Name},
	})
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	if !netguard.IsLoopbackRequest(r) {
		writeError(w, http.StatusForbidden, errors.New("devices can only be managed on the machine Calabrass runs on"))
		return
	}
	user := currentUser(r.Context())
	devices, err := s.auth.Devices(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

func (s *Server) handleRevokeDevice(w http.ResponseWriter, r *http.Request) {
	if !netguard.IsLoopbackRequest(r) {
		writeError(w, http.StatusForbidden, errors.New("devices can only be managed on the machine Calabrass runs on"))
		return
	}
	user := currentUser(r.Context())
	id := r.PathValue("deviceID")
	if err := s.auth.RevokeDevice(r.Context(), user.ID, id); err != nil {
		if errors.Is(err, auth.ErrNoSuchDevice) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.log.Info("device removed", "device", id)
	s.writeLedger(store.LedgerEntry{
		UserID: user.ID, Kind: "device", Tool: "pairing", Outcome: "removed",
		Summary: "Removed a paired device (" + id + ")",
	})
	writeJSON(w, http.StatusOK, map[string]any{"removed": true})
}

// deviceOK is what the network guard asks about a request from another machine:
// does it carry the session of a paired device? It also notes, at most once a
// minute per device, that the device was just seen.
func (s *Server) deviceOK(r *http.Request) bool {
	token := s.requestCredential(r)
	if token == "" {
		return false
	}
	id, ok := s.auth.DeviceForToken(r.Context(), token)
	if !ok {
		return false
	}
	s.seenMu.Lock()
	last := s.seen[id]
	due := time.Since(last) > time.Minute
	if due {
		if s.seen == nil {
			s.seen = map[string]time.Time{}
		}
		s.seen[id] = time.Now()
	}
	s.seenMu.Unlock()
	if due {
		s.auth.TouchDevice(r.Context(), id)
	}
	return true
}

// clientIP is the address a request came from, without the port.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// seenState is the bookkeeping behind deviceOK's "at most once a minute".
type seenState struct {
	seenMu sync.Mutex
	seen   map[string]time.Time
}
