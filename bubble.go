package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	lipgloss "github.com/charmbracelet/lipgloss"
)

// ---[ Lip Gloss Styles ]-----------------------------------------------------

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5F87")).
			Bold(true)

	checkedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A550DF"))

	dimStyle = lipgloss.NewStyle().
			Faint(true)
)

// ---[ Model ]----------------------------------------------------------------

type model struct {
	choices  []string         // items to choose from
	cursor   int              // the current selection our cursor is pointing at
	selected map[int]struct{} // set of selected items
}

func initialModel() model {
	return model{
		choices: []string{
			"Carrots",
			"Beets",
			"Asparagus",
			"Broccoli",
			"Cabbage",
			"Dill",
			"Potatoes",
		},
		selected: make(map[int]struct{}),
	}
}

// ---[ Bubble Tea interface ]-------------------------------------------------

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "j", "down":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case " ", "enter":
			if _, ok := m.selected[m.cursor]; ok {
				delete(m.selected, m.cursor)
			} else {
				m.selected[m.cursor] = struct{}{}
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	// Title
	s := titleStyle.Render("What should we buy at the grocery store?") + "\n\n"

	// List
	for i, choice := range m.choices {
		// Determine cursor
		cursor := " "
		if m.cursor == i {
			cursor = cursorStyle.Render(">")
		}
		// Determine selected state
		checked := " "
		_, isSelected := m.selected[i]
		if isSelected {
			checked = checkedStyle.Render("x")
		}

		// Style the choice differently if it's selected
		line := fmt.Sprintf("%s [%s] %s", cursor, checked, choice)
		if isSelected {
			line = selectedStyle.Render(line)
		}

		// Apply a faint/dim style if the item is neither selected nor under cursor
		if m.cursor != i && !isSelected {
			line = dimStyle.Render(line)
		}
		s += line + "\n"
	}

	s += "\nPress q to quit.\n"
	return s
}

func main() {
	if err := tea.NewProgram(initialModel()).Start(); err != nil {
		fmt.Printf("Error starting program: %v\n", err)
		os.Exit(1)
	}
}
