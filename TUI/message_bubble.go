package main

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// Role says who wrote a message.
type Role int

const (
	RoleUser Role = iota
	RoleAssistant
	RoleNotice // a note from the app itself (help text, errors); never sent to the backend
)

// Message is one entry in the chat.
type Message struct {
	ID   int
	Role Role

	// User message and notice: the text.
	Text    string
	IsError bool // notice: draw it as an error

	// ServerID is the backend's id of this message, once it has one.
	ServerID string

	// Assistant message.
	Parts       []Part // the timeline that is visible so far
	Pending     bool   // sent, nothing has arrived yet
	Streaming   bool   // parts are still arriving
	Interrupted bool   // stopped with Esc, or cut short by an error
	Err         string // what went wrong, shown under the answer
	Questions   []askQuestion
	Started     time.Time
	Elapsed     time.Duration
	Label       string // "Chat"
	Model       string

	Accent color.Color // bar color (user) and label color (assistant)

	// version is bumped whenever anything that affects rendering changes.
	// It is used by the render cache to know when to redraw this message.
	version int
}

// hasWorkingPart reports whether a row of the timeline is still being made.
func (m Message) hasWorkingPart() bool {
	for _, p := range m.Parts {
		if p.Working {
			return true
		}
	}
	return false
}

// Chat colors.
var (
	bubbleBg = lipgloss.Color("#1a1a1a")

	bubbleStyle = lipgloss.NewStyle().
			Background(bubbleBg).
			Foreground(colorText).
			Padding(0, 2)

	msgMuted        = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
	msgThoughtTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("#b8862f")).Bold(true)
	msgThoughtBar   = lipgloss.NewStyle().Foreground(lipgloss.Color("#333333"))
)

// bubbleIndent is how far assistant text is pushed in so it lines up with
// the text inside the user bubble (bar 1 + padding 2).
const bubbleIndent = 3

// wrapText word-wraps plain text to `w` columns.
func wrapText(text string, w int) string {
	if w < 1 {
		w = 1
	}
	return lipgloss.NewStyle().Width(w).Render(text)
}

// barBlock draws lines with a thin dark bar on the left (used by thoughts).
func barBlock(lines []string) []string {
	pad := strings.Repeat(" ", bubbleIndent)
	bar := msgThoughtBar.Render("│") + " "
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = pad + bar + l
	}
	return out
}

// Render draws the message `width` columns wide. For every line it also
// returns whether the line is a clickable timeline row. hover is the
// timeline row under the mouse (first-part index) or -1.
func (m Message) Render(width, hover int, spin string) ([]string, []lineHit) {
	switch m.Role {
	case RoleUser:
		lines := m.renderUser(width)
		return lines, noHits(len(lines))
	case RoleNotice:
		lines := m.renderNotice(width)
		return lines, noHits(len(lines))
	}
	return m.renderAssistant(width, hover, spin)
}

// --- render cache -----------------------------------------------------------
//
// Messages are re-rendered on every frame: scrolling, hovering, the spinner
// ticking, the terminal resizing. With 100+ messages that is what makes the
// UI feel slow. Every message caches its rendered lines + hit map, keyed by
// everything that affects the output. Only the streaming message is
// invalidated on each step, so the rest of the timeline is essentially free.

type msgKey struct {
	id      int
	version int
	width   int
	hover   int    // hoverPart, or -1
	spin    string // spinner frame (only set while the message is animating)
}

type cachedMsg struct {
	key   msgKey
	lines []string
	hits  []lineHit
}

var msgCache = map[int]cachedMsg{} // Message.ID -> cached render

// RenderCached returns the message's rendered lines, using a cache so that
// only the parts of the timeline that actually changed are re-rendered.
func (m Message) RenderCached(width, hover int, spin string) ([]string, []lineHit) {
	// The spinner only changes the look of messages that are still animating.
	spinKey := ""
	if m.Streaming || m.Pending || m.hasWorkingPart() {
		spinKey = spin
	}

	key := msgKey{id: m.ID, version: m.version, width: width, hover: hover, spin: spinKey}
	if c, ok := msgCache[m.ID]; ok && c.key == key {
		return c.lines, c.hits
	}

	lines, hits := m.Render(width, hover, spinKey)
	if len(msgCache) > 400 {
		msgCache = map[int]cachedMsg{} // keep memory bounded across sessions
	}
	msgCache[m.ID] = cachedMsg{key: key, lines: lines, hits: hits}
	return lines, hits
}

// invalidateMsgRender forces this message to be re-rendered next time.
func (m *Message) invalidateMsgRender() {
	m.version++
}

// renderUser: accent bar + grey card with the text, then a blank row.
func (m Message) renderUser(width int) []string {
	cardW := width - 1
	textW := cardW - 4 // card has 2 columns of padding on each side
	text := wrapText(m.Text, textW)
	h := lipgloss.Height(text)

	// One blank row above and below the text.
	card := renderBubbleCard(bubbleStyle, m.Accent, width, h+2, "\n"+text+"\n")

	lines := strings.Split(card, "\n")
	return append(lines, "")
}

// renderNotice: a muted (or red) note from the app, indented like an answer.
func (m Message) renderNotice(width int) []string {
	pad := strings.Repeat(" ", bubbleIndent)
	style := msgMuted
	if m.IsError {
		style = toolErrText
	}
	var lines []string
	for _, raw := range strings.Split(m.Text, "\n") {
		for _, l := range strings.Split(wrapText(raw, width-bubbleIndent-1), "\n") {
			lines = append(lines, pad+style.Render(l))
		}
	}
	return append(lines, "")
}

// renderAssistant: the timeline, then the footer once the answer is complete.
func (m Message) renderAssistant(width, hover int, spin string) ([]string, []lineHit) {
	pad := strings.Repeat(" ", bubbleIndent)

	if m.Pending || (m.Streaming && len(m.Parts) == 0) {
		lines := []string{pad + spin + " " + msgMuted.Render("Thinking"), ""}
		return lines, noHits(len(lines))
	}

	lines, hits := renderTimeline(m.Parts, width, hover, m.Streaming, spin)

	if m.Err != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
			hits = append(hits, noHit)
		}
		for _, l := range strings.Split(wrapText(m.Err, width-bubbleIndent-1), "\n") {
			lines = append(lines, pad+toolErrText.Render(l))
			hits = append(hits, noHit)
		}
	}

	// Footer ("Chat · Model · 9.0s") appears when the answer is complete.
	if !m.Streaming {
		var info string
		if m.Model != "" {
			info += " · " + m.Model
		}
		if m.Elapsed > 0 {
			info += " · " + fmt.Sprintf("%.1fs", m.Elapsed.Seconds())
		}
		if m.Interrupted {
			info += " · interrupted"
		}
		footer := lipgloss.NewStyle().Foreground(m.Accent).Bold(true).Render(m.Label) +
			msgMuted.Render(info)
		lines = append(lines, "", pad+footer)
		hits = append(hits, noHit, noHit)
	}
	lines = append(lines, "")
	hits = append(hits, noHit)

	return lines, hits
}
