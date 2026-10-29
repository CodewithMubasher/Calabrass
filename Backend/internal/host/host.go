// Package host is how one Calabrass host per machine is found and shared.
//
// The host is the backend process. It owns the data, the keys, the schedules
// and the approvals. Desktop, CLI and web are faces: they look for a running
// host, attach to it, and leave it running when they close. When there is none
// the first face to need one starts it.
//
// The host announces itself in host.json inside the data folder: where it
// listens, who started it, and a secret that only a process able to read the
// owner's data folder can know. That secret is what lets the CLI sign in as the
// machine's owner without a password, and what lets `stop` shut the host down.
package host

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/proc"
)

// FileName is the announcement file in the data folder.
const FileName = "host.json"

// SecretHeader carries the host secret on calls made without a login, such as
// asking the host to stop.
const SecretHeader = "X-Calabrass-Host-Secret"

// Owners say what started the host, which decides what happens when it ends.
const (
	// OwnerDaemon is a host that runs on its own, in the background. It is what
	// makes schedules run when no window is open, and it ends only when asked.
	OwnerDaemon = "daemon"
	// OwnerDesktop is a host that lives inside the desktop app and ends with it.
	OwnerDesktop = "desktop"
	// OwnerServer is a host started by hand (`calabrass-server`).
	OwnerServer = "server"
)

// Info is what a running host announces.
type Info struct {
	Addr      string    `json:"addr"`
	PID       int       `json:"pid"`
	Version   string    `json:"version"`
	Owner     string    `json:"owner"`
	StartedAt time.Time `json:"startedAt"`
	// Secret proves the caller can read this user's data folder. Never send it
	// anywhere but back to this host over loopback.
	Secret string `json:"secret"`
}

// Public is the part of Info that is safe to show anyone who can reach the
// host: everything but the secret.
type Public struct {
	Addr      string    `json:"addr"`
	PID       int       `json:"pid"`
	Version   string    `json:"version"`
	Owner     string    `json:"owner"`
	StartedAt time.Time `json:"startedAt"`
	// Background is true when the host keeps running after its faces close.
	Background bool `json:"background"`
}

// Public strips the secret.
func (i Info) Public() Public {
	return Public{
		Addr: i.Addr, PID: i.PID, Version: i.Version, Owner: i.Owner,
		StartedAt: i.StartedAt, Background: i.Owner == OwnerDaemon,
	}
}

// NewSecret returns a fresh random secret.
func NewSecret() string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		// Reading from the system's random source does not fail in practice; if
		// it ever does, a host with no secret is better than one with a weak one.
		panic("host: no randomness available: " + err.Error())
	}
	return hex.EncodeToString(raw)
}

// Path is where the announcement lives.
func Path(dataDir string) string { return filepath.Join(dataDir, FileName) }

// Write announces a host. The file is written whole and then moved into place,
// so a reader never sees half of it.
func Write(dataDir string, info Info) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	tmp := Path(dataDir) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, Path(dataDir))
}

// Read returns the announcement, or os.ErrNotExist.
func Read(dataDir string) (Info, error) {
	data, err := os.ReadFile(Path(dataDir))
	if err != nil {
		return Info{}, err
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return Info{}, fmt.Errorf("host: %s is damaged: %w", FileName, err)
	}
	return info, nil
}

// Remove withdraws the announcement, but only if it is still this process's.
// A host that crashed and was replaced must not remove its successor's file.
func Remove(dataDir string, pid int) {
	info, err := Read(dataDir)
	if err != nil || info.PID != pid {
		return
	}
	_ = os.Remove(Path(dataDir))
}

// DialAddr turns a listen address into one a client on this machine can dial:
// an unspecified host becomes loopback.
func DialAddr(addr string) string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// Healthy reports whether a Calabrass host answers at addr.
func Healthy(ctx context.Context, addr string) bool {
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+DialAddr(addr)+"/api/health", nil)
	if err != nil {
		return false
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return false
	}
	var body struct {
		Status string `json:"status"`
	}
	return json.NewDecoder(res.Body).Decode(&body) == nil && body.Status == "ok"
}

// Discover finds the running host for this data folder. A file left behind by a
// host that is gone is not a host: it is reported as absent.
func Discover(ctx context.Context, dataDir string) (Info, bool) {
	info, err := Read(dataDir)
	if err != nil || info.Addr == "" {
		return Info{}, false
	}
	if !Healthy(ctx, info.Addr) {
		return Info{}, false
	}
	return info, true
}

// ErrAlreadyRunning means another host owns this data folder.
type ErrAlreadyRunning struct{ Info Info }

func (e *ErrAlreadyRunning) Error() string {
	return fmt.Sprintf("Calabrass is already running (pid %d, %s, started by %s). "+
		"Use that one: every window and terminal attaches to it.", e.Info.PID, e.Info.Addr, e.Info.Owner)
}

// IsAlreadyRunning unwraps an ErrAlreadyRunning.
func IsAlreadyRunning(err error) (*ErrAlreadyRunning, bool) {
	var e *ErrAlreadyRunning
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// Claim announces this process as the host, unless a healthy one already is.
func Claim(ctx context.Context, dataDir string, info Info) error {
	if existing, ok := Discover(ctx, dataDir); ok && existing.PID != info.PID {
		return &ErrAlreadyRunning{Info: existing}
	}
	if info.Secret == "" {
		info.Secret = NewSecret()
	}
	if info.StartedAt.IsZero() {
		info.StartedAt = time.Now()
	}
	return Write(dataDir, info)
}

// Stop asks a running host to stop, proving the right to with its secret, and
// waits until it no longer answers.
func Stop(ctx context.Context, info Info) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+DialAddr(info.Addr)+"/api/host/shutdown", nil)
	if err != nil {
		return err
	}
	req.Header.Set(SecretHeader, info.Secret)
	res, err := (&http.Client{Transport: &http.Transport{Proxy: nil}}).Do(req)
	if err != nil {
		return fmt.Errorf("could not reach the host: %w", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("the host refused to stop: %s", res.Status)
	}
	for ctx.Err() == nil {
		if !Healthy(ctx, info.Addr) {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return errors.New("asked the host to stop, but it is still answering")
}

// BinaryNames are what the background host's executable may be called, beside
// the app that starts it.
func BinaryNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"calabrass-backend.exe", "calabrass-server.exe"}
	}
	return []string{"calabrass-backend", "calabrass-server"}
}

// FindBinary looks for the background host's executable in dir.
func FindBinary(dir string) (string, bool) {
	for _, name := range BinaryNames() {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
	}
	return "", false
}

// Spawn starts a background host that outlives the process starting it, and
// waits until it answers. Its output goes to host.log in the data folder.
func Spawn(ctx context.Context, exe, dataDir string, wait time.Duration) (Info, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return Info{}, err
	}
	logFile, err := os.OpenFile(filepath.Join(dataDir, "host.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return Info{}, err
	}
	defer func() { _ = logFile.Close() }()

	cmd := exec.Command(exe, "--daemon")
	cmd.Stdout, cmd.Stderr = logFile, logFile
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		return Info{}, fmt.Errorf("could not start the background host: %w", err)
	}
	// The host is its own process now; release it so it is not a zombie of ours.
	go func() { _ = cmd.Wait() }()

	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if info, ok := Discover(ctx, dataDir); ok {
			return info, nil
		}
		select {
		case <-ctx.Done():
			return Info{}, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	return Info{}, errors.New("the background host did not come up in time; see host.log in the data folder")
}
