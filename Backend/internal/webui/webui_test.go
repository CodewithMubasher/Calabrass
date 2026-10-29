package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

var site = fstest.MapFS{
	"index.html":        {Data: []byte("<html>shell</html>")},
	"assets/app-abc.js": {Data: []byte("console.log(1)")},
	"favicon.svg":       {Data: []byte("<svg/>")},
}

func get(h http.Handler, method, p string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, p, nil))
	return rec
}

func TestServesFilesAndRoutes(t *testing.T) {
	h := Handler(site)
	if r := get(h, "GET", "/"); r.Code != 200 || !strings.Contains(r.Body.String(), "shell") {
		t.Fatalf("/: %d %q", r.Code, r.Body.String())
	}
	if r := get(h, "GET", "/chat/abc123"); r.Code != 200 || !strings.Contains(r.Body.String(), "shell") {
		t.Fatalf("an app route must get the shell: %d", r.Code)
	}
	r := get(h, "GET", "/assets/app-abc.js")
	if r.Code != 200 || !strings.Contains(r.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: %d %q", r.Code, r.Header().Get("Cache-Control"))
	}
	if r := get(h, "GET", "/"); r.Header().Get("Cache-Control") != "no-cache" {
		t.Fatal("the shell must not be cached")
	}
	if r := get(h, "GET", "/assets/missing.js"); r.Code != 404 {
		t.Fatalf("a missing asset must be a 404, not the shell: %d", r.Code)
	}
	if r := get(h, "POST", "/"); r.Code != 405 {
		t.Fatalf("POST: %d", r.Code)
	}
	if r := get(h, "GET", "/"); r.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("missing framing protection")
	}
}

func TestPathTraversalStaysInside(t *testing.T) {
	h := Handler(site)
	if r := get(h, "GET", "/../../etc/passwd"); strings.Contains(r.Body.String(), "root:") {
		t.Fatal("escaped the web root")
	}
}

func TestMountRoutesAPIAndUI(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("api")) })
	m := Mount(api, Handler(site))
	if r := get(m, "GET", "/api/health"); r.Body.String() != "api" {
		t.Fatal("api not routed")
	}
	if r := get(m, "GET", "/schedules"); !strings.Contains(r.Body.String(), "shell") {
		t.Fatal("ui not routed")
	}
	none := Mount(api, nil)
	if r := get(none, "GET", "/"); r.Code != 404 || !strings.Contains(r.Body.String(), "no web interface") {
		t.Fatalf("a build without the interface must say so: %d %q", r.Code, r.Body.String())
	}
	if r := get(none, "GET", "/api/x"); r.Body.String() != "api" {
		t.Fatal("api must still work without the interface")
	}
}

func TestEmbeddedIsAbsentUntilBuilt(t *testing.T) {
	// The placeholder directory has no index.html until the frontend is copied in.
	if _, ok := Embedded(); ok {
		t.Skip("a built frontend is embedded in this checkout")
	}
}
