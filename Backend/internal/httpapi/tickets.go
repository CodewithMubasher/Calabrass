package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"sync"
	"time"
)

const (
	// streamTicketTTL is how long a ticket for a URL-only endpoint lives. It is
	// long enough for an EventSource to reconnect after a blip, and short enough
	// that a URL caught in a proxy log is not a lasting credential.
	streamTicketTTL = 5 * time.Minute
	// streamTicketLimit caps the map so a flood cannot grow it without bound.
	streamTicketLimit = 4096
)

// streamTickets hands out short-lived ways into the endpoints a browser cannot
// put an Authorization header on: EventSource, WebSocket and download links.
//
// The session token itself never goes in a URL. A ticket opens only the
// streaming paths, so a link copied out of history stops working minutes later
// and cannot be used to call the rest of the API.
type streamTickets struct {
	mu    sync.Mutex
	byID  map[string]streamTicket
	swept time.Time
}

// streamTicket is the session a ticket stands for and when it stops working.
type streamTicket struct {
	token   string
	expires time.Time
}

func newStreamTickets() *streamTickets {
	return &streamTickets{byID: map[string]streamTicket{}}
}

// issue mints a ticket for a session token.
func (s *streamTickets) issue(token string) (string, error) {
	id, err := randomTicket()
	if err != nil {
		return "", err
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Expired tickets are dropped lazily: on a slow host nobody may ask often
	// enough for the map to clean itself otherwise.
	if now.Sub(s.swept) > time.Minute || len(s.byID) >= streamTicketLimit {
		for key, ticket := range s.byID {
			if !ticket.expires.After(now) {
				delete(s.byID, key)
			}
		}
		s.swept = now
	}
	// A flood of live tickets still cannot grow without bound. The one dropped
	// belongs to a page that can ask again; refusing the new one would not.
	if len(s.byID) >= streamTicketLimit {
		for key := range s.byID {
			delete(s.byID, key)
			break
		}
	}

	s.byID[id] = streamTicket{token: token, expires: now.Add(streamTicketTTL)}
	return id, nil
}

// resolve returns the session a ticket stands for, or "" when it is unknown or
// has expired. A ticket may be presented more than once within its lifetime:
// EventSource reconnects to the same URL, and a dropped stream must be able to
// come back.
func (s *streamTickets) resolve(id string) string {
	if id == "" {
		return ""
	}

	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	ticket, ok := s.byID[id]
	if !ok {
		return ""
	}
	if !ticket.expires.After(now) {
		delete(s.byID, id)
		return ""
	}
	return ticket.token
}

func randomTicket() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// handleStreamTicket hands a signed-in caller a ticket for the streaming
// endpoints. It is authenticated by the ordinary bearer header, so the session
// token is never the thing in the URL.
func (s *Server) handleStreamTicket(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, errors.New("a bearer token is required"))
		return
	}
	ticket, err := s.tickets.issue(token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticket":    ticket,
		"expiresIn": int(streamTicketTTL.Seconds()),
	})
}
