package main

import (
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/paginator"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"calabrass-tui/internal/api"
)

const (
	// Same margin (in columns) on the left, right, and between the cards.
	// Top/bottom margin is half a row, which looks the same (see card.go).
	margin       = 1
	marginLeft   = 3  // gap on the left side of the app (a bit bigger)
	panelPercent = 25 // the side panel is 25% of the terminal width

	// Key bindings. Change them here.
	keyToggleRight    = "ctrl+r"
	keyToggleRows     = "ctrl+o"
	keyCommandPalette = "ctrl+p"
)

// bootState is how far the connection to the backend has got.
type bootState int

const (
	bootConnecting bootState = iota
	bootFailed
	bootReady
)

// modelEntry is one selectable model of a usable provider.
type modelEntry struct {
	ProviderID   string
	ProviderName string
	ID           string
	Label        string
}

// Model is the top-level Bubble Tea model.
type Model struct {
	width      int
	height     int
	prompt     PromptInput
	dialog     Dialog // command palette
	list       ListDialog
	provForm   ProviderForm
	qform      QuestionForm
	quick      QuickMenu // hold ↓ for two seconds
	slash      SlashMenu // the "/" command menu above the prompt
	screen     Screen    // home (start) or chat
	messages   []Message
	nextID     int
	scroll     int       // chat lines scrolled up from the bottom
	hoverMsg   int       // timeline row under the mouse: message index (-1 = none)
	hoverPart  int       //   and the index of its first part
	cwdPath    string    // the folder the agent works in (absolute)
	cwd        string    // "~/path:branch", shown in the footer and right panel
	started    time.Time // session start
	showRight  bool
	spin       paginator.Model // animated dots in the footer
	sp         spinner.Model   // bubbles Spinner for thinking / edit rows
	spBusy     bool            // the spinner tick loop is running
	spinActive bool            // a spinTick loop is already running
	holdSeq    int             // identifies the current hold-↓ timer
	holdCount  int             // arrow-down presses seen while holding
	holdLast   time.Time       // when the last arrow-down press arrived
	holdSeen   bool            // a hold is in progress or just opened the menu
	toastText  string          // one-line notice in the chat's top-right corner
	toastErr   bool            // draw the toast in the error color
	toastSeq   int             // identifies the current toast timer

	// --- mouse selection + ctrl+c ---
	sel         selection // the highlighted screen range
	selPress    selPoint  // where the left button went down
	selPressing bool      // the button is down, so a drag may start
	selDragging bool      // the drag has moved far enough to be a selection
	quitArmed   bool      // one ctrl+c was pressed; the next one quits
	quitAt      time.Time // when the first ctrl+c arrived

	// --- backend state ---
	app          *appCtx
	boot         bootState
	bootErr      string
	chat         *api.Chat // nil until the first message is sent (or a session is resumed)
	models       []modelEntry
	selProvider  string
	selModel     string
	providerRows []api.ProviderRow
	connectors   []api.Connector
	allChats     []api.Chat
	sessionsAll  bool   // sessions list shows every folder
	confirmDel   string // session id waiting for a second ctrl+d
	turn         *turnState
	usageIn      int // tokens used by turns of this session
	usageOut     int
	banner       string // one-line hint on the home screen
	schedules    []api.ScheduledTask
}

// NewModel creates a new Model instance.
func NewModel(app *appCtx) Model {
	cwd, _ := os.Getwd()
	return Model{
		prompt:    newHomePrompt(),
		dialog:    NewDialog(),
		list:      NewListDialog(),
		provForm:  NewProviderForm(),
		qform:     NewQuestionForm(),
		quick:     NewQuickMenu(),
		slash:     NewSlashMenu(),
		hoverMsg:  -1,
		hoverPart: -1,
		cwdPath:   cwd,
		cwd:       workDirLabel(cwd),
		started:   time.Now().UTC(),
		showRight: true,
		spin:      newSpinner(),
		sp:        newRowSpinner(),
		app:       app,
		boot:      bootConnecting,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.prompt.Init(), bootCmd(m.app))
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

// rightPanelContent is the content of the right card.
func (m Model) rightPanelContent(panelW, bodyH int) string {
	return m.sidebarView(panelW-4, bodyH-2)
}

// modalOpen reports whether a dialog has the keyboard.
func (m Model) modalOpen() bool {
	return m.dialog.IsOpen() || m.list.IsOpen() || m.provForm.IsOpen() ||
		m.qform.IsOpen() || m.quick.IsOpen()
}

// slashVisible reports whether the "/" menu is showing.
func (m Model) slashVisible() bool {
	return !m.modalOpen() && m.slash.Visible(m.prompt.Value())
}

// View implements tea.Model.
func (m Model) View() tea.View {
	v := tea.NewView(m.viewString())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	v.WindowTitle = appName
	return v
}

// viewString draws the whole screen, with the mouse selection painted on top.
func (m Model) viewString() string {
	return m.withSelection(m.renderScreen())
}

// renderScreen draws the whole screen as one string.
func (m Model) renderScreen() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	if m.boot != bootReady {
		return m.bootView()
	}

	var view string
	if m.screen == screenHome {
		view = m.homeView()
	} else {
		view = m.chatView()
	}

	switch {
	case m.dialog.IsOpen():
		view = m.dialog.Overlay(view, m.width, m.height)
	case m.list.IsOpen():
		view = m.list.Overlay(view, m.width, m.height)
	case m.provForm.IsOpen():
		view = m.provForm.Overlay(view, m.width, m.height)
	case m.qform.IsOpen():
		view = m.qform.Overlay(view, m.width, m.height)
	case m.quick.IsOpen():
		view = m.quick.Overlay(view, m.width, m.height)
	}
	return m.withToast(view)
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
		parts = append(parts, spacer,
			renderCard(panelStyle, panelW, bodyH, m.rightPanelContent(panelW, bodyH)))
	}
	parts = append(parts, spacer)

	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}
