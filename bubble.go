package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// The model

type model struct {
	choices  []string         // items to choose from
	cursor   int              // the current selection our cursor is poitning at
	selected map[int]struct{} // set of selected items

}

func initialModel() model {
	return model{
		// The choices we want to display
		choices: []string{"Carrots", "Beets", "Asparagus", "Broccoli", "Cabbage", "Dill", "Potatoes"},

		// a map for storing which choices are selected. The keys refer to the indexes of the choices slice above.
		selected: make(map[int]struct{}),
	}
}

func (m model) Init() tea.Cmd {
	// No IO for the moment, so we return nil
	return nil
}

// Methods

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:

		switch msg.String() {

		// Quit the program when q or ctrl+c is pressed
		case "ctrl+c", "q":
			return m, tea.Quit

		// Move the cursor up when k or the up arrow is pressed
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		// Move the cursor down when j or the down arrow is pressed
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}

		// Toggle the selected state of the item when space or enter is pressed
		case "enter", " ":
			if _, ok := m.selected[m.cursor]; ok {
				delete(m.selected, m.cursor)
			} else {
				m.selected[m.cursor] = struct{}{}
			}
		}
	}

	// Return the updated model
	return m, nil
}

// The view

func (m model) View() string {
	s := "What should we buy at the grocery store?\n\n"

	// Iterate over the choices
	for i, choice := range m.choices {
		// If the cursor is pointing at this choice, we'll highlight it
		cursor := " " // no cursor
		if m.cursor == i {
			cursor = ">" // cursor
		}

		// If the choice is selected, we'll add an "x" to the beginning
		checked := " " // not selected
		if _, ok := m.selected[i]; ok {
			checked = "x" // selected
		}

		// Render the row
		s += fmt.Sprintf("%s [%s] %s\n", cursor, checked, choice)

	}

	s += "\n Press q to quit.\n"

	// Send the UI for rendering
	return s

}

// The main function

func main() {
	p := tea.NewProgram(initialModel())
	if err := p.Start(); err != nil {
		fmt.Printf("Error starting program: %v", err)
		os.Exit(1)
	}
}
