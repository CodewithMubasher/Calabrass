package main

import (
    "fmt"
    "strings"
    "time"

    "charm.land/lipgloss/v2"
)

// Screen says which screen the app is showing.
type Screen int

const (
    screenHome Screen = iota // logo + centered prompt (start screen)
    screenChat               // panels + chat (after the first message)
)

const (
    appName         = "Calabrass"
    appVersion      = "0.1.0"
    homePlaceholder = `Ask anything "Fix a TODO in the codebase"`
    homePromptMax   = 72 // widest the prompt gets on the home screen
)

// newHomePrompt is the prompt-input with the home placeholder.
func newHomePrompt() PromptInput {
    p := NewPromptInput()
    p.Placeholder = homePlaceholder
    return p
}

// homePromptWidth is the prompt width on the home screen.
func (m Model) homePromptWidth() int {
    w := homePromptMax
    if m.width-4 < w {
        w = m.width - 4
    }
    if w < 10 {
        w = 10
    }
    return w
}

// syncPromptWidth sets the prompt width for the current screen.
func (m *Model) syncPromptWidth() {
    if m.screen == screenHome {
        m.prompt.SetWidth(m.homePromptWidth())
    } else {
        m.prompt.SetWidth(m.centerWidth())
    }
}

// goHome shows the home screen again with an empty chat.
func (m *Model) goHome() {
    m.interruptTurn()
    m.messages = nil
    m.scroll = 0
    m.started = time.Now().UTC()
    m.screen = screenHome
    m.prompt.Placeholder = homePlaceholder
    m.syncPromptWidth()
}

// --- logo -------------------------------------------------------------------

// Plain ASCII logo (figlet "Standard" font). To change it, paste any figlet
// output here and set logoSplit to the column where the white part starts.
var logoArt = []string{
    "  ____      _       _                       ",
    " / ___|__ _| | __ _| |__  _ __ __ _ ___ ___ ",
    "| |   / _` | |/ _` | '_ \\| '__/ _` / __/ __|",
    "| |__| (_| | | (_| | |_) | | | (_| \\__ \\__ \\",
    " \\____\\__,_|_|\\__,_|_.__/|_|  \\__,_|___/___/",
}

// logoSplit is the column where "Cala" (grey) ends and "brass" (white) starts.
const logoSplit = 20

// logoView draws the logo: first part dim grey, second part white.
func logoView() string {
    dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
    bright := lipgloss.NewStyle().Foreground(colorText).Bold(true)

    rows := make([]string, len(logoArt))
    for i, line := range logoArt {
        split := logoSplit
        if split > len(line) {
            split = len(line)
        }
        rows[i] = dim.Render(line[:split]) + bright.Render(line[split:])
    }
    return strings.Join(rows, "\n")
}

// --- home view --------------------------------------------------------------

// homeStatusBar: " 0 MCP /mcps ............ 0.1.0"
func (m Model) homeStatusBar() string {
    left := "  " +
        msgMuted.Render("● ") +
        lipgloss.NewStyle().Foreground(colorText).Bold(true).Render(fmt.Sprintf("%d MCP", m.enabledMCPs())) +
        msgMuted.Render(" /mcps")
    right := msgMuted.Render(appVersion) + "  "

    gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
    if gap < 1 {
        gap = 1
    }
    return left + strings.Repeat(" ", gap) + right
}

// homeView: logo, prompt-input and helper line in the middle of the screen,
// status bar on the last row. The "/" menu opens right above the prompt.
func (m Model) homeView() string {
    cardW := m.homePromptWidth()

    logo := logoView()
    if lipgloss.Width(logo) <= cardW {
        logo = lipgloss.PlaceHorizontal(cardW, lipgloss.Center, logo)
    }

    // The block: logo, 2 blank rows, prompt, helper line.
    block := strings.Split(logo, "\n")
    block = append(block, "", "")
    promptStart := len(block)
    block = append(block, strings.Split(m.prompt.View(), "\n")...)
    block = append(block, m.footerView(cardW, true))

    // Center the block by hand, so we know exactly where the prompt is.
    mainH := m.height - 1
    if mainH < 1 {
        mainH = 1
    }
    top := (mainH - len(block)) / 2
    if top < 0 {
        top = 0
    }
    x0 := (m.width - cardW) / 2
    if x0 < 0 {
        x0 = 0
    }
    pad := strings.Repeat(" ", x0)

    lines := make([]string, mainH)
    for i, l := range block {
        if row := top + i; row < mainH {
            lines[row] = pad + l
        }
    }

    // "/" menu: drawn over the rows above the prompt.
    if m.slashVisible() {
        promptTop := top + promptStart
        menu := m.slash.Lines(m.prompt.Value(), cardW, promptTop)
        for i, l := range menu {
            if row := promptTop - len(menu) + i; row >= 0 && row < mainH {
                lines[row] = pad + l
            }
        }
    }

    return lipgloss.JoinVertical(lipgloss.Left, strings.Join(lines, "\n"), m.homeStatusBar())
}