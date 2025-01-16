package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// 1) We define two modes: selecting items or viewing the Glamour-rendered markdown.
type mode int

const (
	selectionMode mode = iota
	questionMode
	displayMode
)

// ---[ Lip Gloss Styles ]-----------------------------------------------------

type formType struct {
	name      string
	questions []string
}

var formTypes = []formType{
	{
		name: "Incident Report",
		questions: []string{
			"What happened?",
			"What did you do?",
			"Why did you do it?",
			"Did it work? If not, what was the result?",
			"What did you learn?",
		},
	},
	{
		name: "Pull Request/Commit Message",
		questions: []string{
			"What did you do?",
			"Why did you do it?",
			"How did you do it?",
			"What did you learn?",
		},
	},
	{
		name: "Service Request",
		questions: []string{
			"What do you want?",
			"Why do you want it?",
			"How do you want it?",
			"What will you do with it?",
		},
	},
}

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
	formTypes     []formType
	cursor        int
	selectedIndex int // The index of the selected item, where -1 means no item is selected

	// For rubric mode:
	currentForm     formType
	answers         []string
	currentQuestion int
	inputString     string

	// For display mode:
	viewport viewport.Model
	// We store the rendered markdown content so we can re-display or update if needed.
	content string
}

// initialModel sets up the choicebox, selection data, and an uninitialized viewport.
func initialModel() model {
	return model{
		currentMode:   selectionMode,
		formTypes:     formTypes,
		selectedIndex: -1,
		answers:       []string{},
		viewport:      viewport.Model{}, // We'll configure this later
	}
}

// ---[ [Bubbletea interface] ]-------------------------------------------------

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.currentMode {
	case selectionMode:
		return m.updateSelectionMode(msg)
	case questionMode:
		return m.updateQuestionMode(msg)
	case displayMode:
		return m.updateDisplayMode(msg)
	default:
		return m, nil
	}
}

func (m model) updateSelectionMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {

		case "ctrl+c", "q":
			// If we're in display mode, pressing q just quits.
			// If we're in selection mode and we haven't pressed "enter" yet,
			// let's also quit. You could handle these differently if you wish.
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.formTypes)-1 {
				m.cursor++
			}
		case " ", "enter":
			if m.currentMode == selectionMode {
				// Toggle selection: since it's single-selection,
				// selecting a new item deselects the previous one.
				if m.selectedIndex == m.cursor {
					// Deselect if already selected
					m.selectedIndex = -1
				} else {
					m.selectedIndex = m.cursor
					m.currentForm = m.formTypes[m.selectedIndex]
					m.currentMode = questionMode
					m.answers = make([]string, len(m.currentForm.questions))
					m.currentQuestion = 0
				}
			}
		}
	}

	return m, nil
}

func (m model) updateQuestionMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc, tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			// Save the current input as an answer
			m.answers[m.currentQuestion] = strings.TrimSpace(m.inputString)
			m.inputString = ""

			// Move on to the next question or finish
			if m.currentQuestion < len(m.currentForm.questions)-1 {
				m.currentQuestion++
			} else {
				// We're done with the form, so it's time to generate the Markdown and switch modes
				md := buildSelectedMarkdown(m)
				if err := renderMarkdownToViewport(md, &m.viewport); err != nil {
					// If there's an error rendering the Markdown, just print it out
					log.Fatal(err)
				}
				m.content = md

				ctx := context.TODO() // or a context with timeout
				if err := makeChatGPTRequest(ctx, &m, md); err != nil {
					log.Printf("Error from ChatGPT: %v\n", err)
				}

				m.currentMode = displayMode
			}
		case tea.KeyBackspace, tea.KeyDelete:
			if len(m.inputString) > 0 {
				m.inputString = m.inputString[:len(m.inputString)-1] // Delete the last character
			}

		default:
			if msg.Type == tea.KeyRunes {
				m.inputString += msg.String()
			}
		}
	}

	return m, nil
}

func (m model) updateDisplayMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			// Quit
			return m, tea.Quit

		default:
			// Pass all other keys to the viewport for scrolling
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// --- [View] ----------------------------------------------------------------

func (m model) View() string {
	switch m.currentMode {

	case selectionMode:
		return m.viewSelectionMode()

	case questionMode:
		return m.viewQuestionMode()

	case displayMode:
		return m.viewDisplayMode()

	default:
		return "Unknown mode."

	}
}

// View rendering for Selection Mode
func (m model) viewSelectionMode() string {
	s := titleStyle.Render("Select Report Type") + "\n\n"

	for i, rt := range m.formTypes {
		cursor := "  "
		if m.cursor == i {
			cursor = cursorStyle.Render(">")
		}

		line := fmt.Sprintf("%s %s", cursor, rt.name)

		if m.cursor == i {
			line = selectedStyle.Render(line)
		} else {
			line = dimStyle.Render(line)
		}

		s += line + "\n"
	}

	s += "\nUse ↑/↓ or k/j to navigate. Press Enter or Space to select.\n"
	s += "Press q to quit.\n"

	return s
}

// View rendering for Question Mode
func (m model) viewQuestionMode() string {
	currentQ := m.currentForm.questions[m.currentQuestion]
	inputLine := "> " + m.inputString

	s := titleStyle.Render(fmt.Sprintf("%s - Question %d/%d", m.currentForm.name, m.currentQuestion+1, len(m.currentForm.questions))) + "\n\n"
	s += fmt.Sprintf("**%s**\n\n", currentQ)
	s += inputLine

	s += "\n\nPress Enter to submit your answer.\n"
	s += "Press Esc or q to quit.\n"

	return s
}

// View rendering for Display Mode
func (m model) viewDisplayMode() string {
	s := titleStyle.Render("Generated Report") + "\n\n"
	s += m.viewport.View() + helpStyle.Render("\n  ↑/↓: Scroll • q: Quit\n")
	return s
}

// --- [ Helper functions ] ------------------------------------

// buildSelectedMarkdown returns a string of Markdown reflecting the selected items.
func buildSelectedMarkdown(m model) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s\n\n", m.currentForm.name))
	for i, question := range m.currentForm.questions {
		sb.WriteString(fmt.Sprintf("## %d. %s\n\n", i+1, question))
		if i < len(m.answers) {
			sb.WriteString(fmt.Sprintf("%s\n\n", m.answers[i]))
		}
	}

	return sb.String()
}

// renderMarkdownToViewport uses Glamour to transform the raw markdown into styled text.
func renderMarkdownToViewport(md string, vp *viewport.Model) error {
	width := 90 // Arbitrary width; adjust as needed
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

// ---[[ OpenAI API ]]------------------------------------------------------------

// makeChatGPTRequest encapsulates the ChatGPT call & viewport re-rendering.
func makeChatGPTRequest(ctx context.Context, m *model, md string) error {
	// Step 1 - Call ChatGPT with the generated response Markdown
	resp, err := processFormWithChatGPT(ctx, md)
	if err != nil {
		return fmt.Errorf("OpenAI request error: %v", err)
	}

	// Step 2 - Append ChatGPT’s response as an optional "analysis" or "summary"
	summary := "\n## ChatGPT Analysis\n\n" + resp
	appendedContent := md + summary

	// Step 3 - Re-render the viewport with the appended content
	if err := renderMarkdownToViewport(appendedContent, &m.viewport); err != nil {
		return fmt.Errorf("render markdown error: %v", err)
	}
	m.content = appendedContent
	return nil
}

func processFormWithChatGPT(ctx context.Context, content string) (string, error) {
	// Initialize the OpenAI client.
	// If the key is already in your environment, you can omit `option.WithAPIKey`.
	client := openai.NewClient(
		option.WithAPIKey(os.Getenv("OPENAI_API_KEY")), // TODO: Replace with app secret for portability
	)

	chatCompletion, err := client.Chat.Completions.New(
		ctx, // Context here allows us to characterize the request, and give it conditions
		openai.ChatCompletionNewParams{ // This is the message we are sending to ChatGPT
			Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
				openai.UserMessage(content),
			}),
			Model: openai.F(openai.ChatModelGPT3_5Turbo),
		},
	)
	if err != nil {
		return "", err
	}
	return chatCompletion.Choices[0].Message.Content, nil
}

// ---[ Main ]------------------------------------------------------------
func main() {
	p := tea.NewProgram(initialModel())
	if err := p.Start(); err != nil {
		fmt.Printf("Error starting program: %v\n", err)
		os.Exit(1)
	}
}
