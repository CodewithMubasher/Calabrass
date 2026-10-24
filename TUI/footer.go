package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	contextLimit = 256000 // fake context window, used for the "3%" number
	baseTokens   = 7295   // fake tokens already used by the system prompt
)

// --- working directory + git branch ---------------------------------------

// workDirLabel returns something like "~/desktop/project:feature/cli-app".
func workDirLabel() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	wd = filepath.ToSlash(wd)
	if home, err := os.UserHomeDir(); err == nil {
		home = filepath.ToSlash(home)
		if strings.HasPrefix(wd, home) {
			wd = "~" + strings.TrimPrefix(wd, home)
		}
	}
	if b := gitBranch(); b != "" {
		wd += ":" + b
	}
	return wd
}

// gitBranch reads .git/HEAD, walking up from the current directory.
func gitBranch() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, ".git", "HEAD"))
		if err == nil {
			s := strings.TrimSpace(string(data))
			if strings.HasPrefix(s, "ref: refs/heads/") {
				return strings.TrimPrefix(s, "ref: refs/heads/")
			}
			if len(s) > 7 {
				return s[:7] // detached HEAD: short hash
			}
			return s
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// --- tokens -----------------------------------------------------------------

// tokens is a rough fake count: base + about 1 token per 4 characters.
func (m Model) tokens() int {
	n := baseTokens
	for _, msg := range m.messages {
		n += msg.charCount() / 4
	}
	return n
}

func (m Model) tokenPercent() int {
	return (m.tokens()*100 + contextLimit/2) / contextLimit
}

// formatInt adds thousands separators: 7295 -> "7,295".
func formatInt(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteString(",")
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// formatShort: 7295 -> "7.3K".
func formatShort(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1fK", float64(n)/1000)
}

// --- footer (one row under the prompt-input) -------------------------------

// footerView is the row under the prompt-input, `w` columns wide.
//
//	chat screen: "~/path:branch ........ 7.3K (3%)   ctrl+p commands"
//	home screen: "tab agents ........ ctrl+p commands"
func (m Model) footerView(w int, home bool) string {
	bold := lipgloss.NewStyle().Foreground(colorText).Bold(true)

	right := bold.Render(keyCommandPalette) + msgMuted.Render(" commands")
	if !home {
		right = msgMuted.Render(fmt.Sprintf("%s (%d%%)", formatShort(m.tokens()), m.tokenPercent())) +
			"   " + right
	}

	var left string
	switch {
	case home:
		left = bold.Render("tab") + msgMuted.Render(" agents")
	case m.running():
		avail := w - lipgloss.Width(right) - 1
		if avail < 0 {
			avail = 0
		}
		left = ansi.Truncate(m.runningView(), avail, "") // dots + esc + interrupt
	default:
		// Chat page: the working folder (cut with "…" if it doesn't fit).
		avail := w - lipgloss.Width(right) - 1
		if avail < 0 {
			avail = 0
		}
		left = msgMuted.Render(ansi.Truncate(m.cwd, avail, "…"))
	}

	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return ansi.Truncate(left+strings.Repeat(" ", gap)+right, w, "")
}

// --- right panel content ----------------------------------------------------

type sidebarItem struct {
	Name   string
	Status string
	Active bool // active items are drawn in blue
}

type sidebarSection struct {
	Title string
	Items []sidebarItem
}

// defaultSidebar is the list shown in the right panel. Edit it freely.
func defaultSidebar() []sidebarSection {
	return []sidebarSection{
		{Title: "MCPs", Items: []sidebarItem{
			{Name: "Gmail", Status: "Disabled"},
			{Name: "Excel", Status: "Disabled"},
			{Name: "Win-control", Status: "Disabled"},
		}},
		{Title: "Workers", Items: []sidebarItem{
			{Name: "Frontend agent", Status: "Idle"},
		}},
	}
}

// truncLeft keeps the END of a path and cuts the start: "…/calabrass/TUI".
func truncLeft(s string, w int) string {
	r := []rune(s)
	if w < 1 || len(r) <= w {
		return s
	}
	return "…" + string(r[len(r)-w+1:])
}

// sidebarView builds the right panel text. inner/innerH are the space left
// inside the card after its padding.
func (m Model) sidebarView(inner, innerH int) string {
	if inner < 1 || innerH < 1 {
		return ""
	}

	// Every style needs the panel background, or gaps show up.
	base := lipgloss.NewStyle().Background(colorPanel)
	title := base.Foreground(colorText).Bold(true)
	muted := base.Foreground(promptMuted)
	blue := base.Foreground(promptAccentDefault)
	blank := base.Render(strings.Repeat(" ", inner))

	var lines []string

	// Session title.
	session := "New session - " + m.started.Format("2006-01-02T15:04:05.000Z")
	for _, l := range strings.Split(wrapText(session, inner), "\n") {
		lines = append(lines, title.Render(l))
	}

	// Context.
	lines = append(lines,
		blank,
		title.Render("Context"),
		muted.Render(formatInt(m.tokens())+" tokens"),
		muted.Render(fmt.Sprintf("%d%% used", m.tokenPercent())),
	)

	// MCPs, Workers, ...
	for _, sec := range defaultSidebar() {
		lines = append(lines, blank, title.Render(sec.Title))
		for _, it := range sec.Items {
			dot, status := muted, muted
			if it.Active {
				dot, status = blue, blue
			}
			left := dot.Render("• ") + title.Render(it.Name)
			right := status.Render(it.Status)
			lines = append(lines, twoCol(left, right, inner, base))
		}
	}

	// A row wider than the panel would wrap and push the layout down.
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], inner, "")
	}

	// Path pinned to the bottom.
	bottom := muted.Render(truncLeft(m.cwd, inner))
	for len(lines) > innerH-1 {
		lines = lines[:len(lines)-1]
	}
	for len(lines) < innerH-1 {
		lines = append(lines, blank)
	}
	lines = append(lines, bottom)

	return strings.Join(lines, "\n")
}
