// Package hostctl starts the Calabrass host when it is not running.
//
// The launcher starts the host before the terminal UI opens, so normally this is
// never needed. It is for the day the host stops while the UI is open (it was
// stopped from another window, the machine slept, it crashed): the UI notices,
// and instead of asking the person to go and fix it, starts the host again.
package hostctl

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"calabrass-tui/internal/hostfile"
)

// EnvBinary names the host program. The launcher sets it for the UI it starts.
const EnvBinary = "CALABRASS_BACKEND_BIN"

// Binary returns the host program the launcher gave this process, if it is real.
func Binary() (string, bool) {
	bin := strings.TrimSpace(os.Getenv(EnvBinary))
	if bin == "" {
		return "", false
	}
	// Only ever start the host's own program, never an arbitrary one that an
	// environment variable happens to name.
	base := strings.ToLower(filepath.Base(bin))
	if !strings.HasPrefix(base, "calabrass-backend") && !strings.HasPrefix(base, "calabrass-server") {
		return "", false
	}
	if info, err := os.Stat(bin); err != nil || info.IsDir() {
		return "", false
	}
	return bin, true
}

// IsLocal reports whether addr is this machine: only then is the host ours to
// start. A host on another machine is somebody else's to run.
func IsLocal(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		host = strings.TrimSpace(addr)
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Healthy reports whether a Calabrass host answers at addr.
func Healthy(ctx context.Context, addr string) bool {
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/api/health", nil)
	if err != nil {
		return false
	}
	res, err := (&http.Client{Transport: &http.Transport{Proxy: nil}}).Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = res.Body.Close() }()
	return res.StatusCode == http.StatusOK
}

// Start launches the host as a background program that outlives this one, and
// waits until it answers. Its output goes to host.log, never to this terminal.
func Start(ctx context.Context, bin, addr string, wait time.Duration) error {
	if Healthy(ctx, addr) {
		return nil // somebody else already started it
	}
	dir := hostfile.DataDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(dir, "host.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = logFile.Close() }()

	cmd := exec.Command(bin, "--daemon")
	cmd.Stdin = nil
	cmd.Stdout, cmd.Stderr = logFile, logFile
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // not a zombie of ours

	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if Healthy(ctx, addr) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	return errors.New("the host did not come up")
}
