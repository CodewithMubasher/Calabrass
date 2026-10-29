package netguard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsLoopbackAddr(t *testing.T) {
	yes := []string{"127.0.0.1:8787", "localhost:8787", "[::1]:8787", "127.0.0.5:1"}
	no := []string{":8787", "0.0.0.0:8787", "192.168.1.5:8787", "[::]:8787", "example.com:80", ""}
	for _, a := range yes {
		if !IsLoopbackAddr(a) {
			t.Errorf("%q should be loopback", a)
		}
	}
	for _, a := range no {
		if IsLoopbackAddr(a) {
			t.Errorf("%q should not be loopback", a)
		}
	}
}

func TestValidateBind(t *testing.T) {
	if err := ValidateBind("127.0.0.1:8787", ""); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBind("0.0.0.0:8787", ""); err == nil {
		t.Fatal("open bind without pairing must be refused")
	}
	if err := ValidateBind("0.0.0.0:8787", "short"); err == nil {
		t.Fatal("a short pairing code must be refused")
	}
	if err := ValidateBind("0.0.0.0:8787", "a-long-enough-pairing-secret"); err != nil {
		t.Fatal(err)
	}
}

func TestIsLocalOrigin(t *testing.T) {
	yes := []string{"http://localhost:5173", "http://127.0.0.1:8787", "http://[::1]:3000",
		"http://wails.localhost", "https://wails.localhost", "wails://wails"}
	no := []string{"", "https://evil.com", "http://localhost.evil.com", "http://127.0.0.1.evil.com:80",
		"http://localhostevil.com"}
	for _, o := range yes {
		if !IsLocalOrigin(o) {
			t.Errorf("%q should be local", o)
		}
	}
	for _, o := range no {
		if IsLocalOrigin(o) {
			t.Errorf("%q should not be local", o)
		}
	}
}

func run(h http.Handler, method, path, remote string, hdr map[string]string) int {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remote
	req.Host = "127.0.0.1:8787"
	for k, v := range hdr {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

var ok = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })

func TestGuardLoopbackOnly(t *testing.T) {
	g := Guard(ok, true, "")
	if c := run(g, "GET", "/api/chats", "127.0.0.1:5000", nil); c != 200 {
		t.Fatalf("local request: %d", c)
	}
	if c := run(g, "GET", "/api/chats", "10.0.0.7:5000", nil); c != 403 {
		t.Fatalf("remote request without pairing must be refused, got %d", c)
	}
	if c := run(g, "GET", "/api/health", "10.0.0.7:5000", nil); c != 200 {
		t.Fatalf("health stays public, got %d", c)
	}
}

func TestGuardRefusesBrowserPagesAndRebinding(t *testing.T) {
	g := Guard(ok, true, "")
	if c := run(g, "POST", "/api/auth/signup", "127.0.0.1:5000", map[string]string{"Origin": "https://evil.com"}); c != 403 {
		t.Fatalf("a foreign page must be refused, got %d", c)
	}
	if c := run(g, "POST", "/api/auth/signup", "127.0.0.1:5000", map[string]string{"Origin": "http://localhost:5173"}); c != 200 {
		t.Fatalf("the dev server must be allowed, got %d", c)
	}
	if c := run(g, "GET", "/api/chats", "127.0.0.1:5000", map[string]string{"Host": "attacker.example:8787"}); c != 403 {
		t.Fatalf("a rebound host name must be refused, got %d", c)
	}
}

func TestGuardPairing(t *testing.T) {
	code := "a-long-enough-pairing-secret"
	g := Guard(ok, false, code)
	remote := "10.0.0.7:5000"
	if c := run(g, "GET", "/api/chats", remote, nil); c != 403 {
		t.Fatalf("no code: %d", c)
	}
	if c := run(g, "GET", "/api/chats", remote, map[string]string{PairingHeader: "wrong"}); c != 403 {
		t.Fatalf("wrong code: %d", c)
	}
	if c := run(g, "GET", "/api/chats", remote, map[string]string{PairingHeader: code}); c != 200 {
		t.Fatalf("right code: %d", c)
	}
	if c := run(g, "GET", "/api/artifacts/stream?pairing="+code, remote, nil); c != 200 {
		t.Fatalf("code in query: %d", c)
	}
	// Even a paired device may not create accounts or use the owner sign-in.
	for _, p := range []string{"/api/auth/signup", "/api/auth/local"} {
		if c := run(g, "POST", p, remote, map[string]string{PairingHeader: code}); c != 403 {
			t.Fatalf("%s from a paired remote must be refused, got %d", p, c)
		}
		if c := run(g, "POST", p, "127.0.0.1:5000", nil); c != 200 {
			t.Fatalf("%s from the machine itself must work, got %d", p, c)
		}
	}
}
