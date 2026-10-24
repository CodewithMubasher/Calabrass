package main

import (
    "strings"

    "github.com/charmbracelet/bubbles/textinput"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
    "github.com/charmbracelet/x/ansi"
)

const (
    dialogMaxWidth   = 64 // widest the dialog gets (in columns)
    dialogPadX       = 3  // left/right padding inside the dialog
    dialogChromeRows = 6  // top pad, title, blank, search, blank, bottom pad
)

// Dialog colors: purple for section headers, blue for the selected row
// and the cursor.
var (
    dialogBg     = lipgloss.Color("#141414")
    dialogBlue   = lipgloss.Color("#5c9cf5")
    dialogPurple = lipgloss.Color("#b27ddb")
    dialogMuted  = lipgloss.Color("#808080")
    dialogDark   = lipgloss.Color("#141414") // text on the blue selected row
)

// Command is one entry in the command palette.
type Command struct {
    ID       string // used by the app to know what to run
    Title    string
    Shortcut string // shown on the right, can be empty
    Section  string // commands with the same section must be next to each other
}

// DialogSelectMsg is sent when the user presses Enter on a command.
type DialogSelectMsg struct {
    Command Command
}

// defaultCommands is the list shown in the palette. Edit it freely.
func defaultCommands() []Command {
    return []Command{
        {ID: "switch-session", Title: "Switch session", Shortcut: "ctrl+x l", Section: "Suggested"},
        {ID: "switch-model", Title: "Switch model", Shortcut: "ctrl+x m", Section: "Suggested"},
        {ID: "open-settings", Title: "Open settings", Section: "Suggested"},

        {ID: "toggle-right", Title: "Toggle right panel", Shortcut: keyToggleRight, Section: "View"},

        {ID: "switch-session", Title: "Switch session", Shortcut: "ctrl+x l", Section: "Session"},
        {ID: "new-session", Title: "New session", Shortcut: "ctrl+x n", Section: "Session"},
        {ID: "clear-session", Title: "Clear session", Section: "Session"},
        {ID: "open-project", Title: "Open session or project", Shortcut: "ctrl+o", Section: "Session"},
        {ID: "close-tab", Title: "Close tab", Shortcut: "ctrl+x w", Section: "Session"},
    }
}

// Dialog is the command-palette modal.
type Dialog struct {
    open     bool
    input    textinput.Model
    commands []Command
    filtered []Command
    cursor   int // index into filtered
    scroll   int // first visible row of the list
    screenW  int
    screenH  int
}

// NewDialog creates a closed dialog.
func NewDialog() Dialog {
    ti := textinput.New()
    ti.Prompt = ""
    ti.CharLimit = 100

    // Every style needs the dialog background, or the terminal's own
    // background shows through as gaps.
    text := lipgloss.NewStyle().Background(dialogBg).Foreground(colorText)
    ti.PromptStyle = text
    ti.TextStyle = text
    ti.Cursor.Style = lipgloss.NewStyle().Foreground(dialogBlue).Background(dialogBg)
    ti.Cursor.TextStyle = text

    cmds := defaultCommands()
    return Dialog{
        input:    ti,
        commands: cmds,
        filtered: cmds,
    }
}

// IsOpen reports whether the dialog is showing.
func (d Dialog) IsOpen() bool { return d.open }

// SetSize tells the dialog how big the whole screen is.
func (d *Dialog) SetSize(w, h int) {
    d.screenW = w
    d.screenH = h
    d.input.Width = d.innerWidth() - 1
}

// Open shows the dialog with an empty search box.
func (d *Dialog) Open() tea.Cmd {
    d.open = true
    d.input.SetValue("")
    d.input.Focus()
    d.cursor = 0
    d.scroll = 0
    d.refilter()
    return textinput.Blink
}

// Close hides the dialog.
func (d *Dialog) Close() {
    d.open = false
    d.input.Blur()
}

func (d Dialog) width() int {
    w := dialogMaxWidth
    if d.screenW-4 < w {
        w = d.screenW - 4
    }
    if w < 20 {
        w = 20
    }
    return w
}

func (d Dialog) innerWidth() int {
    return d.width() - 2*dialogPadX
}

// refilter rebuilds `filtered` from the search text.
func (d *Dialog) refilter() {
    q := strings.ToLower(strings.TrimSpace(d.input.Value()))
    if q == "" {
        d.filtered = d.commands
    } else {
        out := make([]Command, 0, len(d.commands))
        for _, c := range d.commands {
            if strings.Contains(strings.ToLower(c.Title), q) ||
                strings.Contains(strings.ToLower(c.Section), q) {
                out = append(out, c)
            }
        }
        d.filtered = out
    }
    if d.cursor >= len(d.filtered) {
        d.cursor = len(d.filtered) - 1
    }
    if d.cursor < 0 {
        d.cursor = 0
    }
}

// --- list rows (section headers, blank separators, commands) --------------

const (
    rowBlank = iota
    rowHeader
    rowItem
)

type dialogRow struct {
    kind   int
    header string
    item   int // index into the command list, for rowItem
}

func rowsFor(cmds []Command) []dialogRow {
    var rows []dialogRow
    last := ""
    for i, c := range cmds {
        if c.Section != last {
            if last != "" {
                rows = append(rows, dialogRow{kind: rowBlank})
            }
            rows = append(rows, dialogRow{kind: rowHeader, header: c.Section})
            last = c.Section
        }
        rows = append(rows, dialogRow{kind: rowItem, item: i})
    }
    return rows
}

// maxRows is how many list rows fit on screen.
func (d Dialog) maxRows() int {
    n := d.screenH - 4 - dialogChromeRows
    if n > 18 {
        n = 18
    }
    if n < 3 {
        n = 3
    }
    return n
}

// ensureVisible scrolls the list so the selected command is on screen.
func (d *Dialog) ensureVisible() {
    rows := rowsFor(d.filtered)
    sel := -1
    for i, r := range rows {
        if r.kind == rowItem && r.item == d.cursor {
            sel = i
        }
    }
    if sel < 0 {
        d.scroll = 0
        return
    }
    max := d.maxRows()
    if sel < d.scroll {
        d.scroll = sel
        if sel > 0 && rows[sel-1].kind == rowHeader {
            d.scroll = sel - 1 // keep the section title visible
        }
    }
    if sel >= d.scroll+max {
        d.scroll = sel - max + 1
    }
    if d.scroll > len(rows)-max {
        d.scroll = len(rows) - max
    }
    if d.scroll < 0 {
        d.scroll = 0
    }
}

// Update handles keys and cursor-blink messages while the dialog is open.
func (d Dialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
    if key, ok := msg.(tea.KeyMsg); ok {
        switch key.String() {
        case "esc":
            d.Close()
            return d, nil
        case "enter":
            if len(d.filtered) == 0 {
                return d, nil
            }
            c := d.filtered[d.cursor]
            d.Close()
            return d, func() tea.Msg { return DialogSelectMsg{Command: c} }
        case "up":
            if n := len(d.filtered); n > 0 {
                d.cursor = (d.cursor - 1 + n) % n
                d.ensureVisible()
            }
            return d, nil
        case "down":
            if n := len(d.filtered); n > 0 {
                d.cursor = (d.cursor + 1) % n
                d.ensureVisible()
            }
            return d, nil
        }

        before := d.input.Value()
        var cmd tea.Cmd
        d.input, cmd = d.input.Update(msg)
        if d.input.Value() != before {
            d.cursor = 0
            d.scroll = 0
            d.refilter()
        }
        return d, cmd
    }

    var cmd tea.Cmd
    d.input, cmd = d.input.Update(msg)
    return d, cmd
}

// --- rendering --------------------------------------------------------------

// twoCol puts `left` and `right` on one row, with the gap filled by gapStyle.
func twoCol(left, right string, inner int, gapStyle lipgloss.Style) string {
    gap := inner - lipgloss.Width(left) - lipgloss.Width(right)
    if gap < 1 {
        gap = 1
    }
    return left + gapStyle.Render(strings.Repeat(" ", gap)) + right
}

// searchView draws the search box. The placeholder is drawn by hand so the
// cursor sits on the first letter and no unstyled padding appears.
func (d Dialog) searchView() string {
    if d.input.Value() != "" {
        return d.input.View()
    }
    placeStyle := lipgloss.NewStyle().Background(dialogBg).Foreground(dialogMuted)
    c := d.input.Cursor
    c.TextStyle = placeStyle
    c.SetChar("S")
    return c.View() + placeStyle.Inline(true).Render("earch")
}

// View renders the dialog box itself (without the dimmed background).
func (d Dialog) View() string {
    w := d.width()
    inner := d.innerWidth()
    base := lipgloss.NewStyle().Background(dialogBg)

    blank := base.Render(strings.Repeat(" ", w))
    line := func(content string) string {
        gap := inner - lipgloss.Width(content)
        if gap < 0 {
            gap = 0
        }
        return base.Render(strings.Repeat(" ", dialogPadX)) +
            content +
            base.Render(strings.Repeat(" ", gap+dialogPadX))
    }

    out := []string{blank}

    // Title row: "Commands ........ esc"
    title := twoCol(
        base.Foreground(colorText).Bold(true).Render("Commands"),
        base.Foreground(dialogMuted).Render("esc"),
        inner, base,
    )
    out = append(out, line(title), blank, line(d.searchView()), blank)

    // List.
    rows := rowsFor(d.filtered)
    max := d.maxRows()
    // Fixed height: as tall as the full list (or the screen allows), so the
    // dialog doesn't jump around while you type.
    target := len(rowsFor(d.commands))
    if target > max {
        target = max
    }

    shown := 0
    if len(rows) == 0 {
        out = append(out, line(base.Foreground(dialogMuted).Render("No results")))
        shown++
    }
    for i := d.scroll; i < len(rows) && shown < target; i++ {
        r := rows[i]
        switch r.kind {
        case rowBlank:
            out = append(out, blank)
        case rowHeader:
            out = append(out, line(base.Foreground(dialogPurple).Bold(true).Render(r.header)))
        case rowItem:
            c := d.filtered[r.item]
            if r.item == d.cursor {
                // Selected row: blue bar, edge to edge.
                sel := lipgloss.NewStyle().Background(dialogBlue).Foreground(dialogDark)
                pad := sel.Render(strings.Repeat(" ", dialogPadX))
                content := twoCol(sel.Bold(true).Render(c.Title), sel.Render(c.Shortcut), inner, sel)
                out = append(out, pad+content+pad)
            } else {
                content := twoCol(
                    base.Foreground(colorText).Bold(true).Render(c.Title),
                    base.Foreground(dialogMuted).Render(c.Shortcut),
                    inner, base,
                )
                out = append(out, line(content))
            }
        }
        shown++
    }
    for ; shown < target; shown++ {
        out = append(out, blank)
    }

    out = append(out, blank)
    return strings.Join(out, "\n")
}

// Overlay draws the dialog centered on top of `bg` and dims the background.
func (d Dialog) Overlay(bg string, screenW, screenH int) string {
    box := d.View()
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

const (
    ansiReset = "\x1b[0m"
    ansiFaint = "\x1b[2m" // "slightly reduced opacity": dims text and accent colors
)

// dimLine makes a rendered line faint. Every style reset inside the line
// would cancel the faint attribute, so it is re-applied after each one.
func dimLine(s string) string {
    s = strings.ReplaceAll(s, "\x1b[0m", ansiReset+ansiFaint)
    s = strings.ReplaceAll(s, "\x1b[m", ansiReset+ansiFaint)
    return ansiFaint + s + ansiReset
}

// overlay pastes `fg` over `bg` at column x, row y. Both are ANSI strings.
// The background is dimmed, the foreground is left as is.
func overlay(bg, fg string, x, y int) string {
    bgLines := strings.Split(bg, "\n")
    for i := range bgLines {
        bgLines[i] = dimLine(bgLines[i])
    }

    for i, fl := range strings.Split(fg, "\n") {
        row := y + i
        if row < 0 || row >= len(bgLines) {
            continue
        }
        bl := bgLines[row]
        fw := ansi.StringWidth(fl)

        left := ansi.Truncate(bl, x, "")
        if lw := ansi.StringWidth(left); lw < x {
            left += strings.Repeat(" ", x-lw)
        }
        right := ansi.TruncateLeft(bl, x+fw, "")

        bgLines[row] = left + ansiReset + fl + ansiReset + ansiFaint + right + ansiReset
    }

    return strings.Join(bgLines, "\n")
}
