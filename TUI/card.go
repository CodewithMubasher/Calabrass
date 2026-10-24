package main

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// capLines returns the half-block top and bottom rows for a card.
// Each one paints only HALF of a terminal row, so the card's visible
// top/bottom margin is half a row (about the width of one column).
// That is what makes top, bottom, left and right margins look identical.
// The color is taken from the card style's background.
func capLines(style lipgloss.Style, width int) (string, string) {
	capStyle := lipgloss.NewStyle().Foreground(style.GetBackground())
	top := capStyle.Render(strings.Repeat("▄", width))
	bottom := capStyle.Render(strings.Repeat("▀", width))
	return top, bottom
}

// renderCard draws a plain card `width` columns wide and bodyHeight rows
// tall, plus the two half-block caps.
func renderCard(style lipgloss.Style, width, bodyHeight int, content string) string {
	top, bottom := capLines(style, width)

	body := style.
		Width(width).
		Height(bodyHeight).
		Render(content)

	return lipgloss.JoinVertical(lipgloss.Left, top, body, bottom)
}

// renderAccentCard draws:  bar + card
// The bar is a thin half-column line that spans the card's full height
// (body + both half-block caps). The empty right half of the bar's column
// is the gap, so the line sits close to the card but not touching it.
// bar column (1) + card take `width` columns in total.
func renderAccentCard(style lipgloss.Style, accent color.Color, width, bodyHeight int, content string) string {
	const barW = 1

	cardW := width - barW
	if cardW < 1 {
		cardW = 1
	}

	// Card: caps + body, exactly like renderCard.
	card := renderCard(style, cardW, bodyHeight, content)

	// Bar: thin (left half of the cell). Quadrant blocks give the same
	// half-row caps as the card: ▖ (lower-left) on top, ▘ (upper-left) at
	// the bottom, ▌ (left half) in between.
	barStyle := lipgloss.NewStyle().Foreground(accent)
	bar := make([]string, 0, bodyHeight+2)
	bar = append(bar, barStyle.Render("▖"))
	for i := 0; i < bodyHeight; i++ {
		bar = append(bar, barStyle.Render("▌"))
	}
	bar = append(bar, barStyle.Render("▘"))

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		strings.Join(bar, "\n"),
		card,
	)
}

// Bar glyphs for renderBarCard. "▌" fills the left half of a column; "▘"
// is the same width but only half a row tall, so the bar ends exactly where
// the card's bottom half-row cap ends. For an even thinner bar use "▎" for
// barGlyph (the little end piece then looks slightly wider than the bar).
const (
	barGlyph = "▌"
	barNub   = "▘"
)

// renderBarCard is a card with a thin accent bar on the left:
//
//	bodyHeight full rows, then a bottom half-row cap (▀).
//
// The half-row cap gives a small, even bottom padding and leaves a clear gap
// before whatever is drawn under the card. The bar covers the same height,
// including a half-row end piece next to the cap.
func renderBarCard(style lipgloss.Style, accent color.Color, width, bodyHeight int, content string) string {
	const barW = 1

	cardW := width - barW
	if cardW < 1 {
		cardW = 1
	}

	body := style.
		Width(cardW).
		Height(bodyHeight).
		MaxHeight(bodyHeight).
		Render(content)

	capStyle := lipgloss.NewStyle().Foreground(style.GetBackground())
	bottom := capStyle.Render(strings.Repeat("▀", cardW))
	card := lipgloss.JoinVertical(lipgloss.Left, body, bottom)

	barStyle := lipgloss.NewStyle().Foreground(accent)
	bar := make([]string, 0, bodyHeight+1)
	for i := 0; i < bodyHeight; i++ {
		bar = append(bar, barStyle.Render(barGlyph))
	}
	bar = append(bar, barStyle.Render(barNub))

	return lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(bar, "\n"), card)
}

// renderBubbleCard is a card with the accent bar on the left, drawn in full
// rows only (no half-row caps): every row of the card has a bar next to it.
// Put blank rows around the text in `content` for padding.
func renderBubbleCard(style lipgloss.Style, accent color.Color, width, bodyHeight int, content string) string {
	cardW := width - 1
	if cardW < 1 {
		cardW = 1
	}

	body := style.
		Width(cardW).
		Height(bodyHeight).
		MaxHeight(bodyHeight).
		Render(content)

	barStyle := lipgloss.NewStyle().Foreground(accent)
	bar := make([]string, bodyHeight)
	for i := range bar {
		bar[i] = barStyle.Render(barGlyph)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(bar, "\n"), body)
}
