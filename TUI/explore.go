package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// Calls that only look around (read_file, grep, glob, list_dir) are not folded
// into the "+ 2 thoughts" row. A run of them gets its own row:
//
//	→ Explored: 2 searches                         (open: bright, bold)
//	✱ Grep "workspace" in Backend\internal (120 matches)
//	✱ Grep "cwd|Getwd" in TUI (35 matches)
//
//	→ Explored: 2 reads                            (closed: muted)
//
// Click opens / closes it; hover brightens it (same as the folded rows).

// isExplore reports whether a part is one of the looking-around tool calls.
func isExplore(p Part) bool { return p.Kind == PartTool && p.Explore != "" }

// pluralWord: (1, "read", "reads") -> "1 read", (2, "search", "searches") -> "2 searches".
func pluralWord(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// exploreSummary names a run: "2 reads", "1 read, 2 searches".
func exploreSummary(group []Part) string {
	reads, searches, lists := 0, 0, 0
	for _, p := range group {
		switch p.Explore {
		case "read":
			reads++
		case "search":
			searches++
		case "list":
			lists++
		}
	}
	var items []string
	if reads > 0 {
		items = append(items, pluralWord(reads, "read", "reads"))
	}
	if searches > 0 {
		items = append(items, pluralWord(searches, "search", "searches"))
	}
	if lists > 0 {
		items = append(items, pluralWord(lists, "listing", "listings"))
	}
	return strings.Join(items, ", ")
}

// exploreBlock draws one run of exploring calls. The first returned line is
// the clickable row; rowW is the width of its text (the clickable area).
func exploreBlock(group []Part, width int, hovered bool, spin string) (rows []string, rowW int) {
	pad := strings.Repeat(" ", bubbleIndent)
	open := group[0].Expanded

	tone := tlMuted
	if open {
		tone = colorText
	} else if hovered {
		tone = tlHover
	}
	style := lipgloss.NewStyle().Foreground(tone).Bold(open)

	var header string
	if w := workingPart(group); w != nil {
		// While a call is running the row shows the spinner and what it does.
		text := workingLabel(*w)
		rowW = lipgloss.Width("  " + text) // same width as "⠋ " + text
		header = spin + " " + style.Render(text)
	} else {
		label := "→ Explored: " + exploreSummary(group)
		rowW = lipgloss.Width(label)
		header = style.Render(label)
	}
	rows = []string{pad + header}
	if !open {
		return rows, rowW
	}

	textW := width - bubbleIndent - 1 - 2 // minus "✱ "
	for _, p := range group {
		if p.Working {
			continue // not finished yet, the header row shows it
		}
		rows = append(rows, pad+exploreLine(p, textW))
	}
	return rows, rowW
}

// exploreLine: `✱ Grep "workspace" in Backend\internal (120 matches)`.
func exploreLine(p Part, w int) string {
	glyph := msgMuted.Render("→")
	if p.Tool == "grep" || p.Tool == "glob" {
		glyph = msgMuted.Render("✱")
	}
	text := p.Summary
	style := msgMuted
	switch {
	case p.Failed:
		glyph = toolFailGlyph.Render("✗")
		style = toolErrText
		if p.Err != "" {
			text += " — " + oneLine(p.Err)
		}
	case p.Stopped:
		text += " (stopped)"
	}
	return glyph + " " + style.Render(truncatePlain(text, w))
}
