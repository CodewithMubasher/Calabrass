package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"calabrass-tui/internal/api"
	"calabrass-tui/internal/settings"
)

var refused = &api.ConnError{Kind: api.ConnRefused}

// technical lists what must never reach the screen when the host is away.
var technical = []string{"dial tcp", "connectex", "127.0.0.1", "EOF", "refused", "http://", "/api/"}

func assertQuiet(t *testing.T, screen string) {
	t.Helper()
	for _, leak := range technical {
		if strings.Contains(screen, leak) {
			t.Errorf("the screen shows %q:\n%s", leak, screen)
		}
	}
}

func TestTheHostGoingAwayIsOneQuietFooterLineNotAFloodOfErrors(t *testing.T) {
	f := newFakeAPI()
	f.models = oneModel()
	h, closeFn := boot(t, f)
	defer closeFn()
	before := len(h.m.messages)

	// Every background call fails the same way; send several, as a poll would.
	for i := 0; i < 5; i++ {
		h.send(schedulesMsg{err: &api.ConnError{Kind: api.ConnRefused, Err: http.ErrServerClosed}})
	}

	if !h.m.conn.down {
		t.Fatal("the screen did not notice the host was gone")
	}
	if got := len(h.m.messages); got != before {
		t.Fatalf("%d error messages were written into the conversation", got-before)
	}
	screen := h.screen()
	if !strings.Contains(screen, "Host disconnected") || !strings.Contains(screen, "reconnecting") {
		t.Fatalf("no calm status line:\n%s", screen)
	}
	assertQuiet(t, screen)
	if h.m.conn.gen != 1 {
		t.Fatalf("five failures started %d reconnect loops, want 1", h.m.conn.gen)
	}
}

func TestPollingStopsWhileTheHostIsAway(t *testing.T) {
	f := newFakeAPI()
	f.models = oneModel()
	h, closeFn := boot(t, f)
	defer closeFn()

	h.send(schedulesMsg{err: refused})
	if !h.m.conn.down {
		t.Fatal("not down")
	}
	base := f.hitCount("/api/schedules")

	// The 30-second poll fires while the host is away: it must not ask.
	model, cmd := h.m.Update(schedulePollTick{})
	h.m = model.(Model)
	done := make(chan struct{})
	go func() { defer close(done); runCmdFor(cmd, 200*time.Millisecond) }()
	<-done
	if got := f.hitCount("/api/schedules"); got != base {
		t.Fatalf("the poll called the host %d time(s) while it was away", got-base)
	}
}

// runCmdFor runs a command for at most d, discarding what it returns: a poll's
// tick command would otherwise sleep for its full interval.
func runCmdFor(cmd tea.Cmd, d time.Duration) {
	if cmd == nil {
		return
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		if b, ok := msg.(tea.BatchMsg); ok {
			for _, c := range b {
				runCmdFor(c, d)
			}
		}
	case <-time.After(d):
	}
}

func TestWhenTheHostAnswersAgainTheScreenRecoversAndSaysSoOnce(t *testing.T) {
	f := newFakeAPI()
	f.models = oneModel()
	h, closeFn := boot(t, f)
	defer closeFn()

	h.send(schedulesMsg{err: refused})
	gen := h.m.conn.gen

	// A failed look keeps it going with a longer pause.
	h.send(probeMsg{gen: gen, err: refused})
	if !h.m.conn.down || h.m.conn.attempts != 1 {
		t.Fatalf("down=%v attempts=%d", h.m.conn.down, h.m.conn.attempts)
	}
	// A look from an earlier outage is ignored.
	h.send(probeMsg{gen: gen - 1, err: nil})
	if !h.m.conn.down {
		t.Fatal("a stale result ended the outage")
	}

	h.send(probeMsg{gen: gen, err: nil})
	if h.m.conn.down {
		t.Fatal("still down after the host answered")
	}
	if !strings.Contains(h.screen(), "Reconnected") {
		t.Fatalf("recovery was not announced:\n%s", h.screen())
	}
	if strings.Contains(h.screen(), "Host disconnected") {
		t.Fatal("the footer still says disconnected")
	}
}

func TestOnlyAConnectionFailureStartsTheReconnectLoop(t *testing.T) {
	f := newFakeAPI()
	f.models = oneModel()
	h, closeFn := boot(t, f)
	defer closeFn()

	// The host said no. That is not the host being away.
	h.send(schedulesMsg{err: &api.APIError{Status: 500, Message: "boom"}})
	if h.m.conn.down {
		t.Fatal("an HTTP error was treated as the host being gone")
	}
	if !strings.Contains(h.screen(), "boom") {
		t.Fatalf("a real error should still be shown:\n%s", h.screen())
	}
}

func TestAReplyCutShortByTheHostGoingAwayIsNotPresentedAsAnAnswer(t *testing.T) {
	f := newFakeAPI()
	f.models = oneModel()
	f.script = func(w http.ResponseWriter, r *http.Request, emit func(string, any)) {
		emit("chat", api.Chat{ID: "chat-1", Title: "hi", Provider: "p1", Model: "m-1"})
		emit("message", api.Message{ID: "u1", Role: "user", Content: "hi"})
		emit("token", map[string]string{"text": "The answer is"})
		// ...and the host dies: the stream ends with no "done".
	}
	h, closeFn := boot(t, f)
	defer closeFn()

	h.typeAndEnter("hi")

	if !h.m.conn.down {
		t.Fatal("a reply that stopped halfway did not start the reconnect loop")
	}
	screen := h.screen()
	if !strings.Contains(screen, "Lost the connection") && !strings.Contains(screen, "lost the connection") {
		t.Fatalf("the person is not told the reply was cut off:\n%s", screen)
	}
	if !strings.Contains(screen, "/retry") {
		t.Fatalf("no way forward offered:\n%s", screen)
	}
	assertQuiet(t, strings.ReplaceAll(screen, "/retry", ""))
}

func TestStartingWithNoHostRetriesByItselfAndLeaksNothing(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close() // nothing there

	msgCache = map[int]cachedMsg{}
	t.Setenv(settings.EnvDir, t.TempDir())
	t.Setenv("CALABRASS_EMAIL", "")
	t.Setenv("CALABRASS_PASSWORD", "")
	app := &appCtx{client: api.New(addr), st: &settings.Settings{}}
	h := &harness{t: t, m: NewModel(app)}
	h.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	for _, msg := range runCmd(bootCmd(app)) {
		h.send(msg)
	}

	if h.m.boot != bootFailed || !h.m.conn.down {
		t.Fatalf("boot=%v down=%v", h.m.boot, h.m.conn.down)
	}
	screen := h.screen()
	if !strings.Contains(screen, "Can't reach the Calabrass host") || !strings.Contains(screen, "Trying again by itself") {
		t.Fatalf("the start screen does not say what is happening:\n%s", screen)
	}
	assertQuiet(t, screen)

	// When the host turns up, the normal start carries on by itself. (Not through
	// the harness, which would run that start against the still-dead address.)
	model, cmd := h.m.Update(probeMsg{gen: h.m.conn.gen, err: nil})
	h.m = model.(Model)
	if h.m.boot != bootConnecting || h.m.conn.down || cmd == nil {
		t.Fatalf("boot=%v down=%v cmd=%v after the host answered", h.m.boot, h.m.conn.down, cmd != nil)
	}
}

func TestReconnectPausesGrowThenLevelOff(t *testing.T) {
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 10 * time.Second, 10 * time.Second, 10 * time.Second}
	for i, w := range want {
		if got := connDelay(i); got != w {
			t.Errorf("connDelay(%d) = %s, want %s", i, got, w)
		}
	}
}
