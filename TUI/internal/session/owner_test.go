package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"calabrass-tui/internal/api"
	"calabrass-tui/internal/settings"
)

// ownerBackend answers the machine-owner sign-in the way the host does.
func ownerBackend(t *testing.T, secret string, signups *int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("POST /api/auth/local", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Secret string `json:"secret"`
		}
		_ = json.NewDecoder(r.Body).Decode(&b)
		if b.Secret != secret {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"no"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(api.Session{Token: "owner-token", User: api.User{ID: "owner", Email: "owner@calabrass.local"}})
	})
	mux.HandleFunc("POST /api/auth/signup", func(w http.ResponseWriter, r *http.Request) {
		*signups++
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(api.Session{Token: "own-account", User: api.User{ID: "tui"}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeHostFile(t *testing.T, secret string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CALABRASS_DATA_DIR", dir)
	raw, _ := json.Marshal(map[string]any{"addr": "127.0.0.1:1", "pid": 1, "owner": "daemon", "secret": secret})
	if err := os.WriteFile(filepath.Join(dir, "host.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTheTerminalSignsInAsTheMachineOwner(t *testing.T) {
	t.Setenv(EnvEmail, "")
	t.Setenv(EnvPassword, "")
	writeHostFile(t, "the-host-secret")
	signups := 0
	srv := ownerBackend(t, "the-host-secret", &signups)

	c := api.New(srv.URL)
	st := &settings.Settings{}
	user, err := Ensure(context.Background(), c, st)
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != "owner" || st.Token != "owner-token" {
		t.Fatalf("signed in as %+v with token %q", user, st.Token)
	}
	if signups != 0 {
		t.Fatal("the terminal made an account of its own instead of using the owner's")
	}
	if st.Email != "" || st.Password != "" {
		t.Fatal("no password should be stored for the owner sign-in")
	}
}

func TestAStaleSecretFallsBackToAnAccountOfItsOwn(t *testing.T) {
	t.Setenv(EnvEmail, "")
	t.Setenv(EnvPassword, "")
	writeHostFile(t, "a-secret-from-a-dead-host")
	signups := 0
	srv := ownerBackend(t, "the-real-secret", &signups)

	st := &settings.Settings{}
	if _, err := Ensure(context.Background(), api.New(srv.URL), st); err != nil {
		t.Fatal(err)
	}
	if signups != 1 || st.Email == "" {
		t.Fatalf("expected the fallback account, signups=%d email=%q", signups, st.Email)
	}
}

func TestNoHostFileMeansTheOldBehaviour(t *testing.T) {
	t.Setenv(EnvEmail, "")
	t.Setenv(EnvPassword, "")
	t.Setenv("CALABRASS_DATA_DIR", t.TempDir())
	signups := 0
	srv := ownerBackend(t, "x", &signups)
	if _, err := Ensure(context.Background(), api.New(srv.URL), &settings.Settings{}); err != nil {
		t.Fatal(err)
	}
	if signups != 1 {
		t.Fatalf("signups = %d", signups)
	}
}
