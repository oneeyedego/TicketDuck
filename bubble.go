package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/acarl005/stripansi"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// 1) We define three modes: selecting items, answering prompts or viewing the Glamour-rendered markdown.
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
	prompt    string
}

var formTypes = []formType{
	{
		name: "Incident Response",
		questions: []string{
			"What happened?",
			"What did you do?",
			"Why did you do it?",
			"Did it work? If not, what was the result?",
			"What did you learn?",
		},
		prompt: "Using the following text, craft an informative and detailed work note for an incident response. The output of your response should be a between 2 sentences and several paragraphs, depending on the amount of context offered. It does not need to restate the rubric questions.",
	},
	{
		name: "Pull Request/Commit Message",
		questions: []string{
			"What did you do?",
			"Why did you do it?",
			"What did you learn?",
		},
		prompt: "Using the following text, craft an informative and detailed title and description for a commit message or pull request. The output of your response should be a between 2 sentences and several paragraphs, depending on the amount of context offered. It does not need to restate the rubric questions.",
	},
	{
		name: "Service Request",
		questions: []string{
			"What do you want?",
			"Why do you want it?",
			"How do you want it?",
			"What will you do with it?",
		},
		prompt: "Using the following text, craft an informative and detailed message for a service request that is being made of a colleague. The output of your response should be a between 2 sentences and several paragraphs, depending on the amount of context offered. It does not need to restate the rubric questions.",
	},
	{
		name: "Development ticket",
		questions: []string{
			"Is this a feature, bug, or chore?",
			"What is the current behavior?",
			"How do you want to change, modify, or add behavior?",
			"Why do you want this change? What are the benefits?",
			"What are the acceptance criteria for this change?",
		},
		prompt: "Your task is to use the following text to create a detailed and informative ticket for a development task. The output of your response should be a between 2 sentences and several paragraphs, depending on the amount of context offered. It does not need to restate the rubric questions.",
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
	// Store the raw output from ChatGPT so we can re-render if needed.
	gptRawOutput string
	// Store the rendered markdown content so we can re-display or update if needed.
	content string

	gPressed bool // Used only to detect "gg" in display mode
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
	switch msg := msg.(type) {
	// Handle terminal resize events
	case tea.WindowSizeMsg:
		// Use the new dimensions provided by msg
		termWidth := msg.Width
		termHeight := msg.Height

		// Define margins or offsets as used previously
		marginWidth := 4  // e.g., borders, padding
		marginHeight := 8 // e.g., header/footer

		// Calculate new dimensions for the viewport
		width := termWidth - marginWidth
		height := termHeight - marginHeight
		if width < 40 {
			width = 40
		}
		if height < 10 {
			height = 10
		}

		// Update the viewport dimensions
		m.viewport.Width = width
		m.viewport.Height = height
		m.viewport.Style = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			PaddingLeft(2).
			PaddingRight(2)

		// If in display mode, re-render the markdown to adjust wrapping
		if m.currentMode == displayMode {
			// m.content is the raw markdown content that was last rendered.
			if err := renderMarkdownToViewport(m.content, &m.viewport); err != nil {
				log.Printf("Error re-rendering markdown on resize: %v\n", err)
			}
		}
		// Return without further commands, as resizing is now handled.
		return m, nil

	// Handle other message types based on current mode
	case tea.KeyMsg:
		switch m.currentMode {
		case selectionMode:
			return m.updateSelectionMode(msg)
		case questionMode:
			return m.updateQuestionMode(msg)
		case displayMode:
			return m.updateDisplayMode(msg)
		}
	}
	return m, nil
}

func (m model) updateSelectionMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {

		case "ctrl+c", "q":
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
				m = handleFormCompletion(m)
			}
		case tea.KeyCtrlS: // ← Skip question on Ctrl+S
			// Don’t store anything (or store empty string).
			m.answers[m.currentQuestion] = ""
			m.inputString = ""

			if m.currentQuestion < len(m.currentForm.questions)-1 {
				m.currentQuestion++
			} else {
				m = handleFormCompletion(m)
			}
		case tea.KeyBackspace, tea.KeyDelete:
			if len(m.inputString) > 0 {
				m.inputString = m.inputString[:len(m.inputString)-1] // Delete the last character
			}

		default:
			// Runes capture standard alphanumeric input, but not the space key.
			if msg.Type == tea.KeyRunes {
				m.inputString += msg.String()
			} else if msg.Type == tea.KeySpace {
				// Add explicit space handling
				m.inputString += " "
			}
		}
	}

	return m, nil
}

// countLines returns the number of lines in the given string.
func countLines(s string) int {
	return len(strings.Split(s, "\n"))
}

func (m model) updateDisplayMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		// Scroll up one line
		case "up", "k":
			if m.viewport.YOffset > 0 {
				m.viewport.YOffset--
			}
			return m, nil

		// Scroll down one line
		case "down", "j":
			// Calculate total number of lines from the viewport's current content.
			totalLines := countLines(m.content)
			maxYOffset := totalLines - m.viewport.Height
			if m.viewport.YOffset < maxYOffset {
				m.viewport.YOffset++
			}
			return m, nil

		// Page up: scroll up by the height of the viewport.
		case "pgup":
			m.viewport.YOffset -= m.viewport.Height
			if m.viewport.YOffset < 0 {
				m.viewport.YOffset = 0
			}
			return m, nil

		// Page down: scroll down by the height of the viewport.
		case "pgdown":
			totalLines := countLines(m.content)
			maxYOffset := totalLines - m.viewport.Height
			m.viewport.YOffset += m.viewport.Height
			if m.viewport.YOffset > maxYOffset {
				m.viewport.YOffset = maxYOffset
			}
			return m, nil

		// Jump to bottom
		case "G":
			totalLines := countLines(m.content)
			m.viewport.YOffset = totalLines - m.viewport.Height
			if m.viewport.YOffset < 0 {
				m.viewport.YOffset = 0
			}
			m.gPressed = false
			return m, nil

		// Jump to top (with "g" pressed twice)
		case "g":
			if m.gPressed {
				m.viewport.YOffset = 0
				m.gPressed = false
			} else {
				m.gPressed = true
			}
			return m, nil

		// Copy plain text to clipboard
		case "ctrl+y":
			plainText := stripansi.Strip(m.gptRawOutput)
			if err := clipboard.WriteAll(plainText); err != nil {
				log.Printf("Failed to copy to clipboard: %v\n", err)
			}
			return m, nil

		default:
			// For any other keys, ignore or implement additional behavior.
			return m, nil
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
	s += "Press Ctrl+s to skip this question.\n"
	s += "Press Esc or q to quit.\n"

	return s
}

// View rendering for Display Mode
func (m model) viewDisplayMode() string {
	s := titleStyle.Render("Generated Output") + "\n\n"
	s += m.viewport.View() + helpStyle.Render("\n  ↑/↓: Scroll • q: Quit • Ctrl+y to copy to clipboard\n")
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

	// Prepare a Glamour renderer using the dynamic width for proper word wrapping
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(len(md)),
	)

	if err != nil {
		return err
	}

	rendered, err := r.Render(md)
	if err != nil {
		return err
	}

	// Ensure the rendered content ends with a newline for proper display
	rendered = strings.TrimRight(rendered, "\n") + "\n"

	// Now set the content so that the viewport correctly computes the scrollable region
	vp.SetContent(rendered)
	return nil
}

// handleFormCompletion combines the other helper functions to pass the input on to ChatGPT.
func handleFormCompletion(m model) model {
	// Build the Markdown
	md := buildSelectedMarkdown(m)

	if err := renderMarkdownToViewport(md, &m.viewport); err != nil {
		log.Fatal(err)
	}
	m.content = md

	// Make the ChatGPT request
	ctx := context.TODO() // or a context with a timeout
	if err := makeChatGPTRequest(ctx, &m, md); err != nil {
		log.Printf("Error from ChatGPT: %v\n", err)
	}

	m.currentMode = displayMode
	return m
}

// ---[[ OpenAI API ]]------------------------------------------------------------

// makeChatGPTRequest encapsulates the ChatGPT call & viewport re-rendering.
func makeChatGPTRequest(ctx context.Context, m *model, md string) error {

	// Append the prompt to the generated response
	combinedPrompt := m.currentForm.prompt + "\n\n" + md

	// Step 1 - Call ChatGPT with the generated response Markdown
	resp, err := processFormWithChatGPT(ctx, combinedPrompt)
	if err != nil {
		return fmt.Errorf("OpenAI request error: %v", err)
	}

	m.gptRawOutput = resp // Store the raw output

	// Step 2 - Append ChatGPT’s response as an optional "analysis" or "summary"
	summary := "\n## Ticket Summary\n\n" + resp
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
