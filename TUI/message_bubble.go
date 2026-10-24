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
)

// Message is one entry in the chat.
type Message struct {
	ID   int
	Role Role

	// User message: what the user typed.
	Text string

	// Assistant message.
	Query       string // what the user asked (used by the fake model)
	Parts       []Part // the timeline that is visible so far
	Script      []Part // the whole answer the fake model is "typing"
	ScriptIdx   int    // next part of Script to show
	Pos         int    // characters of the current text part already shown
	Pending     bool   // still "thinking", nothing to show yet
	Streaming   bool   // parts are still arriving
	Interrupted bool   // stopped with Esc
	Started     time.Time
	Elapsed     time.Duration
	Label       string // "Chat" / "Crew" / "Build"
	Model       string

	Accent color.Color // bar color (user) and label color (assistant)
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
	bar := msgThoughtBar.Render("▎") + " "
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = pad + bar + l
	}
	return out
}

// charCount is used for the fake token counter.
func (m Message) charCount() int {
	n := len([]rune(m.Text))
	for _, p := range m.Parts {
		n += len([]rune(p.Text)) + len([]rune(p.Code))
		for _, d := range p.Hunk {
			n += len([]rune(d.Text))
		}
	}
	return n
}

// Render draws the message `width` columns wide. For every line it also
// returns whether the line is a clickable timeline row. hover is the
// timeline row under the mouse (first-part index) or -1.
func (m Message) Render(width, hover int) ([]string, []lineHit) {
	if m.Role == RoleUser {
		lines := m.renderUser(width)
		return lines, noHits(len(lines))
	}
	return m.renderAssistant(width, hover)
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

// renderAssistant: the timeline, then the footer once the answer is complete.
func (m Message) renderAssistant(width, hover int) ([]string, []lineHit) {
	pad := strings.Repeat(" ", bubbleIndent)

	if m.Pending {
		lines := []string{pad + msgMuted.Render("Thinking…"), ""}
		return lines, noHits(len(lines))
	}

	lines, hits := renderTimeline(m.Parts, width, hover, m.Streaming)

	// Footer ("Build · Model · 9.0s") appears when the answer is complete.
	if !m.Streaming {
		info := " · " + m.Model + " · " + fmt.Sprintf("%.1fs", m.Elapsed.Seconds())
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
