package host

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func healthServer(t *testing.T) (string, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	return strings.TrimPrefix(srv.URL, "http://"), srv.Close
}

func TestWriteReadRemove(t *testing.T) {
	dir := t.TempDir()
	in := Info{Addr: "127.0.0.1:8787", PID: 42, Version: "1.2.3", Owner: OwnerDaemon, Secret: "s"}
	if err := Write(dir, in); err != nil {
		t.Fatal(err)
	}
	got, err := Read(dir)
	if err != nil || got.PID != 42 || got.Secret != "s" || got.Owner != OwnerDaemon {
		t.Fatalf("read = %+v, %v", got, err)
	}
	if info, err := os.Stat(Path(dir)); err == nil && info.Mode().Perm()&0o077 != 0 && !isWindows() {
		t.Fatalf("host.json is readable by others: %v", info.Mode())
	}
	Remove(dir, 99) // someone else's pid: leave it
	if _, err := Read(dir); err != nil {
		t.Fatal("a different pid removed the announcement")
	}
	Remove(dir, 42)
	if _, err := Read(dir); err == nil {
		t.Fatal("the announcement was not removed")
	}
}

func isWindows() bool { return os.PathSeparator == '\\' }

func TestPublicHidesTheSecret(t *testing.T) {
	p := Info{Secret: "topsecret", Owner: OwnerDaemon}.Public()
	if !p.Background {
		t.Fatal("a daemon host is a background host")
	}
	if (Info{Owner: OwnerDesktop}).Public().Background {
		t.Fatal("a desktop-owned host ends with the app")
	}
}

func TestDiscoverAndStaleFile(t *testing.T) {
	dir := t.TempDir()
	if _, ok := Discover(context.Background(), dir); ok {
		t.Fatal("found a host in an empty folder")
	}
	// A file left behind by a host that is gone is not a host.
	_ = Write(dir, Info{Addr: "127.0.0.1:1", PID: 1})
	if _, ok := Discover(context.Background(), dir); ok {
		t.Fatal("a stale file was treated as a running host")
	}
	addr, stop := healthServer(t)
	defer stop()
	_ = Write(dir, Info{Addr: addr, PID: 7, Owner: OwnerDaemon})
	if info, ok := Discover(context.Background(), dir); !ok || info.PID != 7 {
		t.Fatalf("a live host was not found: %+v %v", info, ok)
	}
}

func TestClaimRefusesASecondHost(t *testing.T) {
	dir := t.TempDir()
	addr, stop := healthServer(t)
	defer stop()
	_ = Write(dir, Info{Addr: addr, PID: 7, Owner: OwnerDaemon})

	err := Claim(context.Background(), dir, Info{Addr: "127.0.0.1:9999", PID: 8, Owner: OwnerServer})
	if e, ok := IsAlreadyRunning(err); !ok || e.Info.PID != 7 {
		t.Fatalf("a second host was allowed: %v", err)
	}
	// The same process may re-announce itself.
	if err := Claim(context.Background(), dir, Info{Addr: addr, PID: 7, Owner: OwnerDaemon}); err != nil {
		t.Fatal(err)
	}
	// Once the first is gone, a new one may take over and gets a secret.
	stop()
	if err := Claim(context.Background(), dir, Info{Addr: "127.0.0.1:9999", PID: 8, Owner: OwnerServer}); err != nil {
		t.Fatal(err)
	}
	got, _ := Read(dir)
	if got.PID != 8 || len(got.Secret) < 32 || got.StartedAt.IsZero() {
		t.Fatalf("claim wrote %+v", got)
	}
}

func TestDialAddr(t *testing.T) {
	for in, want := range map[string]string{
		":8787": "127.0.0.1:8787", "0.0.0.0:8787": "127.0.0.1:8787", "127.0.0.1:1": "127.0.0.1:1",
	} {
		if got := DialAddr(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestStopNeedsTheSecretAndWaitsForTheHostToGo(t *testing.T) {
	var alive atomic.Bool
	alive.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			if !alive.Load() {
				http.Error(w, "down", http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/host/shutdown":
			if r.Header.Get(SecretHeader) != "right" {
				http.Error(w, "no", http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(`{"stopping":true}`))
			go func() { time.Sleep(100 * time.Millisecond); alive.Store(false) }()
		}
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	if err := Stop(context.Background(), Info{Addr: addr, Secret: "wrong"}); err == nil {
		t.Fatal("a wrong secret stopped the host")
	}
	if !alive.Load() {
		t.Fatal("the host went down on a wrong secret")
	}
	if err := Stop(context.Background(), Info{Addr: addr, Secret: "right"}); err != nil {
		t.Fatal(err)
	}
	if alive.Load() {
		t.Fatal("Stop returned before the host was down")
	}
}

func TestFindBinary(t *testing.T) {
	dir := t.TempDir()
	if _, ok := FindBinary(dir); ok {
		t.Fatal("found a binary in an empty folder")
	}
	name := BinaryNames()[0]
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, ok := FindBinary(dir); !ok || filepath.Base(got) != name {
		t.Fatalf("got %q %v", got, ok)
	}
}
