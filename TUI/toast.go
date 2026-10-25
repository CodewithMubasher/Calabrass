package main

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	toastDuration = 3 * time.Second
	toastMaxWidth = 44
	toastTop      = 2 // rows between the top of the screen and the toast
	toastPadRows  = 1 // blank rows above and below the text
)

// toastMsg clears the toast, unless a newer toast replaced it.
type toastMsg struct{ seq int }

// showToast puts a one-line message in the top-right corner of the chat area
// and returns the command that removes it after a few seconds.
func (m *Model) showToast(text string, isErr bool) tea.Cmd {
	m.toastText, m.toastErr = strings.TrimSpace(text), isErr
	m.toastSeq++
	seq := m.toastSeq
	return tea.Tick(toastDuration, func(time.Time) tea.Msg { return toastMsg{seq: seq} })
}

// toastView draws:  │ content │
func (m Model) toastView(maxW int) string {
	if maxW < 10 || strings.TrimSpace(m.toastText) == "" {
		return ""
	}
	inner := maxW - 4 // "│ " + content + " │"
	if inner < 1 {
		return ""
	}

	text := ansi.Truncate(m.toastText, inner, "…")
	text += strings.Repeat(" ", inner-ansi.StringWidth(text))

	base := lipgloss.NewStyle().Background(promptBg)
	accent := promptAccentDefault
	if m.toastErr {
		accent = lipgloss.Color("#e06c75")
	}
	border := base.Foreground(accent).Bold(true)
	body := base.Foreground(colorText)

	row := func(content string) string {
		return border.Render("│ ") + body.Render(content) + border.Render(" │")
	}
	blank := row(strings.Repeat(" ", inner))

	lines := make([]string, 0, 1+2*toastPadRows)
	for i := 0; i < toastPadRows; i++ {
		lines = append(lines, blank)
	}
	lines = append(lines, row(text))
	for i := 0; i < toastPadRows; i++ {
		lines = append(lines, blank)
	}
	return strings.Join(lines, "\n")
}

// withToast pastes the toast over the finished screen. It is applied after the
// dialogs, so a toast is visible even when a modal is open.
func (m Model) withToast(view string) string {
	if strings.TrimSpace(m.toastText) == "" || m.width == 0 {
		return view
	}

	maxW := toastMaxWidth
	if m.screen == screenChat {
		if n := m.centerWidth() - 2; n < maxW {
			maxW = n
		}
	} else if n := m.width - marginLeft - margin - 2; n < maxW {
		maxW = n
	}
	if maxW < 10 {
		return view
	}

	toast := m.toastView(maxW)
	if toast == "" {
		return view
	}

	x := m.width - maxW - margin - 1
	if m.screen == screenChat {
		x = marginLeft + m.centerWidth() - maxW - 1
	}
	if x < 0 {
		x = 0
	}
	return placeOver(view, toast, x, toastTop)
}

// placeOver pastes fg over bg at (x, y) without dimming the background.
func placeOver(bg, fg string, x, y int) string {
	bgLines := strings.Split(bg, "\n")
	for i, fl := range strings.Split(fg, "\n") {
		row := y + i
		if row < 0 || row >= len(bgLines) {
			continue
		}
		fw := ansi.StringWidth(fl)
		bl := bgLines[row]

		left := ansi.Truncate(bl, x, "")
		if lw := ansi.StringWidth(left); lw < x {
			left += strings.Repeat(" ", x-lw)
		}
		right := ansi.TruncateLeft(bl, x+fw, "")

		bgLines[row] = left + ansiReset + fl + ansiReset + right
	}
	return strings.Join(bgLines, "\n")
}
