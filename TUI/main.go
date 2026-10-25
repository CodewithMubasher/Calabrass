package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"calabrass-tui/internal/api"
	"calabrass-tui/internal/settings"
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
	addr := flag.String("addr", "", "backend address (default: $CALABRASS_ADDR, the saved one, or "+api.DefaultAddr+")")
	cont := flag.Bool("c", false, "continue the latest session of this folder")
	flag.BoolVar(cont, "continue", false, "continue the latest session of this folder")
	flag.Parse()

	st, err := settings.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "calabrass: could not read settings:", err)
		st = settings.Settings{}
	}
	switch {
	case *addr != "":
		st.Addr = *addr
	case os.Getenv("CALABRASS_ADDR") != "":
		st.Addr = os.Getenv("CALABRASS_ADDR")
	}

	app := &appCtx{client: api.New(st.Addr), st: &st, resume: *cont}
	p := tea.NewProgram(NewModel(app))
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running TUI:", err)
		os.Exit(1)
	}
}
