package main

import (
    "strings"

    "github.com/charmbracelet/lipgloss"
    "github.com/charmbracelet/x/ansi"
)

// How many commands are visible at once.
const slashMaxRows = 10

// SlashCommand is one entry of the "/" menu.
type SlashCommand struct {
    Name string // with the slash, e.g. "/agents"
    Desc string
}

// SlashSelectMsg is sent when the user picks a command (Enter).
type SlashSelectMsg struct {
    Command SlashCommand
}

// defaultSlashCommands is the menu content. Keep it sorted A-Z. Edit freely.
func defaultSlashCommands() []SlashCommand {
    return []SlashCommand{
        {"/agents", "Switch agent"},
        {"/btw", "One-shot answer from the session's context without adding to it"},
        {"/cd", "Change working directory"},
        {"/clear", "Clear session"},
        {"/connect", "Connect an integration"},
        {"/continue", "Switch session"},
        {"/debug", "View debug info"},
        {"/diff", "Open diff viewer"},
        {"/editor", "Open editor"},
        {"/effort", "Switch model variant"},
        {"/exit", "Exit the app"},
        {"/export", "Export session to Markdown"},
        {"/help", "Show help"},
        {"/init", "Create or update the project guide"},
        {"/mcps", "Toggle MCP servers"},
        {"/models", "Switch model"},
        {"/new", "Start a new session"},
        {"/share", "Share session"},
        {"/themes", "Switch theme"},
    }
}

// SlashMenu is the command list that opens above the prompt-input when the
// text starts with "/".
//
//   "/"        -> all commands
//   "/cl"      -> commands matching "cl" (search)
//   "/cl "     -> a space closes the menu
type SlashMenu struct {
    commands  []SlashCommand
    cursor    int    // selected row (index into the matches)
    scroll    int    // first visible row
    dismissed bool   // closed with Esc, stays closed until the text no longer starts with "/"
    lastValue string // to notice when the search text changed
}

// NewSlashMenu creates the menu with the default commands.
func NewSlashMenu() SlashMenu {
    return SlashMenu{commands: defaultSlashCommands()}
}

// matches returns the commands for the current input text. Name starts with
// the search text first, then name contains it, then description contains it.
func (s SlashMenu) matches(value string) []SlashCommand {
    if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, " \t\n") {
        return nil
    }
    q := strings.ToLower(strings.TrimPrefix(value, "/"))
    if q == "" {
        return s.commands
    }

    var starts, inName, inDesc []SlashCommand
    for _, c := range s.commands {
        name := strings.ToLower(strings.TrimPrefix(c.Name, "/"))
        switch {
        case strings.HasPrefix(name, q):
            starts = append(starts, c)
        case strings.Contains(name, q):
            inName = append(inName, c)
        case strings.Contains(strings.ToLower(c.Desc), q):
            inDesc = append(inDesc, c)
        }
    }
    out := append(starts, inName...)
    return append(out, inDesc...)
}

// Visible reports whether the menu should be drawn for this input text.
func (s SlashMenu) Visible(value string) bool {
    return !s.dismissed && len(s.matches(value)) > 0
}

// Sync must be called after the input text may have changed.
func (s *SlashMenu) Sync(value string) {
    if !strings.HasPrefix(value, "/") {
        s.dismissed = false
    }
    if value != s.lastValue {
        s.lastValue = value
        s.cursor = 0
        s.scroll = 0
    }
}

// Dismiss closes the menu (Esc).
func (s *SlashMenu) Dismiss() { s.dismissed = true }

// Move moves the selection up (-1) or down (+1), wrapping around.
func (s *SlashMenu) Move(delta int, value string) {
    n := len(s.matches(value))
    if n == 0 {
        return
    }
    s.cursor = (s.cursor + delta + n) % n

    rows := slashMaxRows
    if n < rows {
        rows = n
    }
    if s.cursor < s.scroll {
        s.scroll = s.cursor
    }
    if s.cursor >= s.scroll+rows {
        s.scroll = s.cursor - rows + 1
    }
}

// Selected returns the highlighted command.
func (s SlashMenu) Selected(value string) (SlashCommand, bool) {
    ms := s.matches(value)
    if s.cursor < 0 || s.cursor >= len(ms) {
        return SlashCommand{}, false
    }
    return ms[s.cursor], true
}

// nameWidth is the width of the name column (longest name + 3 spaces).
func (s SlashMenu) nameWidth() int {
    w := 0
    for _, c := range s.commands {
        if n := lipgloss.Width(c.Name); n > w {
            w = n
        }
    }
    return w + 3
}

// Lines draws the menu: one string per row, each exactly `width` columns
// wide. At most `maxRows` rows are returned.
func (s SlashMenu) Lines(value string, width, maxRows int) []string {
    ms := s.matches(value)
    count := slashMaxRows
    if len(ms) < count {
        count = len(ms)
    }
    if maxRows < count {
        count = maxRows
    }
    if count < 1 || width < 8 {
        return nil
    }

    cur := s.cursor
    if cur >= len(ms) {
        cur = len(ms) - 1
    }
    start := s.scroll
    if cur < start {
        start = cur
    }
    if cur >= start+count {
        start = cur - count + 1
    }
    if start+count > len(ms) {
        start = len(ms) - count
    }
    if start < 0 {
        start = 0
    }

    nameW := s.nameWidth()
    base := lipgloss.NewStyle().Background(dialogBg)
    sel := lipgloss.NewStyle().Background(dialogBlue).Foreground(dialogDark)

    lines := make([]string, 0, count)
    for i := start; i < start+count; i++ {
        c := ms[i]

        row := base
        name := base.Foreground(colorText).Bold(true)
        desc := base.Foreground(dialogMuted)
        if i == cur {
            row = sel
            name = sel.Bold(true)
            desc = sel
        }

        // Description is cut at the right edge (no "...").
        avail := width - 2 - nameW - 1
        d := ""
        if avail > 0 {
            d = ansi.Truncate(c.Desc, avail, "")
        }

        namePadded := c.Name + strings.Repeat(" ", nameW-lipgloss.Width(c.Name))
        line := row.Render("  ") + name.Render(namePadded) + desc.Render(d)
        if fill := width - lipgloss.Width(line); fill > 0 {
            line += row.Render(strings.Repeat(" ", fill))
        }
        lines = append(lines, line)
    }
    return lines
}

// replaceBottomLines draws `rows` over the last lines of `s`.
func replaceBottomLines(s string, rows []string) string {
    lines := strings.Split(s, "\n")
    if len(rows) > len(lines) {
        rows = rows[len(rows)-len(lines):]
    }
    copy(lines[len(lines)-len(rows):], rows)
    return strings.Join(lines, "\n")
}
