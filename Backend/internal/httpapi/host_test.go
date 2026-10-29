package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/ai"
	"github.com/CodewithMubasher/Calabrass/backend/internal/host"
	"github.com/CodewithMubasher/Calabrass/backend/internal/permission"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
	"github.com/CodewithMubasher/Calabrass/backend/internal/webui"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func registryFor(st *store.Store) *ai.Registry {
	return ai.NewRegistry(func(ctx context.Context, userID string) ([]ai.ProviderRecord, error) {
		rows, err := st.ListProviders(ctx, userID)
		if err != nil {
			return nil, err
		}
		out := make([]ai.ProviderRecord, 0, len(rows))
		for _, r := range rows {
			out = append(out, ai.ProviderRecord{ID: r.ID, Name: r.Name, BaseURL: r.BaseURL, APIKey: r.APIKey, Enabled: r.Enabled})
		}
		return out, nil
	})
}

// do makes one JSON request and returns the status and the decoded body.
func do(t *testing.T, base, method, path, token string, headers map[string]string, body any) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, base+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()
	out := map[string]any{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestProviderKeysAreNeverSentBack(t *testing.T) {
	st := openStore(t)
	srv := httptest.NewServer(NewServer(Deps{Store: st, Logger: slog.Default(), Registry: registryFor(st)}))
	t.Cleanup(srv.Close)

	_, su := do(t, srv.URL, "POST", "/api/auth/signup", "", nil,
		map[string]string{"name": "Ada", "email": "ada@example.com", "password": "a long enough password"})
	token := su["token"].(string)

	const key = "sk-abcdefghijklmnopqrstuvwxyz"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"m"}]}`))
	}))
	t.Cleanup(upstream.Close)
	status, _ := do(t, srv.URL, "POST", "/api/providers", token, nil,
		map[string]any{"name": "Mine", "baseUrl": upstream.URL, "apiKey": key})
	if status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("create provider: %d", status)
	}

	req, _ := http.NewRequest("GET", srv.URL+"/api/providers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	if strings.Contains(string(raw), key) || strings.Contains(string(raw), "abcdefghijklmnop") {
		t.Fatalf("the API key came back in the response: %s", raw)
	}
	var list struct {
		Providers []struct {
			Name    string `json:"name"`
			HasKey  bool   `json:"hasKey"`
			KeyHint string `json:"keyHint"`
		} `json:"providers"`
	}
	_ = json.Unmarshal(raw, &list)
	found := false
	for _, p := range list.Providers {
		if p.Name == "Mine" {
			found = true
			if !p.HasKey || p.KeyHint != "wxyz" {
				t.Fatalf("hasKey=%v hint=%q", p.HasKey, p.KeyHint)
			}
		}
	}
	if !found {
		t.Fatal("the provider is missing from the list")
	}
}

func TestMachineOwnerSignIn(t *testing.T) {
	st := openStore(t)
	info := host.Info{Secret: "s3cret-s3cret-s3cret-s3cret-s3cret-ab", Owner: host.OwnerDaemon, Version: "t"}
	srv := httptest.NewServer(NewServer(Deps{Store: st, Logger: slog.Default(), HostInfo: info}))
	t.Cleanup(srv.Close)

	if code, _ := do(t, srv.URL, "POST", "/api/auth/local", "", nil, map[string]any{"secret": "wrong"}); code != http.StatusForbidden {
		t.Fatalf("wrong secret: %d", code)
	}
	if code, body := do(t, srv.URL, "POST", "/api/auth/local", "", nil, map[string]any{"secret": info.Secret}); code != http.StatusConflict || body["code"] != "no_owner" {
		t.Fatalf("no owner yet: %d %v", code, body)
	}

	code, first := do(t, srv.URL, "POST", "/api/auth/local", "", nil, map[string]any{"secret": info.Secret, "create": true})
	if code != http.StatusOK || first["created"] != true {
		t.Fatalf("create owner: %d %v", code, first)
	}
	userID := first["user"].(map[string]any)["id"]

	code, second := do(t, srv.URL, "POST", "/api/auth/local", "", nil, map[string]any{"secret": info.Secret, "create": true})
	if code != http.StatusOK || second["created"] != false || second["user"].(map[string]any)["id"] != userID {
		t.Fatalf("the second sign-in must be the same owner: %d %v", code, second)
	}
	if code, me := do(t, srv.URL, "GET", "/api/auth/me", second["token"].(string), nil, nil); code != http.StatusOK || me["user"] == nil && me["id"] == nil {
		t.Fatalf("the owner's token does not work: %d %v", code, me)
	}
}

func TestOwnerIsTheFirstAccountWhoeverMadeIt(t *testing.T) {
	st := openStore(t)
	info := host.Info{Secret: "s3cret-s3cret-s3cret-s3cret-s3cret-ab"}
	srv := httptest.NewServer(NewServer(Deps{Store: st, Logger: slog.Default(), HostInfo: info}))
	t.Cleanup(srv.Close)

	_, su := do(t, srv.URL, "POST", "/api/auth/signup", "", nil,
		map[string]string{"name": "Ada", "email": "ada@example.com", "password": "a long enough password"})
	adaID := su["user"].(map[string]any)["id"]

	// The terminal then signs in as the same person, not as a new account.
	_, owner := do(t, srv.URL, "POST", "/api/auth/local", "", nil, map[string]any{"secret": info.Secret})
	if owner["user"].(map[string]any)["id"] != adaID {
		t.Fatalf("the owner is %v, want the first account %v", owner["user"], adaID)
	}
}

func TestHostShutdownNeedsTheSecret(t *testing.T) {
	st := openStore(t)
	var stopped atomic.Bool
	info := host.Info{Secret: "s3cret-s3cret-s3cret-s3cret-s3cret-ab"}
	srv := httptest.NewServer(NewServer(Deps{Store: st, Logger: slog.Default(), HostInfo: info,
		OnShutdown: func() { stopped.Store(true) }}))
	t.Cleanup(srv.Close)

	if code, _ := do(t, srv.URL, "POST", "/api/host/shutdown", "", nil, nil); code != http.StatusForbidden {
		t.Fatalf("no secret: %d", code)
	}
	if code, _ := do(t, srv.URL, "POST", "/api/host/shutdown", "", map[string]string{HostHeader: "nope"}, nil); code != http.StatusForbidden {
		t.Fatalf("wrong secret: %d", code)
	}
	time.Sleep(300 * time.Millisecond)
	if stopped.Load() {
		t.Fatal("the host stopped without the secret")
	}
	if code, _ := do(t, srv.URL, "POST", "/api/host/shutdown", "", map[string]string{HostHeader: info.Secret}, nil); code != http.StatusOK {
		t.Fatalf("right secret: %d", code)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !stopped.Load() {
		if time.Now().After(deadline) {
			t.Fatal("the host was asked to stop but did not")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestHealthSaysWhoOwnsTheHost(t *testing.T) {
	st := openStore(t)
	info := host.Info{Secret: "x", Owner: host.OwnerDaemon, Version: "1.2.3", PID: 4242}
	srv := httptest.NewServer(NewServer(Deps{Store: st, Logger: slog.Default(), HostInfo: info}))
	t.Cleanup(srv.Close)
	_, body := do(t, srv.URL, "GET", "/api/health", "", nil, nil)
	if body["status"] != "ok" || body["owner"] != host.OwnerDaemon || body["background"] != true || body["version"] != "1.2.3" {
		t.Fatalf("health = %v", body)
	}
	if _, leaked := body["secret"]; leaked || strings.Contains(toString(body), info.Secret+"x") {
		t.Fatal("health must never carry the secret")
	}
}

func toString(v any) string { raw, _ := json.Marshal(v); return string(raw) }

func TestServerRefusesStrangersOnTheNetwork(t *testing.T) {
	st := openStore(t)
	code := "a-long-enough-pairing-secret"
	h := NewServer(Deps{Store: st, Logger: slog.Default(), ListenAddr: "0.0.0.0:8787", PairingCode: code})

	call := func(method, path, remote string, hdr map[string]string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		req.RemoteAddr = remote
		req.Host = "192.168.1.20:8787"
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if c := call("GET", "/api/chats", "192.168.1.50:5000", nil); c != http.StatusForbidden {
		t.Fatalf("an unpaired device must be refused, got %d", c)
	}
	if c := call("POST", "/api/auth/signup", "192.168.1.50:5000", map[string]string{"X-Calabrass-Pairing": code}); c != http.StatusForbidden {
		t.Fatalf("a paired device must not be able to sign up, got %d", c)
	}
	if c := call("GET", "/api/chats", "192.168.1.50:5000", map[string]string{"X-Calabrass-Pairing": code}); c != http.StatusUnauthorized {
		t.Fatalf("a paired device still needs a session, got %d", c)
	}
}

func TestWebInterfaceIsServedBesideTheAPI(t *testing.T) {
	st := openStore(t)
	site := fstest.MapFS{"index.html": {Data: []byte("<html>calabrass</html>")}}
	srv := httptest.NewServer(NewServer(Deps{Store: st, Logger: slog.Default(), WebUI: webui.Handler(site)}))
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/chats/abc")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), "calabrass") {
		t.Fatalf("the page: %d %q", res.StatusCode, body)
	}
	if code, _ := do(t, srv.URL, "GET", "/api/chats", "", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("the API behind the page must still need a session, got %d", code)
	}
	if code, body := do(t, srv.URL, "GET", "/api/health", "", nil, nil); code != 200 || body["status"] != "ok" {
		t.Fatalf("health: %d %v", code, body)
	}
}

func TestScheduleScopeIsKeptAndTheModelCannotSetIt(t *testing.T) {
	st := openStore(t)
	srv := httptest.NewServer(NewServer(Deps{Store: st, Logger: slog.Default()}))
	t.Cleanup(srv.Close)
	_, su := do(t, srv.URL, "POST", "/api/auth/signup", "", nil,
		map[string]string{"name": "Ada", "email": "ada@example.com", "password": "a long enough password"})
	token := su["token"].(string)

	runAt := time.Now().Add(time.Hour).UnixMilli()
	code, body := do(t, srv.URL, "POST", "/api/schedules", token, nil, map[string]any{
		"prompt": "run the tests", "runAt": runAt,
		"scope": map[string]any{"maxTier": 2, "commands": []string{"npm test"}},
	})
	if code != http.StatusOK {
		t.Fatalf("create: %d %v", code, body)
	}
	task := body["tasks"].([]any)[0].(map[string]any)
	scope := permission.ParseScope(task["scope"].(string))
	if !scope.Allows(permission.ClassifyCommand("npm test", "", "/w")) {
		t.Fatalf("the person's scope was not kept: %v", task["scope"])
	}
	if scope.Allows(permission.ClassifyCommand("npm publish", "", "/w")) {
		t.Fatal("the scope allows more than the person wrote")
	}

	// A task the model books carries no scope at all, which means the default.
	user, _ := st.OwnerUser(context.Background())
	s := &Server{store: st, log: slog.Default()}
	if _, err := s.scheduleFromTool(context.Background(), "", user.ID, scheduleTaskArgs{Prompt: "later", AfterMinutes: 5}); err != nil {
		t.Fatal(err)
	}
	tasks, _ := st.ListScheduledTasks(context.Background(), user.ID)
	for _, tk := range tasks {
		if tk.Prompt == "later" && tk.Scope != "" {
			t.Fatalf("a model-booked task carries scope %q", tk.Scope)
		}
	}
}
