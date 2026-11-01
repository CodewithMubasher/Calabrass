package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"calabrass-tui/internal/api"
	"calabrass-tui/internal/hostctl"
	"calabrass-tui/internal/session"
)

// The host can go away while the screen is open: it was stopped from another
// window, the machine slept, it crashed. That must not break the screen, spill
// error text into the conversation, or need the person to do anything.
//
// When a call fails because the host cannot be reached, the screen goes into one
// calm state: a line in the footer says so, nothing else is written, background
// polling stops, and it tries again with growing pauses. From the second try it
// also starts the host itself, if this terminal was launched with the host's
// program and the host is on this machine. When the host answers, it signs in
// again, reloads what it showed, and says so once.

type connState struct {
	down     bool
	since    time.Time
	attempts int
	gen      int       // invalidates the ticks of an earlier outage
	lastHeal time.Time // when this screen last tried to start the host
}

// reconnectTickMsg says it is time to look for the host again.
type reconnectTickMsg struct{ gen int }

// probeMsg is the result of looking.
type probeMsg struct {
	gen    int
	err    error
	healed bool // this screen started the host
}

// healEvery is the least time between this screen's attempts to start the host.
const healEvery = 20 * time.Second

// connDelay is how long to wait before try number n: 1, 2, 4, 8, then 10 seconds.
func connDelay(n int) time.Duration {
	d := time.Second << uint(n)
	if n > 3 || d > 10*time.Second {
		d = 10 * time.Second
	}
	return d
}

func reconnectTick(gen int, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return reconnectTickMsg{gen: gen} })
}

// connLost starts the reconnect loop when err means the host cannot be reached.
// It returns nil for any other error, and when a loop is already running.
func (m *Model) connLost(err error) tea.Cmd {
	if !api.IsConnError(err) || m.conn.down {
		return nil
	}
	m.conn = connState{down: true, since: time.Now(), gen: m.conn.gen + 1, lastHeal: m.conn.lastHeal}
	return reconnectTick(m.conn.gen, connDelay(0))
}

// onReconnectTick runs one attempt.
func (m *Model) onReconnectTick(msg reconnectTickMsg) tea.Cmd {
	if !m.conn.down || msg.gen != m.conn.gen {
		return nil
	}
	heal := m.conn.attempts >= 1 && time.Since(m.conn.lastHeal) > healEvery
	if heal {
		m.conn.lastHeal = time.Now()
	}
	return probeCmd(m.app, msg.gen, heal)
}

func probeCmd(app *appCtx, gen int, heal bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := withTimeout(30 * time.Second)
		defer cancel()

		err := app.client.Health(ctx)
		healed := false
		if err != nil && heal {
			addr := strings.TrimPrefix(app.client.Base, "http://")
			if bin, ok := hostctl.Binary(); ok && hostctl.IsLocal(addr) {
				if hostctl.Start(ctx, bin, addr, 10*time.Second) == nil {
					healed = true
					err = app.client.Health(ctx)
				}
			}
		}
		if err == nil {
			// It answers again. A host that was restarted keeps its sessions; one
			// that lost its data does not, so make sure we are still signed in.
			if _, e := session.Ensure(ctx, app.client, app.st); e != nil {
				err = e
			} else {
				_ = app.st.Save()
			}
		}
		return probeMsg{gen: gen, err: err, healed: healed}
	}
}

// onProbe handles what an attempt found.
func (m *Model) onProbe(msg probeMsg) tea.Cmd {
	if !m.conn.down || msg.gen != m.conn.gen {
		return nil
	}
	if errors.Is(msg.err, session.ErrNotPaired) {
		// The host is there, and says this device is not welcome. Waiting will not
		// change that, so stop looking and say what to do.
		m.conn.down = false
		if m.boot != bootReady {
			m.boot, m.bootErr = bootFailed, msg.err.Error()
			return nil
		}
		m.notice(msg.err.Error(), true)
		return nil
	}
	if msg.err != nil {
		m.conn.attempts++
		return reconnectTick(msg.gen, connDelay(m.conn.attempts))
	}

	m.conn.down = false
	if m.boot != bootReady {
		// It never came up in the first place: carry on with the normal start.
		m.boot = bootConnecting
		return bootCmd(m.app)
	}
	text := "Reconnected to the host"
	if msg.healed {
		text = "The host had stopped, so it was started again"
	}
	c := m.app.client
	return tea.Batch(m.showToast(text, false), loadModelsCmd(c, false), loadConnectorsCmd(c), loadSchedulesCmd(c))
}

// connBanner is the footer line while the host cannot be reached.
func (m Model) connBanner() string {
	if !m.conn.down {
		return ""
	}
	// Short: the footer is narrow, and a truncated warning is worse than a brief one.
	return fmt.Sprintf("Host disconnected · reconnecting (try %d)", m.conn.attempts+1)
}
