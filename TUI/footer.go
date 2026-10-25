package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// --- working directory + git branch ---------------------------------------

// workDirLabel returns something like "~/desktop/project:feature/cli-app".
func workDirLabel(dir string) string {
	wd := filepath.ToSlash(dir)
	if home, err := os.UserHomeDir(); err == nil {
		home = filepath.ToSlash(home)
		if strings.HasPrefix(wd, home) {
			wd = "~" + strings.TrimPrefix(wd, home)
		}
	}
	if b := gitBranch(dir); b != "" {
		wd += ":" + b
	}
	return wd
}

// gitBranch reads .git/HEAD, walking up from the current directory.
func gitBranch(dir string) string {
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

// sessionTokens is what the turns of this session have used, as the backend
// reported it.
func (m Model) sessionTokens() int { return m.usageIn + m.usageOut }

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
//	chat screen: "~/path:branch ........ 7.3K tokens   ctrl+o rows   ctrl+p commands"
//	home screen: "~/path:branch (or a hint) ........ ctrl+p commands"
func (m Model) footerView(w int, home bool) string {
	bold := lipgloss.NewStyle().Foreground(colorText).Bold(true)

	right := bold.Render(keyCommandPalette) + msgMuted.Render(" commands")
	if !home {
		right = msgMuted.Render(keyToggleRows+" rows") + "   " + right
		if n := m.sessionTokens(); n > 0 {
			right = msgMuted.Render(formatShort(n)+" tokens") + "   " + right
		}
	}

	avail := w - lipgloss.Width(right) - 1
	if avail < 0 {
		avail = 0
	}
	var left string
	switch {
	case home && m.banner != "":
		left = lipgloss.NewStyle().Foreground(lipgloss.Color("#b8862f")).Render(ansi.Truncate(m.banner, avail, "…"))
	case home:
		left = msgMuted.Render(ansi.Truncate(m.cwd, avail, "…"))
	case m.running():
		left = ansi.Truncate(m.runningView(), avail, "") // dots + esc + interrupt
	default:
		left = msgMuted.Render(ansi.Truncate(m.cwd, avail, "…"))
	}

	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return ansi.Truncate(left+strings.Repeat(" ", gap)+right, w, "")
}

// --- right panel content ----------------------------------------------------

// truncLeft keeps the END of a path and cuts the start: "/calabrass/TUI".
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
	session := "New session"
	if m.chat != nil && strings.TrimSpace(m.chat.Title) != "" {
		session = m.chat.Title
	}
	for _, l := range strings.Split(wrapText(session, inner), "\n") {
		lines = append(lines, title.Render(l))
	}

	// Model + tokens, as the backend reported them.
	model := "none: /connect"
	if m.prompt.Model != "" {
		model = m.prompt.Model
	}
	lines = append(lines, blank, title.Render("Model"), muted.Render(model))
	if m.prompt.Provider != "" {
		lines = append(lines, muted.Render(m.prompt.Provider))
	}
	lines = append(lines, blank, title.Render("Tokens"))
	if m.sessionTokens() == 0 {
		lines = append(lines, muted.Render("none used yet"))
	} else {
		lines = append(lines,
			muted.Render(formatInt(m.sessionTokens())+" this session"),
			muted.Render(formatInt(m.usageIn)+" in · "+formatInt(m.usageOut)+" out"))
	}

	// MCP servers.
	lines = append(lines, blank, title.Render("MCPs"))
	if len(m.connectors) == 0 {
		lines = append(lines, muted.Render("none registered"))
	}
	for _, c := range m.connectors {
		dot, status, label := muted, muted, "Disabled"
		if c.Enabled {
			dot, status, label = blue, blue, "Enabled"
		}
		left := dot.Render("● ") + title.Render(c.Name)
		lines = append(lines, twoCol(left, status.Render(label), inner, base))
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
