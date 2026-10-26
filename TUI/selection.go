package main

import (
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Drag-to-select: a click and drag highlights anything on the finished screen
// (chat, panel, prompt, footer, toast, even a modal). ctrl+c copies it. A plain
// click still opens and closes a timeline row; only a real drag selects.

// selPoint is a cell on the rendered screen: x is a display column, y a row.
type selPoint struct{ x, y int }

// selection is the highlighted range. anchor is where the drag started and
// cursor is where it is now, so dragging up/left works as well as down/right.
type selection struct {
	active bool
	anchor selPoint
	cursor selPoint
}

// selSGR is the selection style as raw escapes: a solid blue background and
// white text (#264f78). It is inserted into already-rendered lines, so the
// text keeps its own colour everywhere the selection is not.
const selSGR = "\x1b[48;2;38;79;120m\x1b[38;2;255;255;255m"

// quitWindow is how soon a second ctrl+c must follow the first to quit.
const quitWindow = 2 * time.Second

// selectionRange returns the top-left and bottom-right of the selection,
// whichever way the drag went.
func (m Model) selectionRange() (selPoint, selPoint) {
	a, b := m.sel.anchor, m.sel.cursor
	if b.y < a.y || (b.y == a.y && b.x < a.x) {
		a, b = b, a
	}
	return a, b
}

// selectionText is the selected screen text, line by line. Trailing spaces are
// trimmed so a selection that runs to the edge of the screen stays clean.
func (m Model) selectionText() string {
	if !m.sel.active {
		return ""
	}
	lines := strings.Split(m.renderScreen(), "\n")
	a, b := m.selectionRange()

	out := make([]string, 0, b.y-a.y+1)
	for y := a.y; y <= b.y; y++ {
		if y < 0 || y >= len(lines) {
			continue
		}
		plain := ansi.Strip(lines[y])
		w := ansi.StringWidth(plain)

		from, to := 0, w
		if y == a.y {
			from = a.x
		}
		if y == b.y {
			to = b.x + 1 // the cell under the cursor is included
		}
		if from < 0 {
			from = 0
		}
		if to > w {
			to = w
		}
		if from >= to {
			out = append(out, "")
			continue
		}
		out = append(out, strings.TrimRight(ansi.Cut(plain, from, to), " "))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// withSelection paints the highlighted range over a finished screen.
func (m Model) withSelection(view string) string {
	if !m.sel.active {
		return view
	}
	lines := strings.Split(view, "\n")
	a, b := m.selectionRange()

	for y := a.y; y <= b.y; y++ {
		if y < 0 || y >= len(lines) {
			continue
		}
		line := lines[y]
		w := ansi.StringWidth(line)

		from, to := 0, w
		if y == a.y {
			from = a.x
		}
		if y == b.y {
			to = b.x + 1
		}
		if from < 0 {
			from = 0
		}
		if to > w {
			to = w
		}
		if from >= to {
			continue
		}
		lines[y] = highlightSpan(line, from, to)
	}
	return strings.Join(lines, "\n")
}

// highlightSpan paints columns [from, to) of one rendered line. The original
// escapes are left in place, so the line keeps its colours outside the span.
func highlightSpan(line string, from, to int) string {
	start := byteAtColumn(line, from)
	end := byteAtColumn(line, to)
	if start >= end {
		return line
	}

	mid := reinforceSelection(line[start:end])
	// Replaying the escapes from the start of the line restores exactly the
	// style that was active where the selection ends.
	restore := sgrBefore(line[:end])
	return line[:start] + selSGR + mid + ansiReset + restore + line[end:]
}

// reinforceSelection re-applies the selection background after every style
// escape inside the selected text, so a nested style cannot punch a hole in it.
func reinforceSelection(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if seq, n := escapeAt(s, i); n > 0 {
			b.WriteString(seq)
			if strings.HasSuffix(seq, "m") {
				b.WriteString(selSGR)
			}
			i += n
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// sgrBefore collects every SGR escape up to the byte offset, in order. Replaying
// them rebuilds the style state at that point.
func sgrBefore(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if seq, n := escapeAt(s, i); n > 0 {
			if strings.HasSuffix(seq, "m") {
				b.WriteString(seq)
			}
			i += n
			continue
		}
		i++
	}
	return b.String()
}

// escapeAt returns the escape sequence starting at i, and its length. It returns
// a zero length when s[i] does not start a sequence.
func escapeAt(s string, i int) (string, int) {
	if i >= len(s) || s[i] != 0x1b {
		return "", 0
	}
	if i+1 >= len(s) {
		return s[i:], len(s) - i
	}
	switch s[i+1] {
	case '[': // CSI: parameters, then a final byte
		j := i + 2
		for j < len(s) && !(s[j] >= 0x40 && s[j] <= 0x7e) {
			j++
		}
		if j < len(s) {
			j++
		}
		return s[i:j], j - i
	case ']': // OSC: ends at BEL or ESC \
		j := i + 2
		for j < len(s) {
			if s[j] == 0x07 {
				j++
				break
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				j += 2
				break
			}
			j++
		}
		return s[i:j], j - i
	default: // two-byte sequence
		return s[i : i+2], 2
	}
}

// byteAtColumn returns the byte offset where display column col starts. Escape
// sequences take no columns, and wide characters count as two.
func byteAtColumn(s string, col int) int {
	if col <= 0 {
		return 0
	}
	w := 0
	for i := 0; i < len(s); {
		if _, n := escapeAt(s, i); n > 0 {
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		rw := ansi.StringWidth(string(r))
		if w+rw > col {
			return i
		}
		w += rw
		i += size
		if w == col {
			return i
		}
	}
	return len(s)
}

// --- mouse ------------------------------------------------------------------

// press starts a possible drag. Nothing is selected until the mouse moves.
func (m *Model) press(x, y int) {
	m.selPress = selPoint{x: x, y: y}
	m.selPressing = true
	m.selDragging = false
}

// drag extends the selection while the left button is held.
func (m *Model) drag(x, y int) {
	if !m.selPressing {
		return
	}
	if !m.selDragging {
		// A little jitter should still count as a click, not a selection.
		dx, dy := x-m.selPress.x, y-m.selPress.y
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		if dx < 2 && dy < 2 {
			return
		}
		m.selDragging = true
		m.sel.active = true
		m.sel.anchor = m.selPress
	}
	m.sel.cursor = selPoint{x: x, y: y}
}

// release ends a drag. A click that never moved clears the selection and opens
// or closes the timeline row under it, as before.
func (m *Model) release(x, y int) {
	if !m.selPressing {
		return
	}
	m.selPressing = false
	if m.selDragging {
		m.selDragging = false
		m.sel.cursor = selPoint{x: x, y: y}
		if strings.TrimSpace(m.selectionText()) == "" {
			m.sel = selection{}
		}
		return
	}
	m.sel = selection{}
	if !m.modalOpen() {
		m.handleClick(x, y)
	}
}

// clearSelection drops the highlight. Any key except ctrl+c does this.
func (m *Model) clearSelection() {
	m.sel = selection{}
	m.selDragging = false
}

// onCtrlC copies a selection when there is one. Without one, the first press
// arms quitting and the second press within quitWindow exits.
func (m Model) onCtrlC() (tea.Model, tea.Cmd) {
	if text := m.selectionText(); strings.TrimSpace(text) != "" {
		m.clearSelection()
		m.quitArmed = false
		toast := m.showToast("Copied the selection.", false)
		return m, tea.Batch(toast, copyCmd(text))
	}

	if m.running() {
		m.interruptTurn()
		return m, nil
	}

	if m.quitArmed && time.Since(m.quitAt) < quitWindow {
		return m, tea.Quit
	}
	m.quitArmed = true
	m.quitAt = time.Now()
	return m, m.showToast("Press ctrl+c again to quit.", false)
}
