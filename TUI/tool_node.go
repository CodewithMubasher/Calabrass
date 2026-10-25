package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// maxToolLines is how many lines of a tool's output an open row shows.
const maxToolLines = 12

var (
	toolOKGlyph   = lipgloss.NewStyle().Foreground(lipgloss.Color("#85d68e")).Bold(true)
	toolFailGlyph = lipgloss.NewStyle().Foreground(lipgloss.Color("#e06c75")).Bold(true)
	toolErrText   = lipgloss.NewStyle().Foreground(lipgloss.Color("#e06c75"))
)

// toolNode draws one tool call of an open row:
//
//	│ ✓ Read main.py                          12ms
//	│   1  def add(a, b): ...
func toolNode(p Part, width int) []string {
	blockW := width - bubbleIndent - 2 - 1
	if blockW < 8 {
		blockW = 8
	}

	glyph := toolOKGlyph.Render("✓")
	switch {
	case p.Failed:
		glyph = toolFailGlyph.Render("✗")
	case p.Stopped:
		glyph = msgMuted.Render("−")
	}

	label := p.Label
	if p.Stopped {
		label += " (stopped)"
	}
	right := ""
	if p.DurMs > 0 {
		right = msgMuted.Render(formatMs(int(p.DurMs)))
	}
	left := glyph + " " + lipgloss.NewStyle().Foreground(colorText).Render(truncatePlain(label, blockW-lipgloss.Width(right)-3))
	gap := blockW - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	lines := barBlock([]string{left + strings.Repeat(" ", gap) + right})

	var body []string
	if p.Err != "" {
		body = append(body, clipText(p.Err, blockW, maxToolLines, toolErrText)...)
	}
	if strings.TrimSpace(p.Output) != "" {
		body = append(body, clipText(p.Output, blockW, maxToolLines, msgMuted)...)
	}
	if len(body) > 0 {
		lines = append(lines, barBlock(body)...)
	}
	return lines
}

// clipText wraps text to w columns and keeps at most max lines.
func clipText(text string, w, max int, style lipgloss.Style) []string {
	text = strings.ReplaceAll(strings.TrimRight(text, "\n \t"), "\t", "    ")
	var out []string
	hidden := 0
	for _, raw := range strings.Split(text, "\n") {
		wrapped := strings.Split(wrapText(raw, w), "\n")
		for _, l := range wrapped {
			if len(out) >= max {
				hidden++
				continue
			}
			out = append(out, style.Render(l))
		}
	}
	if hidden > 0 {
		out = append(out, msgMuted.Render(fmt.Sprintf("… %d more lines", hidden)))
	}
	return out
}

// truncatePlain cuts s to w columns with an ellipsis.
func truncatePlain(s string, w int) string {
	if w < 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

// formatMs: 579 -> "579ms", 1234 -> "1.2s".
func formatMs(ms int) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}

// countLines is the number of lines in text (a trailing newline adds none).
func countLines(text string) int {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

// firstLines keeps the first n lines of text.
func firstLines(text string, n int) string {
	text = strings.TrimRight(text, "\n")
	if n < 1 {
		return ""
	}
	idx := 0
	for i := 0; i < n; i++ {
		j := strings.IndexByte(text[idx:], '\n')
		if j < 0 {
			return text
		}
		idx += j + 1
	}
	return text[:idx-1]
}
