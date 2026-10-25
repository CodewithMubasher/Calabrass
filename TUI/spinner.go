package main

import (
	"time"

	"charm.land/bubbles/v2/paginator"
	"charm.land/bubbles/v2/spinner"
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

// newRowSpinner is the bubbles Spinner shown while the AI is thinking or
// making an edit (it turns into "+ 1 thought" / "+ 1 edit" when done).
func newRowSpinner() spinner.Model {
	return spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#b8862f"))),
	)
}

// running reports whether the AI is thinking or typing.
func (m Model) running() bool {
	return m.turn != nil
}

// advanceSpinner moves the focus to the next dot (wraps around).
func (m *Model) advanceSpinner() {
	m.spin.Page = (m.spin.Page + 1) % spinDots
}

// runningView is the footer while the AI works:           esc interrupt
func (m Model) runningView() string {
	accent := lipgloss.Color("#ffffff")
	if n := len(m.messages); n > 0 {
		accent = m.messages[n-1].Accent
	}

	sp := m.spin // copy, so styling here doesn't touch the model
	sp.ActiveDot = lipgloss.NewStyle().Foreground(accent).Bold(true).Render("■")
	sp.InactiveDot = lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")).Render("⬝")

	esc := lipgloss.NewStyle().Foreground(lipgloss.Color("#b0b0b0")).Bold(true).Render("esc")
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("interrupt")

	return sp.View() + "  " + esc + " " + hint
}
