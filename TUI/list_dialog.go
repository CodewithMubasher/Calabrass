package main

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// ListItem is one row of a ListDialog.
type ListItem struct {
	ID      string
	Title   string
	Desc    string // shown after the title, muted
	Right   string // right-aligned
	Section string // optional group header (e.g. the provider name)
}

// ListSelectMsg is sent when the user presses Enter on a row.
type ListSelectMsg struct {
	Kind string
	Item ListItem
}

// ListKeyMsg is sent for the extra keys a dialog asked for (see ExtraKeys).
type ListKeyMsg struct {
	Kind    string
	Key     string
	Item    ListItem
	HasItem bool
}

// ListDialog is the searchable list modal. One instance serves every list in
// the app (models, sessions, MCP servers, providers); Kind says which.
type ListDialog struct {
	open  bool
	Kind  string
	Title string

	input    textinput.Model
	items    []ListItem
	filtered []ListItem
	cursor   int
	scroll   int
	screenW  int
	screenH  int

	KeepOpen  bool     // Enter does not close the dialog
	Empty     string   // text shown when there are no rows at all
	HintLeft  string   // bottom-left hint
	HintRight string   // bottom-right hint
	ExtraKeys []string // keys that are reported as ListKeyMsg
	Notice    string   // a line under the list (errors, confirmations)
	NoticeErr bool
}

// NewListDialog creates a closed dialog.
func NewListDialog() ListDialog {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 100

	text := lipgloss.NewStyle().Background(dialogBg).Foreground(colorText)
	state := textinput.StyleState{
		Text:        text,
		Placeholder: text.Foreground(dialogMuted),
		Suggestion:  text.Foreground(dialogMuted),
		Prompt:      text,
	}
	ti.SetStyles(textinput.Styles{
		Focused: state,
		Blurred: state,
		Cursor: textinput.CursorStyle{
			Color: dialogBlue,
			Shape: tea.CursorBlock,
			Blink: true,
		},
	})
	ti.Placeholder = "Search"
	return ListDialog{input: ti}
}

// IsOpen reports whether the dialog is showing.
func (d ListDialog) IsOpen() bool { return d.open }

// SetSize tells the dialog how big the whole screen is.
func (d *ListDialog) SetSize(w, h int) {
	d.screenW = w
	d.screenH = h
	d.input.SetWidth(d.innerWidth() - 1)
}

// Show opens the dialog with new content and an empty search box.
func (d *ListDialog) Show(kind, title string, items []ListItem) tea.Cmd {
	d.open = true
	d.Kind = kind
	d.Title = title
	d.items = items
	d.KeepOpen = false
	d.Empty = "Nothing here yet"
	d.HintLeft, d.HintRight = "", ""
	d.ExtraKeys = nil
	d.Notice, d.NoticeErr = "", false
	d.input.SetValue("")
	d.cursor, d.scroll = 0, 0
	d.refilter()
	return d.input.Focus()
}

// Close hides the dialog.
func (d *ListDialog) Close() {
	d.open = false
	d.input.Blur()
}

// SetItems swaps the rows while the dialog is open, keeping the cursor on the
// same row when it is still there.
func (d *ListDialog) SetItems(items []ListItem) {
	keep := ""
	if d.cursor < len(d.filtered) {
		keep = d.filtered[d.cursor].ID
	}
	d.items = items
	d.refilter()
	for i, it := range d.filtered {
		if it.ID == keep {
			d.cursor = i
			break
		}
	}
	d.ensureVisible()
}

// SetNotice shows a line under the list.
func (d *ListDialog) SetNotice(text string, isErr bool) {
	d.Notice, d.NoticeErr = text, isErr
}

// Current returns the highlighted row.
func (d ListDialog) Current() (ListItem, bool) {
	if d.cursor < 0 || d.cursor >= len(d.filtered) {
		return ListItem{}, false
	}
	return d.filtered[d.cursor], true
}

func (d ListDialog) width() int {
	w := dialogMaxWidth
	if d.screenW-4 < w {
		w = d.screenW - 4
	}
	if w < 20 {
		w = 20
	}
	return w
}

func (d ListDialog) innerWidth() int { return d.width() - 2*dialogPadX }

func (d *ListDialog) refilter() {
	q := strings.ToLower(strings.TrimSpace(d.input.Value()))
	if q == "" {
		d.filtered = d.items
	} else {
		out := make([]ListItem, 0, len(d.items))
		for _, it := range d.items {
			if strings.Contains(strings.ToLower(it.Title), q) ||
				strings.Contains(strings.ToLower(it.Desc), q) ||
				strings.Contains(strings.ToLower(it.Section), q) {
				out = append(out, it)
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

// listRow is one rendered row: a group header (-1), a blank separator (-2),
// or an index into d.filtered (>= 0).
type listRow struct {
	section string
	item    int
}

// rows expands the filtered items with their section headers.
func (d ListDialog) rows() []listRow {
	var out []listRow
	last := "\x00" // never equal to a real section
	for i, it := range d.filtered {
		if it.Section != "" && it.Section != last {
			if len(out) > 0 {
				out = append(out, listRow{item: -2})
			}
			out = append(out, listRow{section: it.Section, item: -1})
			last = it.Section
		}
		out = append(out, listRow{item: i})
	}
	return out
}

// maxRows is how many list rows fit on screen.
func (d ListDialog) maxRows() int {
	n := d.screenH - 4 - dialogChromeRows - 2
	if n > 18 {
		n = 18
	}
	if n < 3 {
		n = 3
	}
	return n
}

func (d *ListDialog) ensureVisible() {
	rows := d.rows()
	sel := -1
	for i, r := range rows {
		if r.item == d.cursor {
			sel = i
			break
		}
	}
	if sel < 0 {
		d.scroll = 0
		return
	}

	max := d.maxRows()
	if sel < d.scroll {
		d.scroll = sel
		if sel > 0 && rows[sel-1].item == -1 {
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
func (d ListDialog) Update(msg tea.Msg) (ListDialog, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		d.input, cmd = d.input.Update(msg)
		return d, cmd
	}

	k := key.String()
	for _, extra := range d.ExtraKeys {
		if k == extra {
			item, has := d.Current()
			kind := d.Kind
			return d, func() tea.Msg { return ListKeyMsg{Kind: kind, Key: k, Item: item, HasItem: has} }
		}
	}

	switch k {
	case "esc":
		d.Close()
		return d, nil
	case "enter":
		item, has := d.Current()
		if !has {
			return d, nil
		}
		kind := d.Kind
		if !d.KeepOpen {
			d.Close()
		}
		return d, func() tea.Msg { return ListSelectMsg{Kind: kind, Item: item} }
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
		d.cursor, d.scroll = 0, 0
		d.refilter()
	}
	return d, cmd
}

// View renders the dialog box itself (without the dimmed background).
func (d ListDialog) View() string {
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
	title := twoCol(
		base.Foreground(colorText).Bold(true).Render(d.Title),
		base.Foreground(dialogMuted).Render("esc"),
		inner, base,
	)
	out = append(out, line(title), blank, line(d.input.View()), blank)

	max := d.maxRows()
	rows := d.rows()
	target := len(rows)
	if target > max {
		target = max
	}
	if target < 1 {
		target = 1
	}

	shown := 0
	if len(d.filtered) == 0 {
		msg := "No results"
		if len(d.items) == 0 {
			msg = d.Empty
		}
		out = append(out, line(base.Foreground(dialogMuted).Render(msg)))
		shown++
	}
	for i := d.scroll; i < len(rows) && shown < target; i++ {
		r := rows[i]
		switch {
		case r.item == -2:
			out = append(out, blank)
		case r.item == -1:
			n := 0
			for _, it := range d.filtered {
				if it.Section == r.section {
					n++
				}
			}
			header := base.Foreground(dialogPurple).Bold(true).Render(r.section) +
				base.Foreground(dialogMuted).Render("  "+plural(n, "model"))
			out = append(out, line(header))
		default:
			it := d.filtered[r.item]
			right := it.Right
			avail := inner - lipgloss.Width(right) - 4
			if avail < 4 {
				avail = 4
			}

			if r.item == d.cursor {
				sel := lipgloss.NewStyle().Background(dialogBlue).Foreground(dialogDark)
				pad := sel.Render(strings.Repeat(" ", dialogPadX))
				left := sel.Render(" ") + sel.Bold(true).Render(it.Title)
				if it.Desc != "" {
					left += sel.Render(" " + it.Desc)
				}
				left = ansi.Truncate(left, avail, "")
				out = append(out, pad+twoCol(left, sel.Render(right), inner, sel)+pad)
			} else {
				left := base.Render("  ") + base.Foreground(colorText).Bold(true).Render(it.Title)
				if it.Desc != "" {
					left += base.Foreground(dialogMuted).Render(" " + it.Desc)
				}
				left = ansi.Truncate(left, avail, "")
				out = append(out, line(twoCol(left, base.Foreground(dialogMuted).Render(right), inner, base)))
			}
		}
		shown++
	}
	for ; shown < target; shown++ {
		out = append(out, blank)
	}

	if d.Notice != "" {
		style := base.Foreground(dialogMuted)
		if d.NoticeErr {
			style = base.Foreground(lipgloss.Color("#e06c75"))
		}
		out = append(out, blank, line(style.Render(ansi.Truncate(d.Notice, inner, ""))))
	}

	if d.HintLeft != "" || d.HintRight != "" {
		foot := twoCol(
			base.Foreground(dialogMuted).Render(d.HintLeft),
			base.Foreground(dialogMuted).Render(d.HintRight),
			inner, base,
		)
		out = append(out, blank, line(foot))
	}
	out = append(out, blank)

	return strings.Join(out, "\n")
}

// Overlay draws the dialog centered on top of `bg` and dims the background.
func (d ListDialog) Overlay(bg string, screenW, screenH int) string {
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
