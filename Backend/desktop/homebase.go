package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/CodewithMubasher/Calabrass/backend/internal/host"
	"github.com/CodewithMubasher/Calabrass/backend/internal/netguard"
)

// Home Base is what only the desktop app can be: present on the machine when the
// window is not in front of you.
//
//   - It watches what the agent is waiting on you to allow, and when the window
//     is minimised it puts the question in a native dialog and a notification, so
//     an unattended-looking job is never silently stuck.
//   - It tells you when a scheduled or headless run finishes, fails or is missed.
//   - Dropping a folder on the window opens a chat that works in it.
//   - A global hotkey (Windows) brings the window back from anywhere.
//
// It talks to the host the way any face does: signed in as the machine's owner
// through the host secret, over loopback. It never holds the host's data.

const (
	// openFolderEvent is what the window's page listens for.
	openFolderEvent = "calabrass:folder-dropped"
	approvalPoll    = 4 * time.Second
	runPoll         = 20 * time.Second
)

type homeBase struct {
	d      *desktop
	ctx    context.Context
	base   string
	secret string
	http   *http.Client

	mu       sync.Mutex
	token    string
	notified map[string]bool
	lastRun  int64
}

// startHomeBase starts the watchers. It returns a function that stops them. When
// there is no host to talk to it does nothing: Home Base is an extra, never a
// reason for the window not to open.
func (d *desktop) startHomeBase(ctx context.Context) func() {
	info, ok := d.hostInfoForHomeBase()
	if !ok {
		d.log.Info("home base is off: no host secret to sign in with")
		return func() {}
	}

	hb := &homeBase{
		d:        d,
		ctx:      ctx,
		base:     "http://" + host.DialAddr(info.Addr),
		secret:   info.Secret,
		http:     &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}},
		notified: map[string]bool{},
		lastRun:  time.Now().UnixMilli(),
	}

	if err := runtime.InitializeNotifications(ctx); err != nil {
		d.log.Warn("notifications are unavailable", "error", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	go hb.watchApprovals(runCtx)
	go hb.watchRuns(runCtx)
	stopHotkey := registerShowHotkey(d.log, func() { hb.show() })

	runtime.OnFileDrop(ctx, func(_, _ int, paths []string) { hb.folderDropped(paths) })

	return func() {
		cancel()
		stopHotkey()
		runtime.OnFileDropOff(ctx)
		runtime.CleanupNotifications(ctx)
	}
}

// hostInfoForHomeBase finds the host this window is a face of, with its secret.
func (d *desktop) hostInfoForHomeBase() (host.Info, bool) {
	switch {
	case d.attached.PID != 0 && d.attached.Secret != "":
		return d.attached, true
	case d.backend != nil && d.backend.Host.Secret != "":
		return d.backend.Host, true
	}
	if info, err := host.Read(d.dataDir); err == nil && info.Secret != "" && info.Addr != "" {
		return info, true
	}
	return host.Info{}, false
}

// show brings the window to the front, from minimised or hidden.
func (h *homeBase) show() {
	runtime.WindowUnminimise(h.ctx)
	runtime.WindowShow(h.ctx)
	// Always-on-top for an instant is the dependable way to raise a window over
	// others on Windows without keeping it there.
	runtime.WindowSetAlwaysOnTop(h.ctx, true)
	runtime.WindowSetAlwaysOnTop(h.ctx, false)
}

// --- signing in --------------------------------------------------------------

func (h *homeBase) signIn(ctx context.Context) error {
	body, _ := json.Marshal(map[string]any{"secret": h.secret, "create": false})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.base+"/api/auth/local", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := h.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		// 409 means nobody has an account yet: nothing to watch until they do.
		return fmt.Errorf("owner sign-in answered %s", res.Status)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil || out.Token == "" {
		return fmt.Errorf("owner sign-in gave no token")
	}
	h.mu.Lock()
	h.token = out.Token
	h.mu.Unlock()
	return nil
}

func (h *homeBase) authed(ctx context.Context, method, path string, body any, client *http.Client) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, h.base+path, rd)
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	token := h.token
	h.mu.Unlock()
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return client.Do(req)
}

// sleep waits, and reports false when the context ended first.
func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// --- approvals -----------------------------------------------------------------

type pendingApproval struct {
	ID          string `json:"id"`
	Tier        int    `json:"tier"`
	TierName    string `json:"tierName"`
	Summary     string `json:"summary"`
	Detail      string `json:"detail"`
	CanRemember bool   `json:"canRemember"`
}

func (h *homeBase) watchApprovals(ctx context.Context) {
	for ctx.Err() == nil {
		if err := h.signIn(ctx); err != nil {
			if !sleep(ctx, approvalPoll) {
				return
			}
			continue
		}
		h.streamApprovals(ctx)
		if !sleep(ctx, 2*time.Second) {
			return
		}
	}
}

// streamApprovals follows /api/approvals/stream until it ends.
func (h *homeBase) streamApprovals(ctx context.Context) {
	// A stream has no overall deadline, so it gets a client without one.
	res, err := h.authed(ctx, http.MethodGet, "/api/approvals/stream", nil, &http.Client{Transport: &http.Transport{Proxy: nil}})
	if err != nil {
		return
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return
	}

	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	event := ""
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: ") && event == "approvals":
			var payload struct {
				Approvals []pendingApproval `json:"approvals"`
			}
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &payload) == nil {
				h.onApprovals(payload.Approvals)
			}
		case line == "":
			event = ""
		}
	}
}

func (h *homeBase) onApprovals(list []pendingApproval) {
	current := map[string]bool{}
	for _, a := range list {
		current[a.ID] = true
		h.mu.Lock()
		seen := h.notified[a.ID]
		h.notified[a.ID] = true
		h.mu.Unlock()
		if seen {
			continue
		}
		// The window shows its own dialog when it is in front; this is for when
		// it is not, so the agent is never stuck waiting on a question nobody saw.
		if runtime.WindowIsMinimised(h.ctx) {
			go h.askNatively(a)
		}
	}
	// Forget answered ones so the set does not grow for ever.
	h.mu.Lock()
	for id := range h.notified {
		if !current[id] {
			delete(h.notified, id)
		}
	}
	h.mu.Unlock()
}

func (h *homeBase) askNatively(a pendingApproval) {
	_ = runtime.SendNotification(h.ctx, runtime.NotificationOptions{
		ID:    "approval-" + a.ID,
		Title: "Calabrass needs your OK",
		Body:  clipText(a.Summary, 140),
	})

	const allow, deny = "Allow once", "Deny"
	title := "Calabrass needs your OK"
	if a.Tier >= 3 {
		title = "Calabrass is asking to do something hard to undo"
	}
	message := a.Summary
	if d := strings.TrimSpace(a.Detail); d != "" && d != a.Summary {
		message += "\n\n" + clipText(d, 600)
	}
	choice, err := runtime.MessageDialog(h.ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         title,
		Message:       message,
		Buttons:       []string{allow, deny},
		DefaultButton: deny,
		CancelButton:  deny,
	})
	decision := "deny"
	if err == nil && choice == allow {
		decision = "allow_once"
	}
	// If someone answered it in the window meanwhile, this is a harmless 404.
	if res, err := h.authed(h.ctx, http.MethodPost, "/api/approvals/"+a.ID, map[string]string{"decision": decision}, h.http); err == nil {
		_ = res.Body.Close()
	}
}

// --- finished runs -------------------------------------------------------------

type ledgerRow struct {
	At         int64  `json:"at"`
	Kind       string `json:"kind"`
	Summary    string `json:"summary"`
	Outcome    string `json:"outcome"`
	Unattended bool   `json:"unattended"`
}

// watchRuns tells you how scheduled and headless runs ended while you were away.
func (h *homeBase) watchRuns(ctx context.Context) {
	for sleep(ctx, runPoll) {
		h.mu.Lock()
		have := h.token != ""
		h.mu.Unlock()
		if !have {
			continue
		}
		res, err := h.authed(ctx, http.MethodGet, "/api/ledger?unattended=1&limit=20", nil, h.http)
		if err != nil {
			continue
		}
		var body struct {
			Entries []ledgerRow `json:"entries"`
		}
		_ = json.NewDecoder(res.Body).Decode(&body)
		_ = res.Body.Close()

		newest := h.lastRun
		for _, e := range body.Entries {
			if e.Kind != "run" || e.At <= h.lastRun {
				continue
			}
			if e.At > newest {
				newest = e.At
			}
			title := "Calabrass finished a run"
			switch e.Outcome {
			case "failed":
				title = "A Calabrass run failed"
			case "missed":
				title = "A Calabrass task was missed"
			}
			_ = runtime.SendNotification(h.ctx, runtime.NotificationOptions{
				ID: fmt.Sprintf("run-%d", e.At), Title: title, Body: clipText(e.Summary, 140),
			})
		}
		h.lastRun = newest
	}
}

// --- dropping a folder ---------------------------------------------------------

// folderDropped opens a chat in a dropped folder. The window's own page makes the
// chat, so it belongs to whoever is signed in there; this only says which folder.
func (h *homeBase) folderDropped(paths []string) {
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		dir := p
		if !info.IsDir() {
			// A file stands for the folder it is in.
			dir = filepath.Dir(p)
		}
		h.show()
		runtime.EventsEmit(h.ctx, openFolderEvent, dir)
		return
	}
}

// --- small helpers --------------------------------------------------------------

func clipText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ensure the loopback helper stays referenced for readers of this file: the
// window only ever talks to the host over loopback.
var _ = netguard.IsLoopbackAddr
