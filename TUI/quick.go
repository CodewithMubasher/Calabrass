package main

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// holdTickMsg fires once, two seconds after the first ↓ press.
type holdTickMsg struct{ seq int }

func holdTick(seq int) tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return holdTickMsg{seq: seq} })
}

// QuickSelectMsg is sent when the hold-↓ menu runs an action. The ID matches
// a command-palette ID, so both menus share one dispatcher.
type QuickSelectMsg struct {
	ID string
}

// QuickMenu is the small action popup opened by holding ↓ for two seconds.
type QuickMenu struct {
	open    bool
	cursor  int
	screenW int
	screenH int
}

type quickAction struct {
	ID    string
	Title string
	Hint  string
}

var quickActions = []quickAction{
	{ID: "copy", Title: "Copy last response", Hint: "/copy"},
	{ID: "help", Title: "Help", Hint: "/help"},
	{ID: "switch-session", Title: "Open an earlier session", Hint: "/continue"},
	{ID: "switch-model", Title: "Switch model", Hint: "/models"},
	{ID: "new-session", Title: "New session", Hint: "/new"},
	{ID: "mcps", Title: "MCP servers", Hint: "/mcps"},
}

func NewQuickMenu() QuickMenu { return QuickMenu{} }

func (q QuickMenu) IsOpen() bool { return q.open }

// SetSize tells the popup how big the whole screen is.
func (q *QuickMenu) SetSize(w, h int) { q.screenW, q.screenH = w, h }

// Open shows the menu on its first row.
func (q *QuickMenu) Open() {
	q.open = true
	q.cursor = 0
}

// Close hides the menu.
func (q *QuickMenu) Close() { q.open = false }

func (q QuickMenu) width() int {
	w := 46
	if q.screenW-4 < w {
		w = q.screenW - 4
	}
	if w < 20 {
		w = 20
	}
	return w
}

func (q QuickMenu) innerWidth() int { return q.width() - 2*dialogPadX }

func (q QuickMenu) Selected() quickAction {
	if q.cursor < 0 || q.cursor >= len(quickActions) {
		return quickAction{}
	}
	return quickActions[q.cursor]
}

// Update handles keys while the quick menu is open.
func (q QuickMenu) Update(msg tea.Msg) (QuickMenu, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return q, nil
	}
	switch key.String() {
	case "esc", "left":
		q.Close()
	case "up", "k":
		q.cursor = (q.cursor - 1 + len(quickActions)) % len(quickActions)
	case "down", "j":
		q.cursor = (q.cursor + 1) % len(quickActions)
	case "enter":
		a := q.Selected()
		q.Close()
		if a.ID == "" {
			return q, nil
		}
		return q, func() tea.Msg { return QuickSelectMsg{ID: a.ID} }
	}
	return q, nil
}

// View renders the popup itself (without the dimmed background).
func (q QuickMenu) View() string {
	w := q.width()
	inner := q.innerWidth()
	base := lipgloss.NewStyle().Background(dialogBg)

	blank := base.Render(strings.Repeat(" ", w))
	line := func(content string) string {
		content = ansi.Truncate(content, inner, "…")
		gap := inner - lipgloss.Width(content)
		if gap < 0 {
			gap = 0
		}
		return base.Render(strings.Repeat(" ", dialogPadX)) +
			content +
			base.Render(strings.Repeat(" ", gap+dialogPadX))
	}

	title := twoCol(
		base.Foreground(colorText).Bold(true).Render("Quick actions"),
		base.Foreground(dialogMuted).Render("esc"),
		inner, base,
	)

	out := []string{blank, line(title), blank}
	for i, a := range quickActions {
		if i == q.cursor {
			sel := lipgloss.NewStyle().Background(dialogBlue).Foreground(dialogDark)
			pad := sel.Render(strings.Repeat(" ", dialogPadX))
			content := twoCol(
				sel.Bold(true).Render("› "+a.Title),
				sel.Render(a.Hint),
				inner, sel,
			)
			out = append(out, pad+content+pad)
		} else {
			content := twoCol(
				base.Render("  ")+base.Foreground(colorText).Bold(true).Render(a.Title),
				base.Foreground(dialogMuted).Render(a.Hint),
				inner, base,
			)
			out = append(out, line(content))
		}
	}
	hint := twoCol(
		base.Foreground(dialogMuted).Render("enter run"),
		base.Foreground(dialogMuted).Render("hold ↓"),
		inner, base,
	)
	out = append(out, blank, line(hint), blank)
	return strings.Join(out, "\n")
}

// Overlay draws the popup centered on top of `bg` and dims the background.
func (q QuickMenu) Overlay(bg string, screenW, screenH int) string {
	box := q.View()
	x := (screenW - lipgloss.Width(box)) / 2
	y := (screenH - lipgloss.Height(box)) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return overlay(bg, box, x, y)
}
