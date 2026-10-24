package main

import (
	"time"

	"charm.land/bubbles/v2/paginator"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	spinDots     = 8                     // number of dots
	spinInterval = 90 * time.Millisecond // how fast the focus moves
)

// spinTickMsg moves the active dot one step.
type spinTickMsg struct{}

func spinTick() tea.Cmd {
	return tea.Tick(spinInterval, func(time.Time) tea.Msg { return spinTickMsg{} })
}

// newSpinner builds the paginator that is used as an animated dot row.
// The paginator itself does not animate: a timer moves its active page.
func newSpinner() paginator.Model {
	p := paginator.New()
	p.Type = paginator.Dots
	p.PerPage = 1
	p.SetTotalPages(spinDots)
	return p
}

// running reports whether the AI is thinking or typing.
func (m Model) running() bool {
	for _, msg := range m.messages {
		if msg.Role == RoleAssistant && (msg.Pending || msg.Streaming) {
			return true
		}
	}
	return false
}

// advanceSpinner moves the focus to the next dot (wraps around).
func (m *Model) advanceSpinner() {
	m.spin.Page = (m.spin.Page + 1) % spinDots
}

// interrupt stops every answer that is still running (Esc).
func (m *Model) interrupt() {
	for i := range m.messages {
		msg := &m.messages[i]
		if msg.Role == RoleAssistant && (msg.Pending || msg.Streaming) {
			msg.Pending = false
			msg.Streaming = false
			msg.Interrupted = true
			msg.Elapsed = time.Since(msg.Started)
		}
	}
}

// runningView is the footer while the AI works:  • • ● • • • • •  esc interrupt
func (m Model) runningView() string {
	accent := lipgloss.Color("#ffffff")
	if n := len(m.messages); n > 0 {
		accent = m.messages[n-1].Accent
	}

	sp := m.spin // copy, so styling here doesn't touch the model
	sp.ActiveDot = lipgloss.NewStyle().Foreground(accent).Bold(true).Render("•")
	sp.InactiveDot = lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")).Render("•")

	esc := lipgloss.NewStyle().Foreground(lipgloss.Color("#b0b0b0")).Bold(true).Render("esc")
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("interrupt")

	return sp.View() + "  " + esc + " " + hint
}
