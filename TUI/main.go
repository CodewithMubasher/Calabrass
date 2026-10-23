package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	// App style with black background
	appStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("0")).
			Foreground(lipgloss.Color("7"))

	// Panel style: card style with grey color
	panelStyle = lipgloss.NewStyle().
			Width(40).
			Height(10).
			Background(lipgloss.Color("8")). // Dark grey/charcoal
			Foreground(lipgloss.Color("15")). // White/grey text
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("5")). // Blue accent for border
			Padding(1, 2)

	// Left panel style (same as panelStyle for now)
	leftPanelStyle = panelStyle

	// Right panel style (same as panelStyle for now)
	rightPanelStyle = panelStyle
)

// LeftPanel is a component that displays a left-side panel
type LeftPanel struct{}

// Init initializes the left panel
func (l LeftPanel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the left panel
func (l LeftPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return l, nil
}

// View renders the left panel
func (l LeftPanel) View() string {
	return fmt.Sprintf(
		"%s\n%s\n%s\n%s\n%s",
		"┌─────────────────────────────┐",
		"│         LEFT PANEL          │",
		"│    (Empty - Ready for use)  │",
		"└─────────────────────────────┘",
		"",
	)
}

// RightPanel is a component that displays a right-side panel
type RightPanel struct{}

// Init initializes the right panel
func (r RightPanel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the right panel
func (r RightPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return r, nil
}

// View renders the right panel
func (r RightPanel) View() string {
	return fmt.Sprintf(
		"%s\n%s\n%s\n%s\n%s",
		"┌─────────────────────────────┐",
		"│         RIGHT PANEL         │",
		"│    (Empty - Ready for use)  │",
		"└─────────────────────────────┘",
		"",
	)
}

// PanelHeader is a reusable header component
type PanelHeader struct {
	title string
}

// NewPanelHeader creates a new panel header
func NewPanelHeader(title string) PanelHeader {
	return PanelHeader{title: title}
}

// Init initializes the panel header
func (p PanelHeader) Init() tea.Cmd {
	return nil
}

// Update handles messages for the panel header
func (p PanelHeader) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return p, nil
}

// View renders the panel header
func (p PanelHeader) View() string {
	return fmt.Sprintf(
		"  %s  ",
		p.title,
	)
}

// AppModel is the main model that holds all panel components
type AppModel struct {
	LeftPanel LeftPanel
	RightPanel RightPanel
	Header    PanelHeader
	quit      bool
}

// Init initializes the application model
func (m AppModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the application
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quit = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// View renders the full application with panels layout
func (m AppModel) View() string {
	// Render left panel with its style
	leftContent := leftPanelStyle.Render(m.LeftPanel.View())

	// Render right panel with its style
	rightContent := rightPanelStyle.Render(m.RightPanel.View())

	// Combine: header at top, then left and right panels side by side
	// The space between them represents the empty center
	fullView := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftContent,
		rightContent,
	)

	return appStyle.Render(fullView)
}

func main() {
	p := tea.NewProgram(AppModel{
		Header: NewPanelHeader("Calabrass CLI"),
	})
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running TUI:", err)
	}
}