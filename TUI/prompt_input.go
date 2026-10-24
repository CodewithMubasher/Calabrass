package main

import (
    "strings"

    "github.com/charmbracelet/bubbles/textarea"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

// Card layout: top padding, the input (1 row, grows while you type several
// lines), a blank row and the status line, plus a bottom half-row cap.
// The input has the same space above it (padding) as below it (blank row),
// and the status line sits close to the bottom edge.
const (
    promptChromeRows  = 3 // top padding + blank row + status line
    promptMaxInputRows = 8 // the input grows up to this many rows, then scrolls
)

// Leave empty to show only the cursor (like the reference design).
const promptPlaceholder = ""

// Colors for the prompt-input card.
var (
    promptBg      = lipgloss.Color("#1e1e1e") // card background

    // Accent colors per mode.
    promptAccentChat  = lipgloss.Color("#5bc0de") // blue for chat mode
    promptAccentCrew  = lipgloss.Color("#9b59b6") // purple for crew mode
    promptAccentDefault = lipgloss.Color("#5c9cf5") // default/Build color

    promptMuted   = lipgloss.Color("#808080") // dots and provider
    promptPlaceFg = lipgloss.Color("#777777") // placeholder text

    // Card style: only horizontal padding, vertical rows are laid out by hand.
    promptCardStyle = lipgloss.NewStyle().
            Background(promptBg).
            Foreground(colorText).
            Padding(0, 2)
)

// PromptSubmitMsg is sent when the user presses Enter in the prompt-input.
type PromptSubmitMsg struct {
    Value string
}

// PromptInput is the "prompt-input" component: a card with a text field
// and a status line underneath.
type PromptInput struct {
    input    textarea.Model
    width    int // full card width, including the left bar and padding
    innerW   int // width of the text area itself
    Mode     string
    Placeholder string // text shown while the input is empty
    Model    string
    Provider string
}

// NewPromptInput creates a focused prompt-input.
func NewPromptInput() PromptInput {
    ta := textarea.New()
    ta.Prompt = "" // no "┃" in front of every line
    ta.ShowLineNumbers = false
    ta.CharLimit = 4000
    ta.SetHeight(1)
    ta.Focus()

    // Every inner style needs the card background, otherwise the text
    // resets the color and leaves black gaps inside the card.
    base := lipgloss.NewStyle().Background(promptBg).Foreground(colorText)
    style := textarea.Style{
        Base:             base,
        CursorLine:       base, // no highlighted line
        CursorLineNumber: base,
        EndOfBuffer:      base,
        LineNumber:       base,
        Placeholder:      base.Foreground(promptPlaceFg),
        Prompt:           base,
        Text:             base,
    }
    ta.FocusedStyle = style
    ta.BlurredStyle = style
    ta.Cursor.Style = base
    ta.Cursor.TextStyle = base

    return PromptInput{
        input:    ta,
        Mode:     "chat",      // start in chat mode
        Model:    "Gemini-3.5-flash",
        Provider: "Google",
    }
}

// Init starts the cursor blinking.
func (p PromptInput) Init() tea.Cmd {
    return textarea.Blink
}

// SetWidth sets the full card width (bar and padding included).
func (p *PromptInput) SetWidth(w int) {
    p.width = w
    // card width - left bar (1) - horizontal padding (4) - cursor (1)
    inner := w - 1 - 4 - 1
    if inner < 1 {
        inner = 1
    }
    p.innerW = inner
    p.input.SetWidth(inner)
    p.fitHeight()
}

// inputRows is how many rows the text needs at the current width.
func (p PromptInput) inputRows() int {
    w := p.innerW
    if w < 1 {
        w = 1
    }
    rows := 0
    for _, line := range strings.Split(p.input.Value(), "\n") {
        rows += lipgloss.Height(lipgloss.NewStyle().Width(w).Render(line))
    }
    if rows < 1 {
        rows = 1
    }
    if rows > promptMaxInputRows {
        rows = promptMaxInputRows
    }
    return rows
}

// fitHeight makes the text area as tall as its text (1 row when empty).
func (p *PromptInput) fitHeight() {
    p.input.SetHeight(p.inputRows())
}

// Update handles keys and cursor-blink messages.
func (p PromptInput) Update(msg tea.Msg) (PromptInput, tea.Cmd) {
    if key, ok := msg.(tea.KeyMsg); ok {
        switch key.String() {
        case "enter":
            // Enter sends.
            value := strings.TrimSpace(p.input.Value())
            if value == "" {
                return p, nil
            }
            p.input.SetValue("")
            p.fitHeight()
            return p, func() tea.Msg { return PromptSubmitMsg{Value: value} }
        case "ctrl+j", "ctrl+enter", "alt+enter", "shift+enter":
            // New line. Most terminals send Ctrl+Enter as Ctrl+J; Alt+Enter
            // works in all of them.
            p.input.InsertString("\n")
            p.fitHeight()
            return p, nil
        case "tab", "shift+tab":
            // Cycle modes: chat -> crew -> chat
            p.CycleMode()
            return p, nil
        }
    }

    var cmd tea.Cmd
    p.input, cmd = p.input.Update(msg)
    p.fitHeight()
    return p, cmd
}

// bodyHeight is the number of full rows in the card.
func (p PromptInput) bodyHeight() int {
    return promptChromeRows + p.inputRows()
}

// Height is the total rows the card occupies (body + the bottom cap row).
func (p PromptInput) Height() int {
    return p.bodyHeight() + 1
}

// placeholderView draws the placeholder by hand. The built-in one pads its
// right side without the card background, which showed up as a black strip.
// Here only the text is drawn and the card's own padding fills the rest.
func (p PromptInput) placeholderView() string {
    placeStyle := lipgloss.NewStyle().Background(promptBg).Foreground(promptPlaceFg)

    c := p.input.Cursor
    c.TextStyle = placeStyle

    runes := []rune(p.Placeholder)
    if len(runes) == 0 {
        c.SetChar(" ")
        return c.View()
    }
    c.SetChar(string(runes[0]))
    return c.View() + placeStyle.Inline(true).Render(string(runes[1:]))
}

// statusView is the line under the input: "Chat · Model Provider" or "Crew · Model Provider".
func (p PromptInput) statusView() string {
    base := lipgloss.NewStyle().Background(promptBg).Inline(true)

    // Choose accent color and label based on mode.
    var accent lipgloss.Color
    var label string
    switch p.Mode {
    case "chat":
        accent = promptAccentChat
        label = "Chat"
    case "crew":
        accent = promptAccentCrew
        label = "Crew"
    default:
        accent = promptAccentDefault
        label = "Build"
    }

    return base.Foreground(accent).Bold(true).Render(label) +
        base.Foreground(promptMuted).Render(" · ") +
        base.Foreground(colorText).Render(p.Model) +
        base.Foreground(promptMuted).Render(" "+p.Provider)
}

// View renders the card.
func (p PromptInput) View() string {
    input := p.input.View()
    if p.input.Value() == "" {
        input = p.placeholderView()
    }

    // Determine accent color based on mode.
    var accent lipgloss.Color
    switch p.Mode {
    case "chat":
        accent = promptAccentChat
    case "crew":
        accent = promptAccentCrew
    default:
        accent = promptAccentDefault
    }

    // rows: top padding / input / blank / status
    content := strings.Join([]string{"", input, "", p.statusView()}, "\n")

    return renderBarCard(promptCardStyle, accent, p.width, p.bodyHeight(), content)
}

// ModeStyle returns the accent color and label of the current mode.
func (p PromptInput) ModeStyle() (lipgloss.Color, string) {
    switch p.Mode {
    case "chat":
        return promptAccentChat, "Chat"
    case "crew":
        return promptAccentCrew, "Crew"
    }
    return promptAccentDefault, "Build"
}

// Value returns the text in the input.
func (p PromptInput) Value() string { return p.input.Value() }

// SetValue replaces the text and puts the cursor at the end.
func (p *PromptInput) SetValue(v string) {
    p.input.SetValue(v)
    p.input.CursorEnd()
    p.fitHeight()
}

// CycleMode switches chat <-> crew (same as pressing tab).
func (p *PromptInput) CycleMode() {
    if p.Mode == "chat" {
        p.Mode = "crew"
    } else {
        p.Mode = "chat"
    }
}
