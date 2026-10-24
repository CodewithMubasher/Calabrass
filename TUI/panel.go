package main

import (
    "strings"
    "time"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

const (
    // Same margin (in columns) on the left, right, and between the cards.
    // Top/bottom margin is half a row, which looks the same (see card.go).
    margin       = 1
    marginLeft   = 3 // gap on the left side of the app (a bit bigger)
    panelPercent = 25 // the side panel is 25% of the terminal width

    // Key bindings. Change them here.
    keyToggleRight = "ctrl+r"
    keyCommandPalette = "ctrl+p"
)

// Model is the top-level Bubble Tea model.
type Model struct {
    width     int
    height    int
    prompt    PromptInput
    dialog    Dialog
    slash     SlashMenu // the "/" command menu above the prompt
    screen    Screen // home (start) or chat
    messages  []Message
    nextID    int
    scroll    int // chat lines scrolled up from the bottom
    hoverMsg  int // timeline row under the mouse: message index (-1 = none)
    hoverPart int //   and the index of its first part
    cwd       string    // "~/path:branch", shown in the footer and right panel
    started   time.Time // session start, shown in the right panel
    showRight bool
}

// NewModel creates a new Model instance.
func NewModel() Model {
    return Model{
        prompt:    newHomePrompt(),
        dialog:    NewDialog(),
        slash:     NewSlashMenu(),
        hoverMsg:  -1,
        hoverPart: -1,
        cwd:       workDirLabel(),
        started:   time.Now().UTC(),
        showRight: true,
    }
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
    return m.prompt.Init()
}

func (m Model) panelWidth() int {
    return m.width * panelPercent / 100
}

// centerWidth is whatever is left after the right panel (if open) and the
// margins. When the panel is closed, its width AND its gap go to the center.
func (m Model) centerWidth() int {
    w := m.width - marginLeft - margin // outer left + right margin
    if m.showRight {
        w -= m.panelWidth() + margin
    }
    if w < 1 {
        w = 1
    }
    return w
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    // While the command palette is open it gets every key, so typing goes
    // to its search box and Esc closes it instead of quitting the app.
    if m.dialog.IsOpen() {
        if key, ok := msg.(tea.KeyMsg); ok {
            switch key.String() {
            case "ctrl+c":
                return m, tea.Quit
            case keyCommandPalette:
                m.dialog.Close()
                return m, nil
            }
            var cmd tea.Cmd
            m.dialog, cmd = m.dialog.Update(msg)
            return m, cmd
        }
    }

    // "/" menu: while it is open it takes the navigation keys.
    if key, ok := msg.(tea.KeyMsg); ok && m.slashVisible() {
        value := m.prompt.Value()
        switch key.String() {
        case "esc":
            m.slash.Dismiss()
            return m, nil
        case "up":
            m.slash.Move(-1, value)
            return m, nil
        case "down":
            m.slash.Move(1, value)
            return m, nil
        case "tab":
            // Complete the command name and close the menu (space dismisses it).
            if c, ok := m.slash.Selected(value); ok {
                m.prompt.SetValue(c.Name + " ")
                m.slash.Sync(m.prompt.Value())
            }
            return m, nil
        case "enter":
            if c, ok := m.slash.Selected(value); ok {
                m.prompt.SetValue("")
                m.slash.Sync("")
                return m, func() tea.Msg { return SlashSelectMsg{Command: c} }
            }
        }
    }

    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "ctrl+c", "esc":
            return m, tea.Quit
        case keyCommandPalette:
            return m, m.dialog.Open()
        case "pgup":
            m.scrollBy(m.chatHeight() / 2)
            return m, nil
        case "pgdown":
            m.scrollBy(-m.chatHeight() / 2)
            return m, nil
        case keyToggleRight:
            m.showRight = !m.showRight
            m.syncPromptWidth()
            return m, nil
        }
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        m.syncPromptWidth()
        m.dialog.SetSize(msg.Width, msg.Height)
        return m, nil
    case tea.MouseMsg:
        if m.dialog.IsOpen() {
            return m, nil
        }
        switch {
        case msg.Button == tea.MouseButtonWheelUp:
            m.scrollBy(3)
        case msg.Button == tea.MouseButtonWheelDown:
            m.scrollBy(-3)
        case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
            m.handleClick(msg.X, msg.Y)
        case msg.Action == tea.MouseActionMotion:
            m.handleHover(msg.X, msg.Y)
        }
        return m, nil
    case fakeReplyMsg:
        cmd := m.finishReply(msg.ID)
        return m, cmd
    case streamTickMsg:
        cmd := m.streamStep(msg.ID)
        return m, cmd
    case SlashSelectMsg:
        switch msg.Command.Name {
        case "/clear":
            m.messages = nil
            m.scroll = 0
            m.started = time.Now().UTC()
        case "/new":
            m.goHome()
        case "/agents":
            m.prompt.CycleMode()
        case "/exit":
            return m, tea.Quit
        }
        // Hook: handle the other commands here.
        return m, nil
    case DialogSelectMsg:
        switch msg.Command.ID {
        case "new-session":
            m.goHome()
        case "clear-session":
            m.messages = nil
            m.scroll = 0
            m.started = time.Now().UTC()
        case "toggle-right":
            m.showRight = !m.showRight
            m.syncPromptWidth()
        }
        // Hook: handle the other command IDs here later.
        return m, nil
    case PromptSubmitMsg:
        // First message: leave the home screen and show the chat.
        if m.screen == screenHome {
            m.screen = screenChat
            m.prompt.Placeholder = ""
            m.syncPromptWidth()
        }
        cmd := m.send(msg.Value)
        return m, cmd
    }

    // Everything else (typing, cursor blink) goes to the prompt-input.
    var cmd tea.Cmd
    m.prompt, cmd = m.prompt.Update(msg)
    m.slash.Sync(m.prompt.Value()) // typing changes what the "/" menu shows

    // The palette's search box needs the cursor-blink messages too.
    if m.dialog.IsOpen() {
        var dcmd tea.Cmd
        m.dialog, dcmd = m.dialog.Update(msg)
        cmd = tea.Batch(cmd, dcmd)
    }
    return m, cmd
}

// slashVisible reports whether the "/" menu is showing.
func (m Model) slashVisible() bool {
    return !m.dialog.IsOpen() && m.slash.Visible(m.prompt.Value())
}

// View implements tea.Model.
func (m Model) View() string {
    if m.width == 0 || m.height == 0 {
        return "Initializing..."
    }

    var view string
    if m.screen == screenHome {
        view = m.homeView()
    } else {
        view = m.chatView()
    }

    // Command palette: centered modal over a dimmed app.
    if m.dialog.IsOpen() {
        return m.dialog.Overlay(view, m.width, m.height)
    }
    return view
}

// chatView is the chat screen: chat, prompt-input and the right panel.
func (m Model) chatView() string {
    panelW := m.panelWidth()
    centerW := m.centerWidth()

    // Each card gets 2 extra rows for its half-block caps.
    bodyH := m.height - 2
    if bodyH < 1 {
        bodyH = 1
    }

    spacer := appStyle.Width(margin).Height(m.height).Render("")
    leftSpacer := appStyle.Width(marginLeft).Height(m.height).Render("")

    // Center column: chat on top (newest message at the bottom), prompt-input
    // pinned to the bottom.
    chatH := m.chatHeight()
    lines, _ := m.chatLines()
    start, end := visibleRange(len(lines), chatH, m.scroll)
    chat := lipgloss.Place(
        centerW, chatH,
        lipgloss.Left, lipgloss.Top,
        strings.Join(lines[start:end], "\n"),
    )
    // "/" menu: drawn over the bottom rows of the chat, right above the prompt.
    if m.slashVisible() {
        chat = replaceBottomLines(chat, m.slash.Lines(m.prompt.Value(), centerW, chatH))
    }
    center := lipgloss.JoinVertical(lipgloss.Left, chat, m.prompt.View(), m.footerView(centerW, false))

    // Build the row: center, then the right panel if it is open.
    parts := []string{leftSpacer, center}
    if m.showRight {
        parts = append(parts, spacer, renderCard(panelStyle, panelW, bodyH, m.sidebarView(panelW-4, bodyH-2)))
    }
    parts = append(parts, spacer)

    return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}
