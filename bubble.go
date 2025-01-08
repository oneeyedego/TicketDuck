package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// 1) We define two modes: selecting items or viewing the Glamour-rendered markdown.
type mode int

const (
	selectionMode mode = iota
	displayMode
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

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

// ---[ Model ]----------------------------------------------------------------

type model struct {
	currentMode mode

	// For selection mode:
	choices  []string
	cursor   int
	selected map[int]struct{}

	// For display mode:
	viewport viewport.Model
	// We store the rendered markdown content so we can re-display or update if needed.
	content string
}

// initialModel sets up the grocery items, selection data, and an uninitialized viewport.
func initialModel() model {
	return model{
		currentMode: selectionMode,
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
		viewport: viewport.Model{}, // We'll configure it later, in code
	}
}

// ---[ Helper: Build and Render Markdown ]------------------------------------

// buildSelectedMarkdown returns a string of Markdown reflecting the selected items.
func buildSelectedMarkdown(m model) string {
	var sb strings.Builder

	sb.WriteString("# Grocery List\n\n")
	sb.WriteString("These items have been selected:\n\n")

	for i := range m.selected {
		sb.WriteString(fmt.Sprintf("- %s\n", m.choices[i]))
	}

	// If no items selected, mention that:
	if len(m.selected) == 0 {
		sb.WriteString("\n*No items selected.*\n")
	}

	// Additional flourish or instructions:
	sb.WriteString("\n\nBon appétit!\n")

	return sb.String()
}

// renderMarkdownToViewport uses Glamour to transform the raw markdown into styled text.
func renderMarkdownToViewport(md string, vp *viewport.Model) error {
	width := 78 // Arbitrary width; adjust as needed
	// Prepare a Glamour renderer
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return err
	}

	rendered, err := r.Render(md)
	if err != nil {
		return err
	}

	// Setup the viewport with the rendered content
	vp.SetContent(rendered)
	vp.Width = width
	vp.Height = 20
	vp.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		PaddingRight(2)

	return nil
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
			// If we're in display mode, pressing q just quits.
			// If we're in selection mode and we haven't pressed "enter" yet,
			// let's also quit. You could handle these differently if you wish.
			return m, tea.Quit

		// Only relevant in selection mode
		case "up", "k":
			if m.currentMode == selectionMode && m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.currentMode == selectionMode && m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case " ", "enter":
			if m.currentMode == selectionMode {
				// Toggle the selected state of the item
				if _, ok := m.selected[m.cursor]; ok {
					delete(m.selected, m.cursor)
				} else {
					m.selected[m.cursor] = struct{}{}
				}
			}
		case "r":
			// For demonstration, let's say "r" means "Render now!"
			// We switch to display mode.
			if m.currentMode == selectionMode {
				// 1) Build the Markdown from current selection
				md := buildSelectedMarkdown(m)
				m.content = md

				// 2) Render with Glamour into the viewport
				if err := renderMarkdownToViewport(md, &m.viewport); err != nil {
					fmt.Println("Could not render markdown:", err)
					return m, tea.Quit
				}

				// 3) Switch mode
				m.currentMode = displayMode
			}
		}

		// If we’re in display mode, pass the update to the viewport for scroll handling:
		if m.currentMode == displayMode {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

	}

	return m, nil
}

func (m model) View() string {
	switch m.currentMode {

	case selectionMode:
		// Render the selection list
		s := titleStyle.Render("What should we buy at the grocery store?") + "\n\n"

		for i, choice := range m.choices {
			cursor := " "
			if m.cursor == i {
				cursor = cursorStyle.Render(">")
			}

			checked := " "
			if _, ok := m.selected[i]; ok {
				checked = checkedStyle.Render("x")
			}

			line := fmt.Sprintf("%s [%s] %s", cursor, checked, choice)

			if _, ok := m.selected[i]; ok {
				line = selectedStyle.Render(line)
			}

			if _, ok := m.selected[i]; m.cursor != i && !ok {
				line = dimStyle.Render(line)
			}

			s += line + "\n"
		}
		s += "\nPress [↑/↓ or k/j] to move; [space or enter] to toggle selections.\n"
		s += "Press [r] to render your selection in Glamour or [q] to quit.\n"
		return s

	case displayMode:
		// Render the viewport with the Glamour-styled Markdown
		return m.viewport.View() + helpStyle.Render("\n  ↑/↓: Scroll • q: Quit\n")

	default:
		return "Unknown mode."
	}
}

func main() {
	p := tea.NewProgram(initialModel())
	if err := p.Start(); err != nil {
		fmt.Printf("Error starting program: %v\n", err)
		os.Exit(1)
	}
}
