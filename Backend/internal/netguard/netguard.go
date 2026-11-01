// Package netguard keeps Calabrass on the machine it runs on unless the owner
// has deliberately opened it up.
//
// The agent can run commands, so reaching the API is as good as reaching a
// shell. Three rules follow:
//
//  1. The listen address must be loopback, unless pairing is on.
//  2. Pairing means every request from another machine carries the pairing
//     code. Without the code it is refused before any account is consulted.
//  3. Accounts can only be created, and the machine-owner sign-in can only be
//     used, from the machine itself.
//
// Independently of where it listens, a request that carries an Origin or Host
// the app never uses is refused, which is what stops a web page from driving
// the local API.
package netguard

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// MinPairingCodeLength is the shortest pairing code accepted. It is a shared
// secret that gates a shell, so it has to be long enough not to be guessed.
const MinPairingCodeLength = 16

// PairingHeader carries the pairing code on requests from other machines.
const PairingHeader = "X-Calabrass-Pairing"

// IsLoopbackAddr reports whether a listen address is reachable only from this
// machine. An empty host ("" or ":8787") listens on every interface and is not
// loopback.
func IsLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		host = strings.TrimSpace(addr)
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ErrRemoteWithoutPairing is returned when the address is open to the network
// and no pairing code is set.
var ErrRemoteWithoutPairing = errors.New(
	"Calabrass will not listen beyond this machine unless devices can be paired. " +
		"Set CALABRASS_ALLOW_REMOTE=1 (then `cal pair` makes a one-time code for each device), " +
		"or listen on 127.0.0.1")

// ValidateBind refuses an unsafe listen configuration.
func ValidateBind(addr, pairingCode string) error { return ValidateBindWith(addr, pairingCode, false) }

// ValidateBindWith is ValidateBind for a host that may also pair devices
// (allowRemote): that is "pairing on" as much as a shared code is.
func ValidateBindWith(addr, pairingCode string, allowRemote bool) error {
	if IsLoopbackAddr(addr) || allowRemote {
		return nil
	}
	if len(strings.TrimSpace(pairingCode)) < MinPairingCodeLength {
		return fmt.Errorf("%w (listen address %q)", ErrRemoteWithoutPairing, addr)
	}
	return nil
}

// IsLoopbackRequest reports whether a request came from this machine.
func IsLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// IsLocalOrigin reports whether an Origin header names a page the app itself
// serves: the dev server on any local port, or the desktop shell.
func IsLocalOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	for _, prefix := range []string{
		"http://localhost:", "http://localhost",
		"http://127.0.0.1:", "http://127.0.0.1",
		"http://[::1]:", "http://[::1]",
		"http://wails.localhost", "https://wails.localhost", "wails://",
	} {
		if strings.HasPrefix(origin, prefix) {
			// "http://localhost.evil.com" starts with "http://localhost".
			rest := strings.TrimPrefix(origin, prefix)
			if prefix == "wails://" || rest == "" || rest[0] == ':' || rest[0] == '/' || strings.HasSuffix(prefix, ":") {
				return true
			}
		}
	}
	return false
}

func isLocalHostHeader(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	h = strings.Trim(strings.ToLower(h), "[]")
	switch h {
	case "localhost", "127.0.0.1", "::1", "wails.localhost":
		return true
	}
	if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}

// Guard wraps the whole API.
//
// listenOnLoopback says how the server was bound; pairingCode is empty unless
// the owner turned pairing on.
func Guard(next http.Handler, listenOnLoopback bool, pairingCode string) http.Handler {
	return GuardWith(next, listenOnLoopback, Options{PairingCode: pairingCode})
}

// Options says how other machines may reach the host.
type Options struct {
	// PairingCode is a shared secret every request from another machine may carry
	// instead of being a paired device. It is the older way; a device is better,
	// since it can be named and revoked.
	PairingCode string
	// AllowRemote lets a machine that is not this one claim a pairing code.
	AllowRemote bool
	// DeviceOK says whether a request carries the session of a paired device.
	DeviceOK func(r *http.Request) bool
}

// machineOnly are the paths that only the machine the host runs on may use, even
// for a paired device: they decide who else gets in.
func machineOnly(path string) bool {
	return path == "/api/pairing/start" || path == "/api/devices" || strings.HasPrefix(path, "/api/devices/") ||
		path == "/api/host/shutdown"
}

// GuardWith is Guard with the options a host that pairs devices needs.
func GuardWith(next http.Handler, listenOnLoopback bool, opts Options) http.Handler {
	pairingCode := strings.TrimSpace(opts.PairingCode)
	deviceOK := func(r *http.Request) bool { return opts.DeviceOK != nil && opts.DeviceOK(r) }
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		local := IsLoopbackRequest(r)

		// 1. A page in the user's browser must not be able to drive the API,
		//    whether by rebinding a hostname to 127.0.0.1 or by posting a form.
		if origin := r.Header.Get("Origin"); origin != "" && !IsLocalOrigin(origin) && !pairedOrigin(r, pairingCode) && !deviceOK(r) {
			refuse(w, http.StatusForbidden, "this page is not allowed to use the local Calabrass API")
			return
		}
		if listenOnLoopback && !isLocalHostHeader(r.Host) {
			refuse(w, http.StatusForbidden, "unexpected host name")
			return
		}

		if local {
			next.ServeHTTP(w, r)
			return
		}

		// 2. Another machine: it must be paired, and it may not create accounts
		//    or use the machine-owner sign-in.
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		if machineOnly(r.URL.Path) {
			refuse(w, http.StatusForbidden, "that can only be done on the machine Calabrass runs on")
			return
		}
		// The one thing a machine that is not paired yet may do: trade a one-time
		// code for a device session.
		if opts.AllowRemote && r.URL.Path == "/api/pairing/claim" && r.Method == http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		paired := (pairingCode != "" && matchesPairing(r, pairingCode)) || deviceOK(r)
		if !paired {
			refuse(w, http.StatusForbidden, "this Calabrass only accepts paired devices")
			return
		}
		// A paired device is already signed in. It never creates accounts or
		// signs in some other way.
		switch r.URL.Path {
		case "/api/auth/signup", "/api/auth/local", "/api/auth/login":
			refuse(w, http.StatusForbidden, "accounts can only be created or signed in on the machine Calabrass runs on")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// pairedOrigin lets a paired remote page through the Origin check: it still has
// to show the code, which a hostile page cannot know.
func pairedOrigin(r *http.Request, pairingCode string) bool {
	return pairingCode != "" && matchesPairing(r, pairingCode)
}

func matchesPairing(r *http.Request, code string) bool {
	got := r.Header.Get(PairingHeader)
	if got == "" {
		// EventSource and WebSocket cannot set headers, so streams may carry
		// the code in the query, as they already do for the session token.
		got = r.URL.Query().Get("pairing")
	}
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(code)) == 1
}

func refuse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "{\"error\":%q}\n", message)
}

// OriginHost is a helper for logs: the host part of an Origin header.
func OriginHost(origin string) string {
	u, err := url.Parse(origin)
	if err != nil {
		return origin
	}
	return u.Host
}
