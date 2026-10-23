package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbletea"
)

func main() {
	p := tea.NewProgram(initialModel(), bubbletea.WithOutput(os.Stdout))
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running TUI:", err)
		os.Exit(1)
	}
}

type model struct {
	quit bool
}

func initialModel() model {
	return model{}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m model) View() string {
	return `
Calabrass CLI/TUI Application

Press Ctrl+Q or Q to quit
`
}