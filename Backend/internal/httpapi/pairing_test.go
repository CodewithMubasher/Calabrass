package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// peer builds requests as they would arrive from a given address, straight into
// the server's handler (guard included), so "another machine" is exact.
type peer struct {
	t *testing.T
	h http.Handler
}

func (p peer) call(remote, method, path, token string, body any) (int, map[string]any) {
	p.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	req.RemoteAddr = remote
	req.Host = "192.168.1.20:8787"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	p.h.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

const (
	here   = "127.0.0.1:50000"
	laptop = "192.168.1.50:51000"
)

func remoteServer(t *testing.T, allow bool) (peer, string) {
	t.Helper()
	st := openStore(t)
	h := NewServer(Deps{Store: st, Logger: slog.Default(), ListenAddr: "0.0.0.0:8787", AllowRemote: allow})
	p := peer{t: t, h: h}
	code, su := p.call(here, "POST", "/api/auth/signup", "", map[string]string{
		"name": "Ada", "email": "ada@example.com", "password": "a long enough password"})
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("signup: %d %v", code, su)
	}
	return p, su["token"].(string)
}

func TestPairingNeedsRemoteAccessSwitchedOn(t *testing.T) {
	p, owner := remoteServer(t, false)
	code, body := p.call(here, "POST", "/api/pairing/start", owner, nil)
	if code != http.StatusConflict || body["code"] != "remote_off" || !strings.Contains(body["error"].(string), "CALABRASS_ALLOW_REMOTE") {
		t.Fatalf("a code for a host that nobody can reach must say how to fix it: %d %v", code, body)
	}
	if code, _ := p.call(laptop, "POST", "/api/pairing/claim", "", map[string]string{"code": "ABCD-EFGH"}); code != http.StatusForbidden {
		t.Fatalf("a remote claim with remote access off: %d", code)
	}
}

func TestAPairedDeviceCanUseTheHostAndNothingElseDoes(t *testing.T) {
	p, owner := remoteServer(t, true)

	// The owner, at the machine, asks for a code.
	code, start := p.call(here, "POST", "/api/pairing/start", owner, nil)
	if code != http.StatusOK {
		t.Fatalf("start: %d %v", code, start)
	}
	pairingCode := start["code"].(string)
	if len(pairingCode) != 9 || pairingCode[4] != '-' {
		t.Fatalf("code %q should look like ABCD-EFGH", pairingCode)
	}

	// A stranger on the network gets nowhere.
	if c, _ := p.call(laptop, "GET", "/api/chats", "", nil); c != http.StatusForbidden {
		t.Fatalf("an unpaired machine reached the API: %d", c)
	}
	if c, _ := p.call(laptop, "POST", "/api/auth/login", "", map[string]string{"email": "ada@example.com", "password": "a long enough password"}); c != http.StatusForbidden {
		t.Fatalf("a password sign-in from another machine must be refused, got %d", c)
	}

	// The other machine gives the code, typed however a person types it.
	typed := strings.ToLower(strings.ReplaceAll(pairingCode, "-", " "))
	code, claimed := p.call(laptop, "POST", "/api/pairing/claim", "", map[string]string{"code": typed, "name": "Ada's laptop"})
	if code != http.StatusOK {
		t.Fatalf("claim: %d %v", code, claimed)
	}
	device := claimed["token"].(string)
	if claimed["device"].(map[string]any)["name"] != "Ada's laptop" {
		t.Fatalf("device = %v", claimed["device"])
	}

	// Now that machine is signed in as the owner...
	if c, _ := p.call(laptop, "GET", "/api/chats", device, nil); c != http.StatusOK {
		t.Fatalf("a paired device was refused: %d", c)
	}
	// ...but cannot decide who else gets in, create accounts, or stop the host.
	for _, path := range []string{"/api/pairing/start", "/api/devices"} {
		method := "POST"
		if path == "/api/devices" {
			method = "GET"
		}
		if c, _ := p.call(laptop, method, path, device, nil); c != http.StatusForbidden {
			t.Errorf("a paired device used %s: %d", path, c)
		}
	}
	if c, _ := p.call(laptop, "POST", "/api/auth/signup", device, map[string]string{"name": "x", "email": "x@y.zz", "password": "another long password"}); c != http.StatusForbidden {
		t.Fatalf("a paired device created an account: %d", c)
	}
	if c, _ := p.call(laptop, "POST", "/api/host/shutdown", device, nil); c != http.StatusForbidden {
		t.Fatalf("a paired device reached shutdown: %d", c)
	}
}

func TestACodeWorksOnceOnly(t *testing.T) {
	p, owner := remoteServer(t, true)
	_, start := p.call(here, "POST", "/api/pairing/start", owner, nil)
	body := map[string]string{"code": start["code"].(string), "name": "first"}

	if c, _ := p.call(laptop, "POST", "/api/pairing/claim", "", body); c != http.StatusOK {
		t.Fatalf("first use: %d", c)
	}
	body["name"] = "second"
	c, again := p.call("192.168.1.99:40000", "POST", "/api/pairing/claim", "", body)
	if c != http.StatusForbidden || !strings.Contains(again["error"].(string), "not valid") {
		t.Fatalf("a used code worked again: %d %v", c, again)
	}
}

func TestEveryWrongCodeSaysTheSameThing(t *testing.T) {
	p, owner := remoteServer(t, true)
	_, start := p.call(here, "POST", "/api/pairing/start", owner, nil)
	_ = start
	var seen []string
	for _, code := range []string{"", "x", "ZZZZ-ZZZZ", "BCDF-GHJK", "not even close at all"} {
		c, body := p.call("192.168.1.7"+string(rune('0'+len(seen)))+":1", "POST", "/api/pairing/claim", "", map[string]string{"code": code})
		if c != http.StatusForbidden {
			t.Fatalf("%q: %d", code, c)
		}
		seen = append(seen, body["error"].(string))
	}
	for _, msg := range seen[1:] {
		if msg != seen[0] {
			t.Fatalf("different wrong codes got different answers, which tells a guesser something: %q vs %q", msg, seen[0])
		}
	}
}

func TestGuessingCodesIsRateLimited(t *testing.T) {
	p, _ := remoteServer(t, true)
	var last int
	for i := 0; i < 8; i++ {
		last, _ = p.call(laptop, "POST", "/api/pairing/claim", "", map[string]string{"code": "BCDF-GHJK"})
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("the eighth attempt in a minute from one address got %d, want 429", last)
	}
	// Another address is not locked out by it...
	if c, _ := p.call("192.168.1.77:1", "POST", "/api/pairing/claim", "", map[string]string{"code": "BCDF-GHJK"}); c != http.StatusForbidden {
		t.Fatalf("a different address got %d", c)
	}
}

func TestRemovingADeviceSignsItOutAtOnce(t *testing.T) {
	p, owner := remoteServer(t, true)
	_, start := p.call(here, "POST", "/api/pairing/start", owner, nil)
	_, claimed := p.call(laptop, "POST", "/api/pairing/claim", "", map[string]string{"code": start["code"].(string), "name": "Laptop"})
	device := claimed["token"].(string)
	id := claimed["device"].(map[string]any)["id"].(string)

	c, list := p.call(here, "GET", "/api/devices", owner, nil)
	devices, _ := list["devices"].([]any)
	if c != http.StatusOK || len(devices) != 1 {
		t.Fatalf("list: %d %v", c, list)
	}
	if c, _ := p.call(laptop, "GET", "/api/chats", device, nil); c != http.StatusOK {
		t.Fatalf("before removing: %d", c)
	}

	if c, _ := p.call(here, "DELETE", "/api/devices/"+id, owner, nil); c != http.StatusOK {
		t.Fatalf("remove: %d", c)
	}
	if c, _ := p.call(laptop, "GET", "/api/chats", device, nil); c != http.StatusForbidden {
		t.Fatalf("a removed device still has access: %d", c)
	}
	if c, _ := p.call(here, "DELETE", "/api/devices/"+id, owner, nil); c != http.StatusNotFound {
		t.Fatalf("removing twice: %d", c)
	}
	// And the ledger recorded both events.
	_, ledger := p.call(here, "GET", "/api/ledger", owner, nil)
	raw, _ := json.Marshal(ledger)
	if !strings.Contains(string(raw), "Paired a device") || !strings.Contains(string(raw), "Removed a paired device") {
		t.Fatalf("pairing is not in the ledger: %s", raw)
	}
}

func TestAnotherAccountCannotRemoveMyDevice(t *testing.T) {
	p, owner := remoteServer(t, true)
	_, start := p.call(here, "POST", "/api/pairing/start", owner, nil)
	_, claimed := p.call(laptop, "POST", "/api/pairing/claim", "", map[string]string{"code": start["code"].(string), "name": "Laptop"})
	id := claimed["device"].(map[string]any)["id"].(string)

	_, other := p.call(here, "POST", "/api/auth/signup", "", map[string]string{"name": "Bob", "email": "bob@example.com", "password": "another long password"})
	if c, _ := p.call(here, "DELETE", "/api/devices/"+id, other["token"].(string), nil); c != http.StatusNotFound {
		t.Fatalf("another account removed the device: %d", c)
	}
	if c, _ := p.call(laptop, "GET", "/api/chats", claimed["token"].(string), nil); c != http.StatusOK {
		t.Fatal("the device lost access to someone else's removal attempt")
	}
}

func TestADeviceNameCannotCarryTerminalControls(t *testing.T) {
	p, owner := remoteServer(t, true)
	_, start := p.call(here, "POST", "/api/pairing/start", owner, nil)
	_, claimed := p.call(laptop, "POST", "/api/pairing/claim", "", map[string]string{
		"code": start["code"].(string), "name": "evil\x1b]0;pwned\x07\u202ename  with   gaps\n" + strings.Repeat("x", 200)})
	name := claimed["device"].(map[string]any)["name"].(string)
	if strings.ContainsAny(name, "\x1b\x07\n\u202e") || len([]rune(name)) > 60 {
		t.Fatalf("name = %q", name)
	}
}

func TestAttemptLimiter(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newAttemptLimiter(3, time.Minute)
	l.now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		if !l.Allow("a") {
			t.Fatalf("attempt %d refused", i+1)
		}
	}
	if l.Allow("a") {
		t.Fatal("the fourth attempt was allowed")
	}
	if !l.Allow("b") {
		t.Fatal("another key was limited")
	}
	now = now.Add(61 * time.Second)
	if !l.Allow("a") {
		t.Fatal("the window did not reset")
	}
}
