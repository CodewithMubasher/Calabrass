package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

// A browser cannot put a header on an EventSource, so it gets a short-lived
// ticket instead. The session token itself must never be accepted from a URL.
func TestStreamTickets(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	server := httptest.NewServer(NewServer(Deps{Store: st, Logger: slog.Default()}))
	t.Cleanup(server.Close)
	token := signUpAndGetToken(t, server)

	// The old ?access_token= spelling is gone, even with a real session token.
	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/schedules?access_token="+token, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a session token in the URL answered %s, want 401", res.Status)
	}

	// Minting a ticket is an ordinary authenticated call.
	req, err = http.NewRequest(http.MethodPost, server.URL+"/api/auth/stream-ticket", nil)
	if err != nil {
		t.Fatalf("build ticket request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err = server.Client().Do(req)
	if err != nil {
		t.Fatalf("ticket request: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("asking for a ticket answered %s", res.Status)
	}
	var payload struct {
		Ticket string `json:"ticket"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("read the ticket: %v", err)
	}
	if payload.Ticket == "" {
		t.Fatal("the ticket was empty")
	}

	// It opens the stream it was meant for.
	ctx, cancel := context.WithCancel(context.Background())
	streamReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		server.URL+"/api/schedules/stream?stream_ticket="+payload.Ticket, nil)
	if err != nil {
		cancel()
		t.Fatalf("build stream request: %v", err)
	}
	streamRes, err := server.Client().Do(streamReq)
	if err != nil {
		cancel()
		t.Fatalf("stream request: %v", err)
	}
	if streamRes.StatusCode != http.StatusOK {
		cancel()
		_ = streamRes.Body.Close()
		t.Fatalf("a ticket did not open the stream: %s", streamRes.Status)
	}
	cancel()
	_ = streamRes.Body.Close()

	// It does not open anything else.
	req, err = http.NewRequest(http.MethodGet, server.URL+"/api/schedules?stream_ticket="+payload.Ticket, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	res, err = server.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a stream ticket called the API: %s", res.Status)
	}
}
