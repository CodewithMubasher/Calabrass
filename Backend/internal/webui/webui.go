// Package webui lets the host serve the web interface itself, so "the web
// version" is just a page on the host: nothing to install, no second server,
// and the page and the API always come from the same place and the same build.
//
// The built frontend is copied into dist/ before the backend is compiled (see
// scripts/build-backend.ps1 and the release workflow). A backend built without
// it still works; Embedded then reports false and the API simply has no page.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Embedded returns the built interface, and whether one was compiled in.
func Embedded() (fs.FS, bool) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, false
	}
	return sub, true
}

// Handler serves a single-page app from fsys. A path that names a file is that
// file; a path that does not, and has no extension, is a route inside the app
// and gets index.html; a missing asset is a plain 404, so a typo in a script
// path never comes back as an HTML page that the browser tries to run.
func Handler(fsys fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}

		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")

		info, err := fs.Stat(fsys, name)
		if err != nil || info.IsDir() {
			if path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
			name = "index.html"
		}

		if name == "index.html" {
			// The shell must always be fetched fresh: it names the hashed
			// assets of the current build.
			h.Set("Cache-Control", "no-cache")
		} else if strings.HasPrefix(name, "assets/") {
			// Build output is named by content hash and never changes.
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeFileFS(w, r, fsys, name)
	})
}

// Mount returns a handler that sends /api/* to api and everything else to the
// interface. With no interface, everything else is a short explanation.
func Mount(api http.Handler, ui http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			api.ServeHTTP(w, r)
			return
		}
		if ui == nil {
			http.Error(w,
				"Calabrass is running, but this build has no web interface compiled in. "+
					"Use the desktop app or the `cal` command, or rebuild with the frontend.",
				http.StatusNotFound)
			return
		}
		ui.ServeHTTP(w, r)
	})
}
