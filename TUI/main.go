package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// App-wide styles
var (
	colorPanel = lipgloss.Color("#1e1e1e") // darker grey panel (matches prompt input)
	colorText  = lipgloss.Color("#ffffff") // white text

	// The main app style: NO background color, so it inherits the terminal's
	// own background and the app area never looks different from the window.
	appStyle = lipgloss.NewStyle().
			Foreground(colorText)

	// Panel style (grey, no borders, with padding)
	panelStyle = lipgloss.NewStyle().
			Background(colorPanel).
			Foreground(colorText).
			Padding(1, 2) // 1 row top/bottom, 2 cols left/right
)

func main() {
	p := tea.NewProgram(NewModel())
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running TUI:", err)
		os.Exit(1)
	}
}
