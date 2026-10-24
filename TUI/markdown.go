package main

import (
    "strings"

    "github.com/charmbracelet/glamour"
    gansi "github.com/charmbracelet/glamour/ansi"
    "github.com/charmbracelet/glamour/styles"
    "github.com/charmbracelet/x/ansi"
)

// Markdown colors.
const (
    mdAccent    = "#d97757" // headings and list numbers
    mdCodeGreen = "#85d68e" // inline code
)

// Rendering Markdown is slow, and the chat is redrawn on every key press or
// mouse move, so finished results are cached by (text, width).
type mdKey struct {
    text  string
    width int
}

var mdCache = map[mdKey][]string{}

// markdownLines renders Markdown text to terminal lines, `width` columns wide.
func markdownLines(text string, width int) []string {
    if width < 10 {
        width = 10
    }
    key := mdKey{text, width}
    if lines, ok := mdCache[key]; ok {
        return lines
    }
    lines := renderMarkdown(text, width)
    if len(mdCache) > 200 {
        mdCache = map[mdKey][]string{} // keep memory bounded (e.g. many resizes)
    }
    mdCache[key] = lines
    return lines
}

func renderMarkdown(text string, width int) []string {
    // Glamour's dark theme, without the outer margin and blank lines, so the
    // text lines up with the rest of the chat.
    zero := uint(0)
    style := styles.DarkStyleConfig
    style.Document.Margin = &zero
    style.Document.BlockPrefix = ""
    style.Document.BlockSuffix = ""

    // Glamour's dark theme prints "## " / "### " in front of headings.
    // Draw real headings instead: no marker, just the accent color + bold.
    for _, h := range []*gansi.StyleBlock{
        &style.H1, &style.H2, &style.H3, &style.H4, &style.H5, &style.H6,
    } {
        h.Prefix = ""
        h.Suffix = ""
        h.Color = sp(mdAccent)
        h.BackgroundColor = nil
        h.Bold = bp(true)
    }

    // Ordered list numbers ("1." "2." ...) get the accent color too.
    style.Enumeration.Color = sp(mdAccent)
    style.Enumeration.Bold = bp(true)

    // Inline code (file names, commands): just green text, no pill.
    style.Code.Prefix = ""
    style.Code.Suffix = ""
    style.Code.BackgroundColor = nil
    style.Code.Color = sp(mdCodeGreen)

    r, err := glamour.NewTermRenderer(
        glamour.WithStyles(style),
        glamour.WithWordWrap(width),
    )
    if err != nil {
        return plainLines(text, width)
    }
    out, err := r.Render(text)
    if err != nil {
        return plainLines(text, width)
    }

    lines := strings.Split(out, "\n")

    // Drop blank lines at the start and the end.
    for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[0])) == "" {
        lines = lines[1:]
    }
    for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
        lines = lines[:len(lines)-1]
    }

    // Long code lines are not wrapped by glamour; cut them so they can't
    // break the layout.
    for i := range lines {
        lines[i] = ansi.Truncate(lines[i], width, "")
    }
    return lines
}

// plainLines is the fallback when Markdown rendering fails.
func plainLines(text string, width int) []string {
    return strings.Split(wrapText(text, width), "\n")
}

func sp(s string) *string { return &s }
func bp(b bool) *bool     { return &b }

// While an answer is being typed, the same text is drawn many times per step
// (every redraw, every mouse move). Remember just the last result.
var streamCache struct {
    text  string
    width int
    lines []string
    ok    bool
}

// streamMarkdownLines renders text that is still growing, without filling the
// main cache with half-finished texts.
func streamMarkdownLines(text string, width int) []string {
    if width < 10 {
        width = 10
    }
    if streamCache.ok && streamCache.text == text && streamCache.width == width {
        return streamCache.lines
    }
    lines := renderMarkdown(text, width)
    streamCache.text, streamCache.width, streamCache.lines, streamCache.ok = text, width, lines, true
    return lines
}
