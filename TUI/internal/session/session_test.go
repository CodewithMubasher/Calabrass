package session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"calabrass-tui/internal/api"
	"calabrass-tui/internal/settings"
)

// fakeBackend implements just the auth endpoints, with the same status codes
// as the real server.
type fakeBackend struct {
	mu     sync.Mutex
	users  map[string]string // email -> password
	tokens map[string]string // token -> email
	n      int
}

func newFake() *fakeBackend {
	return &fakeBackend{users: map[string]string{}, tokens: map[string]string{}}
}

func (f *fakeBackend) handler() http.Handler {
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, code int, v any) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	session := func(email string) api.Session {
		f.n++
		tok := "tok" + string(rune('a'+f.n))
		f.tokens[tok] = email
		return api.Session{Token: tok, User: api.User{ID: "u", Name: "n", Email: email}}
	}
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /api/auth/signup", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var b struct{ Name, Email, Password string }
		_ = json.NewDecoder(r.Body).Decode(&b)
		if len(b.Password) < 8 || !strings.Contains(b.Email, "@") {
			reply(w, 400, map[string]string{"error": "invalid"})
			return
		}
		if _, ok := f.users[b.Email]; ok {
			reply(w, 409, map[string]string{"error": "taken"})
			return
		}
		f.users[b.Email] = b.Password
		reply(w, 201, session(b.Email))
	})
	mux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var b struct{ Email, Password string }
		_ = json.NewDecoder(r.Body).Decode(&b)
		if pw, ok := f.users[b.Email]; !ok || pw != b.Password {
			reply(w, 401, map[string]string{"error": "wrong"})
			return
		}
		reply(w, 200, session(b.Email))
	})
	mux.HandleFunc("GET /api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		email, ok := f.tokens[tok]
		if !ok {
			reply(w, 401, map[string]string{"error": "no"})
			return
		}
		reply(w, 200, map[string]api.User{"user": {ID: "u", Email: email}})
	})
	return mux
}

func setup(t *testing.T) (*fakeBackend, *httptest.Server) {
	t.Helper()
	t.Setenv(EnvEmail, "")
	t.Setenv(EnvPassword, "")
	f := newFake()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	return f, srv
}

func TestFirstRunCreatesALocalAccountAndSavesIt(t *testing.T) {
	f, srv := setup(t)
	c, st := api.New(srv.URL), &settings.Settings{}

	user, err := Ensure(context.Background(), c, st)
	if err != nil {
		t.Fatal(err)
	}
	if c.Token == "" || st.Token != c.Token {
		t.Errorf("token not adopted: client=%q settings=%q", c.Token, st.Token)
	}
	if !strings.HasSuffix(user.Email, "@calabrass.local") || st.Email != user.Email || st.Password == "" {
		t.Errorf("local account not remembered: %+v / %+v", user, st)
	}
	if len(f.users) != 1 {
		t.Errorf("users = %d", len(f.users))
	}
}

func TestSavedTokenIsReusedWithoutNewAccounts(t *testing.T) {
	f, srv := setup(t)
	st := &settings.Settings{}
	if _, err := Ensure(context.Background(), api.New(srv.URL), st); err != nil {
		t.Fatal(err)
	}
	before := f.n

	c := api.New(srv.URL)
	if _, err := Ensure(context.Background(), c, st); err != nil {
		t.Fatal(err)
	}
	if f.n != before || len(f.users) != 1 {
		t.Errorf("a second start made new sessions/accounts: n=%d users=%d", f.n, len(f.users))
	}
}

func TestExpiredTokenFallsBackToStoredCredentials(t *testing.T) {
	f, srv := setup(t)
	st := &settings.Settings{}
	if _, err := Ensure(context.Background(), api.New(srv.URL), st); err != nil {
		t.Fatal(err)
	}
	f.tokens = map[string]string{} // every session expires

	c := api.New(srv.URL)
	if _, err := Ensure(context.Background(), c, st); err != nil {
		t.Fatal(err)
	}
	if len(f.users) != 1 {
		t.Errorf("expected to sign back in, got %d accounts", len(f.users))
	}
}

func TestWipedDatabaseCreatesAFreshAccount(t *testing.T) {
	f, srv := setup(t)
	st := &settings.Settings{}
	if _, err := Ensure(context.Background(), api.New(srv.URL), st); err != nil {
		t.Fatal(err)
	}
	oldEmail := st.Email
	f.users, f.tokens = map[string]string{}, map[string]string{}

	if _, err := Ensure(context.Background(), api.New(srv.URL), st); err != nil {
		t.Fatal(err)
	}
	if st.Email == oldEmail || len(f.users) != 1 {
		t.Errorf("did not make a fresh account: %q -> %q", oldEmail, st.Email)
	}
}

func TestEnvironmentCredentialsWin(t *testing.T) {
	f, srv := setup(t)
	f.users["me@example.com"] = "correct horse"
	t.Setenv(EnvEmail, "me@example.com")
	t.Setenv(EnvPassword, "correct horse")

	st := &settings.Settings{}
	user, err := Ensure(context.Background(), api.New(srv.URL), st)
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "me@example.com" || st.Email != "" {
		t.Errorf("user=%+v settings=%+v", user, st)
	}

	t.Setenv(EnvPassword, "wrong")
	st = &settings.Settings{}
	if _, err := Ensure(context.Background(), api.New(srv.URL), st); !api.IsUnauthorized(err) {
		t.Errorf("a wrong password must surface as 401, got %v", err)
	}
}

func TestUnreachableBackend(t *testing.T) {
	_, srv := setup(t)
	url := srv.URL
	srv.Close()

	_, err := Ensure(context.Background(), api.New(url), &settings.Settings{Token: "keep-me"})
	var un *ErrUnreachable
	if !errors.As(err, &un) {
		t.Fatalf("err = %v", err)
	}
}
