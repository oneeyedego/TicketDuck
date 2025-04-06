package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/acarl005/stripansi"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	anthropic "github.com/liushuangls/go-anthropic"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// ---[ DEBUG: Logging ]-------------------------------------------------------
//
// This section defines the logging functionality for the application.
//
// The logging is used to record the state and behavior of the application.
// It is stored in a file in the user's home directory.
//
// I'm using this to debug the application, but I might delete it before finalizing the project.

// Initialize the logger
var (
	// Placeholder for our file logger
	logger *log.Logger
	// Log file handle
	logFile *os.File
)

// setupLogging initializes file-based logging
func setupLogging() error {
	// Create logs directory if it doesn't exist
	logsDir := filepath.Join(getConfigDir(), "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %v", err)
	}

	// Create log file with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	logFilePath := filepath.Join(logsDir, fmt.Sprintf("ticketsummarytool_%s.log", timestamp))

	var err error
	logFile, err = os.Create(logFilePath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %v", err)
	}

	// Configure the logger
	logger = log.New(logFile, "", log.LstdFlags)
	logger.Printf("Logging initialized at %s", timestamp)

	return nil
}

// closeLogging properly closes the log file
func closeLogging() {
	if logFile != nil {
		logger.Println("Logging terminated")
		logFile.Close()
	}
}

// logf is a helper function for logging formatted messages
func logf(format string, v ...interface{}) {
	if logger != nil {
		logger.Printf(format, v...)
	}
}

// ---[ Configuration ]-------------------------------------------------------
//
// This section defines the configuration for the application.
//
// The configuration is used to manage the state and behavior of the application.
// It defines the modes and providers for the application, as well as the API keys and base URLs for the LLM providers, if applicable.
// Its state is stored in raw JSON in a config file in the user's home directory.
//

type mode int

const (
	selectionMode mode = iota
	questionMode
	displayMode
	apiKeyInputMode
	modelSelectMode
	styleSelectMode
)

// ModelProvider represents the different AI providers supported by the application
type ModelProvider string

const (
	ProviderOpenAI    ModelProvider = "openai"
	ProviderAnthropic ModelProvider = "claude"
	ProviderLocal     ModelProvider = "local"
)

// ModelConfig holds configuration for a specific AI model
type ModelConfig struct {
	Provider   ModelProvider `json:"provider"`
	ModelName  string        `json:"model_name"`
	APIKey     string        `json:"api_key,omitempty"`
	APIBaseURL string        `json:"api_base_url,omitempty"` // For local models or custom endpoints
}

// Config holds all application configuration
type Config struct {
	ActiveModel string                 `json:"active_model"`
	Models      map[string]ModelConfig `json:"models"`
}

// This provides presets for common providers of pre-trained models, but you could certainly add more
// The local models (e.g., Mistral, Llama) should probably be modified to suit your hosting situation,
// which you'll be able to configure at runtime.

var DefaultModelConfigs = map[string]ModelConfig{
	"openai": {
		Provider:  ProviderOpenAI,
		ModelName: "gpt-3.5-turbo", // Default model, can be changed
	},
	"anthropic": {
		Provider:  ProviderAnthropic,
		ModelName: "claude-3-sonnet-20240229", // Default model, can be changed
	},
	"ollama": {
		Provider:   ProviderLocal,
		ModelName:  "llama3", // Default model, can be changed
		APIBaseURL: "http://localhost:11434",
	},
}

// getConfigDir returns the directory for storing configuration
func getConfigDir() string {
	// First try to use the XDG_CONFIG_HOME environment variable
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir != "" {
		return filepath.Join(configDir, "ticketsummarytool")
	}

	// Fall back to the user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Warning: Could not get user home directory: %v\n", err)
		return ".ticketsummarytool" // Use current directory as fallback
	}

	return filepath.Join(homeDir, ".ticketsummarytool")
}

// saveConfig saves the configuration to the config file
func saveConfig(config Config) error {
	configDir := getConfigDir()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	configFile := filepath.Join(configDir, "config.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	if err := ioutil.WriteFile(configFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	return nil
}

// loadConfig loads the configuration from the config file
func loadConfig() (Config, error) {
	config := Config{
		ActiveModel: "", // No default model selected
		Models:      make(map[string]ModelConfig),
	}

	// Copy default model configs to the config
	for k, v := range DefaultModelConfigs {
		config.Models[k] = v
	}

	configDir := getConfigDir()
	configFile := filepath.Join(configDir, "config.json")

	// Check if config file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return config, nil // Return default config if file doesn't exist
	}

	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return config, fmt.Errorf("failed to read config file: %v", err)
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("failed to parse config file: %v", err)
	}

	// Ensure all default models exist in the config
	for k, v := range DefaultModelConfigs {
		if _, exists := config.Models[k]; !exists {
			config.Models[k] = v
		}
	}

	return config, nil
}

// ---[ Lip Gloss Styles ]-----------------------------------------------------

// StyleTheme represents a predefined style theme
type StyleTheme struct {
	Name  string
	Base  lipgloss.AdaptiveColor
	Accent lipgloss.AdaptiveColor
	Error  lipgloss.AdaptiveColor
	Success lipgloss.AdaptiveColor
}

// Available style themes
var styleThemes = []StyleTheme{
	{
		Name: "Default",
		Base: lipgloss.AdaptiveColor{Light: "#04B575", Dark: "#02BF87"},
		Accent: lipgloss.AdaptiveColor{Light: "#7D56F4", Dark: "#7D56F4"},
		Error: lipgloss.AdaptiveColor{Light: "#FF5F87", Dark: "#FF5F87"},
		Success: lipgloss.AdaptiveColor{Light: "#02BA84", Dark: "#02BF87"},
	},
	{
		Name: "Ocean",
		Base: lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7571F9"},
		Accent: lipgloss.AdaptiveColor{Light: "#00B4D8", Dark: "#00B4D8"},
		Error: lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF6B6B"},
		Success: lipgloss.AdaptiveColor{Light: "#4ECDC4", Dark: "#4ECDC4"},
	},
	{
		Name: "Sunset",
		Base: lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF6B6B"},
		Accent: lipgloss.AdaptiveColor{Light: "#FFD166", Dark: "#FFD166"},
		Error: lipgloss.AdaptiveColor{Light: "#EF476F", Dark: "#EF476F"},
		Success: lipgloss.AdaptiveColor{Light: "#06D6A0", Dark: "#06D6A0"},
	},
}

// Styles defines the styling for the application
type Styles struct {
	Base,
	HeaderText,
	Status,
	StatusHeader,
	Highlight,
	ErrorHeaderText,
	Help,
	// Status bar styles
	StatusBar,
	StatusText,
	StatusNugget,
	StatusMode lipgloss.Style
}

// NewStyles creates a new Styles instance with the given theme
func NewStyles(lg *lipgloss.Renderer, theme StyleTheme) *Styles {
	s := Styles{}
	s.Base = lg.NewStyle().
		Padding(1, 4, 0, 1)
	s.HeaderText = lg.NewStyle().
		Foreground(theme.Base).
		Bold(true).
		Padding(0, 1, 0, 2)
	s.Status = lg.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Base).
		PaddingLeft(1).
		MarginTop(1)
	s.StatusHeader = lg.NewStyle().
		Foreground(theme.Base).
		Bold(true)
	s.Highlight = lg.NewStyle().
		Foreground(theme.Base)
	s.ErrorHeaderText = lg.NewStyle().
		Foreground(theme.Error).
		Bold(true).
		Padding(0, 1, 0, 2)
	s.Help = lg.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "241", Dark: "241"})
	
	// Initialize status bar styles
	s.StatusBar = lg.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#343433", Dark: "#C1C6B2"}).
		Background(lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#353533"})
	
	s.StatusText = lg.NewStyle().
		Inherit(s.StatusBar)
	
	s.StatusNugget = lg.NewStyle().
		Foreground(lipgloss.Color("#FFFDF5")).
		Padding(0, 1)
	
	s.StatusMode = lg.NewStyle().
		Inherit(s.StatusBar).
		Foreground(lipgloss.Color("#FFFDF5")).
		Background(theme.Base).
		Padding(0, 1).
		MarginRight(1)
	
	return &s
}

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
		prompt: "Using the following text, craft an informative and detailed work note for an incident response. The output of your response should be a between 2 sentences and several paragraphs, depending on the amount of context offered. It does not need to restate the rubric questions. Ensure clarity and conciseness, without referring explicitly to 'the incident response'",
	},
	{
		name: "Pull Request/Commit Message",
		questions: []string{
			"What did you do?",
			"Why did you do it?",
			"What did you learn?",
		},
		prompt: "Using the following text, craft an informative and detailed title and description for a commit message or pull request. The output of your response should be a between 2 sentences and several paragraphs, depending on the amount of context offered. It does not need to restate the rubric questions. Ensure clarity and conciseness, without referring explicitly to 'the pull request' or 'the commit message'",
	},
	{
		name: "Service Request",
		questions: []string{
			"What do you want?",
			"Why do you want it?",
			"How do you want it?",
			"What will you do with it?",
		},
		prompt: "Using the following text, craft an informative and detailed message for a service request that is being made of a colleague. The output of your response should be a between 2 sentences and several paragraphs, depending on the amount of context offered. It does not need to restate the rubric questions. Ensure clarity and conciseness, without referring explicitly to 'the service request'",
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
		prompt: "Your task is to use the following text to create a detailed and informative ticket for a development task. The output of your response should be a between 2 sentences and several paragraphs, depending on the amount of context offered. It does not need to restate the rubric questions. Ensure clarity and conciseness, without referring explicitly to 'the ticket' or 'the development task'",
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
//
// This section defines the Model interface (Model as in Model-View-Controller/MVC, not Model as in machine learning model)
// and its implementation for the bubbletea framework.
//
// The Model interface is used to manage the state and behavior of the application.
// It defines the Update method, which is called when a message is received from the terminal.
//

type model struct {
	currentMode mode
	styles      *Styles

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
	// Store the raw output from the LLM so we can re-render if needed.
	gptRawOutput string
	// Store the rendered markdown content so we can re-display or update if needed.
	content string

	gPressed bool // Used only to detect "gg" in display mode

	// For API key input mode:
	apiKeyInput    textinput.Model
	apiBaseInput   textinput.Model
	modelNameInput textinput.Model
	focusedInput   int // 0 for API key, 1 for base URL, 2 for model name, 3 for save checkbox
	saveConfig     bool

	// For model selection:
	config        Config
	modelCursor   int
	modelKeys     []string // Keys from the Models map for easier navigation
	selectedModel string   // Currently selected model key

	width int // Added for appBoundaryView

	// For style selection:
	styleThemeIndex int
	styleThemes     []StyleTheme
}

// initialModel sets up the choicebox, selection data, and an uninitialized viewport.
func initialModel() model {
	// Load config with model information
	config, err := loadConfig()
	if err != nil {
		log.Printf("Warning: Failed to load config: %v\n", err)
		config = Config{
			ActiveModel: "", // No default model selected
			Models:      DefaultModelConfigs,
		}
	}

	// Create sorted list of model keys for UI navigation
	modelKeys := make([]string, 0, len(config.Models))
	for k := range config.Models {
		modelKeys = append(modelKeys, k)
	}
	sort.Strings(modelKeys)

	// Set up API key input field
	tiKey := textinput.New()
	tiKey.Placeholder = "Enter API key here..."
	tiKey.Focus()
	tiKey.CharLimit = 1000
	tiKey.Width = 60

	// Set up API base URL input field
	tiBase := textinput.New()
	tiBase.Placeholder = "http://localhost:8000/v1"
	tiBase.CharLimit = 100
	tiBase.Width = 60

	// Set up model name input field
	tiModelName := textinput.New()
	tiModelName.Placeholder = "Model name for API requests (e.g., llama3)"
	tiModelName.CharLimit = 100
	tiModelName.Width = 60

	// Always start with selection mode, let the user navigate to model selection if needed
	initialMode := selectionMode

	// If no active model is set or it's empty, go to model selection first
	if config.ActiveModel == "" {
		initialMode = modelSelectMode
	}

	m := model{
		currentMode:    initialMode,
		formTypes:      formTypes,
		selectedIndex:  -1,
		answers:        []string{},
		viewport:       viewport.Model{}, // We'll configure this later
		apiKeyInput:    tiKey,
		apiBaseInput:   tiBase,
		modelNameInput: tiModelName,
		focusedInput:   0,
		saveConfig:     true,
		config:         config,
		modelKeys:      modelKeys,
		selectedModel:  config.ActiveModel,
		modelCursor:    indexOf(modelKeys, config.ActiveModel),
		styleThemes:     styleThemes,
		styleThemeIndex: 0,
		styles:         NewStyles(lipgloss.DefaultRenderer(), styleThemes[0]),
		width:          80, // Assuming a default width
	}

	return m
}

// indexOf returns the index of a string in a slice, or 0 if not found
func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return 0
}

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

		// Update the viewport dimensions and style
		m.viewport.Width = width
		m.viewport.Height = height
		m.viewport.Style = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(m.styleThemes[m.styleThemeIndex].Base).
			PaddingLeft(2).
			PaddingRight(2)

		// If in display mode, re-render the markdown to adjust wrapping
		if m.currentMode == displayMode {
			theme := m.styleThemes[m.styleThemeIndex]
			if err := renderMarkdownToViewport(m.content, &m.viewport, theme); err != nil {
				log.Printf("Error re-rendering markdown on resize: %v\n", err)
			}
		}
		// Return without further commands, as resizing is now handled.
		return m, nil

	// Handle other message types based on current mode
	case tea.KeyMsg:
		// Global key handlers that work in any mode
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "esc":
			// Return to main menu from any mode except selection mode
			if m.currentMode != selectionMode {
				m.currentMode = selectionMode
				return m, nil
			}
		case "~":
			// Add global shortcut to switch to model selection mode
			m.currentMode = modelSelectMode
			return m, nil
		case "ctrl+t":
			// Add global shortcut to switch to style selection mode
			m.currentMode = styleSelectMode
			return m, nil
		}

		// Mode-specific key handlers
		switch m.currentMode {
		case selectionMode:
			return m.updateSelectionMode(msg)
		case questionMode:
			return m.updateQuestionMode(msg)
		case displayMode:
			return m.updateDisplayMode(msg)
		case apiKeyInputMode:
			return m.updateAPIKeyInputMode(msg)
		case modelSelectMode:
			return m.updateModelSelectMode(msg)
		case styleSelectMode:
			return m.updateStyleSelectMode(msg)
		}
	}
	return m, nil
}

// updateAPIKeyInputMode handles user input in the API key input mode
func (m model) updateAPIKeyInputMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// Get the currently selected model config
	modelConfig := m.config.Models[m.selectedModel]
	isLocalModel := modelConfig.Provider == ProviderLocal

	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return m, tea.Quit

	case tea.KeyEnter:
		if isLocalModel {
			// For local models, we need to save the API base URL and model name
			baseURL := strings.TrimSpace(m.apiBaseInput.Value())
			modelName := strings.TrimSpace(m.modelNameInput.Value())

			// If base URL is empty, keep default
			if baseURL == "" {
				baseURL = "http://localhost:11434"
			}

			// If model name is empty, use a default
			if modelName == "" {
				modelName = "llama3"
			}

			m.config.Models[m.selectedModel] = ModelConfig{
				Provider:   modelConfig.Provider,
				ModelName:  modelName,
				APIBaseURL: baseURL,
			}
		} else {
			// For cloud models, we need to save the API key and model name
			apiKey := strings.TrimSpace(m.apiKeyInput.Value())
			modelName := strings.TrimSpace(m.modelNameInput.Value())

			// If model name is empty, use the default from the provider
			if modelName == "" {
				if modelConfig.Provider == ProviderOpenAI {
					modelName = "gpt-3.5-turbo"
				} else if modelConfig.Provider == ProviderAnthropic {
					modelName = "claude-3-sonnet-20240229"
				}
			}

			logf("Saved API key length: %d characters, model name: %s", len(apiKey), modelName)

			m.config.Models[m.selectedModel] = ModelConfig{
				Provider:  modelConfig.Provider,
				ModelName: modelName,
				APIKey:    apiKey,
			}
		}

		// Save the config if the checkbox is checked
		if m.saveConfig {
			if err := saveConfig(m.config); err != nil {
				log.Printf("Failed to save config: %v\n", err)
			}
		}

		// Switch to selection mode
		m.currentMode = selectionMode
		return m, nil

	case tea.KeyUp, tea.KeyDown:
		// Cycle between input fields and save checkbox
		// For all providers, cycle through input fields and save checkbox (3 fields total)
		m.focusedInput = (m.focusedInput + 1) % 3

		// Update focus on input fields
		m.apiKeyInput.Blur()
		m.apiBaseInput.Blur()
		m.modelNameInput.Blur()

		if isLocalModel {
			if m.focusedInput == 0 {
				m.apiBaseInput.Focus()
			} else if m.focusedInput == 1 {
				m.modelNameInput.Focus()
			}
		} else {
			if m.focusedInput == 0 {
				m.apiKeyInput.Focus()
			} else if m.focusedInput == 1 {
				m.modelNameInput.Focus()
			}
		}
		return m, nil

	case tea.KeySpace:
		// Toggle save config option when focused on it
		if m.focusedInput == 2 {
			m.saveConfig = !m.saveConfig
		}
		return m, nil
	}

	// Handle input for the appropriate field based on model type and focus
	if isLocalModel {
		if m.focusedInput == 0 {
			m.apiBaseInput, cmd = m.apiBaseInput.Update(msg)
		} else if m.focusedInput == 1 {
			m.modelNameInput, cmd = m.modelNameInput.Update(msg)
		}
	} else {
		if m.focusedInput == 0 {
			m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
		} else if m.focusedInput == 1 {
			m.modelNameInput, cmd = m.modelNameInput.Update(msg)
		}
	}

	return m, cmd
}

func (m model) updateSelectionMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {

		case "q":
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
			// Don't store anything (or store empty string).
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
		case "q":
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

// updateModelSelectMode handles user input in the model selection mode
func (m model) updateModelSelectMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "up", "k":
		if m.modelCursor > 0 {
			m.modelCursor--
		}
	case "down", "j":
		if m.modelCursor < len(m.modelKeys)-1 {
			m.modelCursor++
		}
	case " ", "enter":
		// Select the model at the current cursor position
		m.selectedModel = m.modelKeys[m.modelCursor]
		m.config.ActiveModel = m.selectedModel

		// Save the config
		if err := saveConfig(m.config); err != nil {
			log.Printf("Failed to save config: %v\n", err)
		}

		// Check if the selected model needs configuration
		selectedModelConfig := m.config.Models[m.selectedModel]
		if (selectedModelConfig.Provider != ProviderLocal && selectedModelConfig.APIKey == "") ||
			(selectedModelConfig.Provider == ProviderLocal && selectedModelConfig.APIBaseURL == "") {
			// Go to API key input mode if needed
			m.currentMode = apiKeyInputMode
		} else {
			// Otherwise go to form selection mode
			m.currentMode = selectionMode
		}
	case "c":
		// Configure the model at the current cursor position
		m.selectedModel = m.modelKeys[m.modelCursor]
		m.config.ActiveModel = m.selectedModel
		m.currentMode = apiKeyInputMode
	}

	return m, nil
}

// updateStyleSelectMode handles user input in the style selection mode
func (m model) updateStyleSelectMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.styleThemeIndex > 0 {
			m.styleThemeIndex--
		}
	case "down", "j":
		if m.styleThemeIndex < len(m.styleThemes)-1 {
			m.styleThemeIndex++
		}
	case "enter":
		// Apply the selected theme
		m.styles = NewStyles(lipgloss.DefaultRenderer(), m.styleThemes[m.styleThemeIndex])
		m.currentMode = selectionMode // Return to selection mode
	case "esc":
		m.currentMode = selectionMode // Return to selection mode
	}
	return m, nil
}

// --- [View] ----------------------------------------------------------------

func (m model) View() string {
	var content string
	
	switch m.currentMode {
	case selectionMode:
		content = m.viewSelectionMode()
	case questionMode:
		content = m.viewQuestionMode()
	case displayMode:
		content = m.viewDisplayMode()
	case apiKeyInputMode:
		content = m.viewAPIKeyInputMode()
	case modelSelectMode:
		content = m.viewModelSelectMode()
	case styleSelectMode:
		content = m.viewStyleSelectMode()
	default:
		content = "Unknown mode."
	}

	// Add the status bar at the bottom
	return lipgloss.JoinVertical(
		lipgloss.Left,
		content,
		"\n" + m.renderStatusBar(),
	)
}

// View rendering for API Key Input Mode
func (m model) viewAPIKeyInputMode() string {
	modelConfig := m.config.Models[m.selectedModel]
	isLocalModel := modelConfig.Provider == ProviderLocal

	var title string

	if isLocalModel {
		title = fmt.Sprintf("Configure Ollama: %s", m.selectedModel)

		// Initialize input field values if they're empty
		if m.apiBaseInput.Placeholder == "" {
			m.apiBaseInput.Placeholder = "http://localhost:11434"
		}

		if m.modelNameInput.Placeholder == "" {
			m.modelNameInput.Placeholder = "Model name as shown in 'ollama list' (e.g., llama3)"
		}

		// Set existing values if available
		if modelConfig.APIBaseURL != "" && m.apiBaseInput.Value() == "" {
			m.apiBaseInput.SetValue(modelConfig.APIBaseURL)
		}

		if modelConfig.ModelName != "" && m.modelNameInput.Value() == "" {
			m.modelNameInput.SetValue(modelConfig.ModelName)
		}
	} else {
		providerName := string(modelConfig.Provider)
		providerName = strings.ToUpper(providerName[:1]) + providerName[1:]

		title = fmt.Sprintf("Configure %s API", providerName)

		// Set model name input placeholder and value
		m.modelNameInput.Placeholder = fmt.Sprintf("Model name for %s (e.g., %s)", providerName, modelConfig.ModelName)
		if modelConfig.ModelName != "" && m.modelNameInput.Value() == "" {
			m.modelNameInput.SetValue(modelConfig.ModelName)
		}

		// Set API key placeholder based on provider
		switch modelConfig.Provider {
		case ProviderOpenAI:
			m.apiKeyInput.Placeholder = "Enter your OpenAI API key..."
		case ProviderAnthropic:
			m.apiKeyInput.Placeholder = "Enter your Claude API key..."
		default:
			m.apiKeyInput.Placeholder = "Enter your API key..."
		}

		// Set existing API key if available
		if modelConfig.APIKey != "" && m.apiKeyInput.Value() == "" {
			m.apiKeyInput.SetValue(modelConfig.APIKey)
		}
	}

	s := m.appBoundaryView(title) + "\n\n"

	if isLocalModel {
		// For local models, show both base URL and model name inputs
		baseURLFocused := m.focusedInput == 0
		modelNameFocused := m.focusedInput == 1

		// API Base URL field
		if baseURLFocused {
			s += m.styles.Highlight.Render("API Base URL:") + "\n"
		} else {
			s += "API Base URL:" + "\n"
		}
		s += m.apiBaseInput.View() + "\n"

		// Add URL hint for Ollama users
		s += m.styles.Help.Render("For Ollama: Use http://localhost:11434 (without path segments)") + "\n\n"

		// Model Name field
		if modelNameFocused {
			s += m.styles.Highlight.Render("Model Name:") + "\n"
		} else {
			s += "Model Name:" + "\n"
		}
		s += m.modelNameInput.View() + "\n"

		// Add model name hint for Ollama users
		s += m.styles.Help.Render("For Ollama: Use exactly the model name shown in 'ollama list'") + "\n\n"
	} else {
		// For cloud models, show both API key and model name inputs
		apiKeyFocused := m.focusedInput == 0
		modelNameFocused := m.focusedInput == 1

		// API Key field
		if apiKeyFocused {
			s += m.styles.Highlight.Render("API Key:") + "\n"
		} else {
			s += "API Key:" + "\n"
		}
		s += m.apiKeyInput.View() + "\n\n"

		// Model Name field
		if modelNameFocused {
			s += m.styles.Highlight.Render("Model Name:") + "\n"
		} else {
			s += "Model Name:" + "\n"
		}
		s += m.modelNameInput.View() + "\n"

		if modelConfig.Provider == ProviderAnthropic {
			s += m.styles.Help.Render("For Claude: Examples include claude-3-opus-20240229, claude-3-sonnet-20240229, claude-3-haiku-20240307") + "\n\n"
		} else if modelConfig.Provider == ProviderOpenAI {
			s += m.styles.Help.Render("For OpenAI: Examples include gpt-3.5-turbo, gpt-4, gpt-4-turbo") + "\n\n"
		}
	}

	// Save configuration checkbox
	saveText := "[ ] Save configuration to config file"
	if m.saveConfig {
		saveText = "[x] Save configuration to config file"
	}

	saveFocused := m.focusedInput == 2
	if saveFocused {
		s += m.styles.Highlight.Render(saveText) + "\n\n"
	} else {
		s += saveText + "\n\n"
	}

	// Help text
	s += m.styles.Help.Render("↑/↓: Cycle through fields • Space: Toggle checkbox • Enter: Confirm") + "\n"
	s += m.styles.Help.Render("Esc to return to menu • q to quit")

	return s
}

// View rendering for Selection Mode
func (m model) viewSelectionMode() string {
	s := m.appBoundaryView("Select Report Type") + "\n\n"

	for i, rt := range m.formTypes {
		cursor := "  "
		if m.cursor == i {
			cursor = m.styles.Highlight.Render(">")
		}

		line := fmt.Sprintf("%s %s", cursor, rt.name)

		if m.cursor == i {
			line = m.styles.Highlight.Render(line)
		} else {
			line = m.styles.Help.Render(line)
		}

		s += line + "\n"
	}

	s += "\n" + m.styles.Help.Render("Use ↑/↓ or j/k to navigate • Enter to select") + "\n"
	s += m.styles.Help.Render(fmt.Sprintf("Current model: %s", m.config.ActiveModel)) + "\n"
	s += m.styles.Help.Render("~ to change model • Ctrl+t to change theme • q to quit") + "\n"

	return s
}

// View rendering for Question Mode
func (m model) viewQuestionMode() string {
	currentQ := m.currentForm.questions[m.currentQuestion]
	inputLine := "> " + m.inputString

	s := m.appBoundaryView(fmt.Sprintf("%s - Question %d/%d", m.currentForm.name, m.currentQuestion+1, len(m.currentForm.questions))) + "\n\n"
	s += m.styles.Highlight.Render(fmt.Sprintf("**%s**", currentQ)) + "\n\n"
	s += inputLine

	s += "\n\n" + m.styles.Help.Render("Enter to submit • Ctrl+s to skip") + "\n"
	s += m.styles.Help.Render("Esc to return to menu • q to quit") + "\n"

	return s
}

// View rendering for Display Mode
func (m model) viewDisplayMode() string {
	s := m.appBoundaryView("Generated Output") + "\n\n"
	s += m.viewport.View()
	s += m.styles.Help.Render("\n↑/↓: Scroll • Ctrl+y to copy • Esc to return to menu • q to quit\n")
	return s
}

// viewModelSelectMode renders the model selection interface
func (m model) viewModelSelectMode() string {
	s := m.appBoundaryView("Select AI Provider") + "\n\n"

	for i, key := range m.modelKeys {
		modelConfig := m.config.Models[key]

		cursor := "  "
		if m.modelCursor == i {
			cursor = m.styles.Highlight.Render(">")
		}

		// Get a user-friendly provider name
		var providerDisplay string
		switch modelConfig.Provider {
		case ProviderOpenAI:
			providerDisplay = "OpenAI"
		case ProviderAnthropic:
			providerDisplay = "Anthropic (Claude)"
		case ProviderLocal:
			providerDisplay = "Ollama (Local)"
		default:
			providerDisplay = string(modelConfig.Provider)
		}

		// Format model info to show current model name or configuration status
		var modelInfo string
		if key == "openai" || key == "anthropic" || key == "ollama" {
			// For the main providers, show model name if configured
			if (modelConfig.Provider != ProviderLocal && modelConfig.APIKey != "") ||
				(modelConfig.Provider == ProviderLocal && modelConfig.APIBaseURL != "") {
				modelInfo = fmt.Sprintf("%s - %s", providerDisplay, modelConfig.ModelName)
			} else {
				modelInfo = fmt.Sprintf("%s (not configured)", providerDisplay)
			}
		} else {
			// For custom configurations, show provider and model name
			modelInfo = fmt.Sprintf("%s (%s)", key, providerDisplay)
		}

		// Show configuration status
		status := ""
		if modelConfig.Provider != ProviderLocal && modelConfig.APIKey != "" {
			status = m.styles.StatusHeader.Render(" ✓")
		} else if modelConfig.Provider == ProviderLocal && modelConfig.APIBaseURL != "" {
			status = m.styles.StatusHeader.Render(" ✓")
		}

		line := fmt.Sprintf("%s %s%s", cursor, modelInfo, status)

		if m.modelCursor == i {
			line = m.styles.Highlight.Render(line)
		} else {
			line = m.styles.Help.Render(line)
		}

		s += line + "\n"
	}

	s += "\n" + m.styles.Help.Render("Use ↑/↓ or j/k to navigate • Enter to select") + "\n"
	s += m.styles.Help.Render("c to configure provider • Ctrl+t to change theme") + "\n"
	if m.config.ActiveModel != "" {
		s += m.styles.Help.Render(fmt.Sprintf("Current model: %s - %s", m.config.ActiveModel, m.config.Models[m.config.ActiveModel].ModelName)) + "\n"
	}
	s += m.styles.Help.Render("Esc to return to menu • q to quit") + "\n"

	return s
}

// viewStyleSelectMode renders the style selection interface
func (m model) viewStyleSelectMode() string {
	s := m.appBoundaryView("Select Style Theme") + "\n\n"

	for i, theme := range m.styleThemes {
		cursor := "  "
		if m.styleThemeIndex == i {
			cursor = m.styles.Highlight.Render(">")
		}

		line := fmt.Sprintf("%s %s", cursor, theme.Name)
		if m.styleThemeIndex == i {
			line = m.styles.Highlight.Render(line)
		}

		s += line + "\n"
	}

	s += "\n" + m.styles.Help.Render("Use ↑/↓ to navigate • Enter to select") + "\n"
	s += m.styles.Help.Render("Esc to return to menu • q to quit") + "\n"

	return s
}

// appBoundaryView renders a consistent header for the application
func (m model) appBoundaryView(text string) string {
	theme := m.styleThemes[m.styleThemeIndex]
	return lipgloss.PlaceHorizontal(
		m.width,
		lipgloss.Left,
		m.styles.HeaderText.Render(text),
		lipgloss.WithWhitespaceChars("/"),
		lipgloss.WithWhitespaceForeground(theme.Base),
	)
}

// appErrorBoundaryView renders a consistent error header for the application
func (m model) appErrorBoundaryView(text string) string {
	theme := m.styleThemes[m.styleThemeIndex]
	return lipgloss.PlaceHorizontal(
		m.width,
		lipgloss.Left,
		m.styles.ErrorHeaderText.Render(text),
		lipgloss.WithWhitespaceChars("/"),
		lipgloss.WithWhitespaceForeground(theme.Error),
	)
}

// --- [ I/O ] ------------------------------------
//
// This section defines helper functions to take the user input in the viewport and pass it to the LLM.
//

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
func renderMarkdownToViewport(md string, vp *viewport.Model, theme StyleTheme) error {
	// Create base styles using lipgloss
	baseStyle := lipgloss.NewStyle().Foreground(theme.Base)
	headerStyle := lipgloss.NewStyle().
		Foreground(theme.Base).
		Bold(true)

	// Prepare a Glamour renderer with minimal styling
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

	// Post-process the rendered content to apply our styles
	lines := strings.Split(rendered, "\n")
	var styledLines []string

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "# "):
			// H1 headers
			line = headerStyle.Render(line)
		case strings.HasPrefix(line, "## "):
			// H2 headers
			line = headerStyle.Render(line)
		case strings.HasPrefix(line, "### "):
			// H3 headers
			line = headerStyle.Render(line)
		default:
			// Regular text
			if strings.TrimSpace(line) != "" {
				line = baseStyle.Render(line)
			}
		}
		styledLines = append(styledLines, line)
	}

	// Join the lines back together
	styledContent := strings.Join(styledLines, "\n")

	// Ensure the rendered content ends with a newline for proper display
	styledContent = strings.TrimRight(styledContent, "\n") + "\n"

	// Set the content in the viewport
	vp.SetContent(styledContent)
	return nil
}

// handleFormCompletion combines the other helper functions to pass the input on to the LLM.
func handleFormCompletion(m model) model {
	// Build the Markdown
	md := buildSelectedMarkdown(m)
	theme := m.styleThemes[m.styleThemeIndex]
	if err := renderMarkdownToViewport(md, &m.viewport, theme); err != nil {
		logf("Error rendering markdown: %v", err)
	}
	m.content = md

	// Update viewport style with theme colors
	m.viewport.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.Base).
		PaddingLeft(2).
		PaddingRight(2)

	// Check if the active model has the required API key or base URL
	activeModelConfig := m.config.Models[m.config.ActiveModel]
	if (activeModelConfig.Provider != ProviderLocal && activeModelConfig.APIKey == "") ||
		(activeModelConfig.Provider == ProviderLocal && activeModelConfig.APIBaseURL == "") {
		// Go to API key input mode if needed
		m.currentMode = apiKeyInputMode
		return m
	}

	// Create a channel to capture the ChatGPT request result
	done := make(chan error, 1)

	// Show a simple "Processing..." message in the viewport
	processingMsg := fmt.Sprintf("## Processing with %s\n\nGenerating summary...", m.config.ActiveModel)
	if err := renderMarkdownToViewport(processingMsg, &m.viewport, theme); err != nil {
		logf("Error rendering processing message: %v", err)
	}

	// Launch ChatGPT request concurrently
	go func() {
		err := makeLLMRequest(context.TODO(), &m, md)
		done <- err
	}()

	// Create a cancellable context for the spinner
	spinnerCtx, cancelSpinner := context.WithCancel(context.Background())
	defer cancelSpinner()

	// Start the spinner in a separate goroutine
	go func() {
		err := spinner.New().
			Context(spinnerCtx).
			Action(func() {
				// Instead of sleeping, just block until the spinnerCtx is cancelled
				<-spinnerCtx.Done()
			}).
			Accessible(rand.Int()%2 == 0).
			Run()
		if err != nil {
			logf("Spinner error: %v", err)
		}
	}()

	// Wait for the ChatGPT request to complete
	if err := <-done; err != nil {
		logf("Error from LLM: %v", err)
		// Show error in viewport
		errorMsg := fmt.Sprintf("## Error\n\nFailed to get response from %s: %v\n\nCheck the log file for details.",
			m.config.ActiveModel, err)
		if err := renderMarkdownToViewport(errorMsg, &m.viewport, theme); err != nil {
			logf("Error rendering error message: %v", err)
		}
	}

	// Cancel the spinner once the ChatGPT request is done
	cancelSpinner()

	logf("Request completed")
	m.currentMode = displayMode
	return m
}

// ---[[ LLM Requests ]]------------------------------------------------------------

// makeLLMRequest encapsulates the LLM API call & viewport re-rendering.
func makeLLMRequest(ctx context.Context, m *model, md string) error {
	// Get the active model configuration
	activeModelConfig := m.config.Models[m.config.ActiveModel]

	// Append the prompt to the generated response
	combinedPrompt := m.currentForm.prompt + "\n\n" + md

	// Step 1 - Call the LLM with the generated response Markdown
	resp, err := processFormWithLLM(ctx, activeModelConfig, combinedPrompt)
	if err != nil {
		return fmt.Errorf("LLM API error: %v", err)
	}

	m.gptRawOutput = resp // Store the raw output

	// Step 2 - Append the LLM's response as an optional "analysis" or "summary"
	summary := "\n## Ticket Summary\n\n" + resp
	appendedContent := md + summary

	// Step 3 - Re-render the viewport with the appended content
	if err := renderMarkdownToViewport(appendedContent, &m.viewport, m.styleThemes[m.styleThemeIndex]); err != nil {
		return fmt.Errorf("render markdown error: %v", err)
	}
	m.content = appendedContent
	return nil
}

func processFormWithLLM(ctx context.Context, modelConfig ModelConfig, content string) (string, error) {
	logf("Processing request with provider: %s, model: %s", modelConfig.Provider, modelConfig.ModelName)

	// Create the appropriate LLM client based on the model configuration
	client, err := CreateLLMClient(modelConfig)
	if err != nil {
		logf("ERROR: Failed to create LLM client: %v", err)
		return "", fmt.Errorf("failed to create LLM client: %v", err)
	}

	logf("Client created successfully, sending request to %s", modelConfig.Provider)

	// Calculate prompt size metrics
	promptCharLength := len(content)
	promptLines := len(strings.Split(content, "\n"))
	logf("Sending prompt with %d characters, %d lines", promptCharLength, promptLines)

	// Use the client to complete the prompt
	response, err := client.Complete(ctx, content)
	if err != nil {
		logf("ERROR: %s completion failed: %v", modelConfig.Provider, err)
		return "", err
	}

	logf("Request completed successfully, received %d character response", len(response))
	return response, nil
}

// ---[[ LLM Client Interface ]]------------------------------------------------------------

// LLMClient defines the interface for different LLM providers
type LLMClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// OpenAIClient implements the LLMClient interface for OpenAI
type OpenAIClient struct {
	client *openai.Client
	model  string
}

func NewOpenAIClient(apiKey, model string) *OpenAIClient {
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &OpenAIClient{
		client: client,
		model:  model,
	}
}

func (c *OpenAIClient) Complete(ctx context.Context, prompt string) (string, error) {
	logf("OpenAI: Sending request to model %s", c.model)

	params := openai.ChatCompletionNewParams{
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		}),
		Model: openai.F(c.model),
	}

	logf("OpenAI: Calling Chat Completions API")
	chatCompletion, err := c.client.Chat.Completions.New(ctx, params)

	if err != nil {
		logf("OpenAI ERROR: API request failed: %v", err)
		return "", err
	}

	logf("OpenAI: Request successful, received %d choices", len(chatCompletion.Choices))
	if len(chatCompletion.Choices) > 0 {
		responseLength := len(chatCompletion.Choices[0].Message.Content)
		logf("OpenAI: Response length: %d characters", responseLength)
	}

	return chatCompletion.Choices[0].Message.Content, nil
}

// ClaudeClient implements the LLMClient interface for Anthropic
type ClaudeClient struct {
	client *anthropic.Client
	model  string
}

func NewClaudeClient(apiKey, model string) *ClaudeClient {
	client := anthropic.NewClient(apiKey)

	return &ClaudeClient{
		client: client,
		model:  model,
	}
}

func (c *ClaudeClient) Complete(ctx context.Context, prompt string) (string, error) {
	logf("Claude: Sending request to model %s", c.model)

	// Log model version info to help with debugging
	logf("Claude: Using client with model %s", c.model)

	// Use the go-anthropic client to create a messages completion
	mesReq := anthropic.MessagesRequest{
		Model: c.model,
		Messages: []anthropic.Message{
			{
				Role: anthropic.RoleUser,
				Content: []anthropic.MessageContent{
					{
						Type: "text",
						Text: &prompt,
					},
				},
			},
		},
		MaxTokens: 4096,
	}

	logf("Claude: Sending message to %s with max tokens: %d", c.model, mesReq.MaxTokens)

	resp, err := c.client.CreateMessages(ctx, mesReq)
	if err != nil {
		var apiErr *anthropic.APIError
		if errors.As(err, &apiErr) {
			logf("Claude ERROR: API error (type: %s): %s", apiErr.Type, apiErr.Message)

			// Provide helpful guidance for model not found errors
			if apiErr.Type == "not_found_error" && strings.Contains(apiErr.Message, "model") {
				logf("Claude ERROR: The specified model name '%s' was not found", c.model)
				logf("Claude INFO: Available Claude models typically include:")
				logf("  - claude-3-opus-20240229")
				logf("  - claude-3-sonnet-20240229")
				logf("  - claude-3-haiku-20240307")
				return "", fmt.Errorf("Claude API error: Model '%s' not found. Try using claude-3-opus-20240229, claude-3-sonnet-20240229, or claude-3-haiku-20240307", c.model)
			}

			return "", fmt.Errorf("Claude API error (type: %s): %s", apiErr.Type, apiErr.Message)
		}
		logf("Claude ERROR: Unknown error: %v", err)
		return "", fmt.Errorf("Claude API error: %v", err)
	}

	logf("Claude: Response received! ID: %s, Model: %s", resp.ID, resp.Model)

	// Get the response text from the content blocks
	if len(resp.Content) > 0 {
		for _, content := range resp.Content {
			if content.Type == "text" {
				return content.Text, nil
			}
		}
	}

	return "", fmt.Errorf("Claude returned no text content")
}

// LocalLLMClient implements the LLMClient interface for local LLMs
type LocalLLMClient struct {
	baseURL string
	model   string
}

func NewLocalLLMClient(baseURL, model string) *LocalLLMClient {
	return &LocalLLMClient{
		baseURL: baseURL,
		model:   model,
	}
}

func (c *LocalLLMClient) Complete(ctx context.Context, prompt string) (string, error) {
	logf("Local LLM: Sending request to %s, model: %s", c.baseURL, c.model)

	// Format the base URL correctly for the Ollama API
	baseURL := c.baseURL

	// Strip trailing slashes
	baseURL = strings.TrimSuffix(baseURL, "/")

	// For Ollama, use the simpler API endpoint format
	if strings.Contains(baseURL, "localhost:11434") || strings.Contains(baseURL, "127.0.0.1:11434") {
		// For Ollama, use its native API format: /api/chat
		logf("Local LLM: Detected Ollama server, using native API endpoint")
		baseURL = baseURL + "/api/chat"
	} else {
		// For OpenAI-compatible APIs, use the standard endpoint format
		// First, check for existing path components to avoid duplication
		if strings.Contains(baseURL, "/v1/chat/completions") {
			// URL already contains the correct full path, use as is
			logf("Local LLM: URL already contains complete path")
		} else if strings.Contains(baseURL, "/chat/completions") {
			// URL already contains the correct endpoint, use as is
			logf("Local LLM: URL already contains chat/completions endpoint")
		} else if strings.HasSuffix(baseURL, "/v1") {
			// URL ends with /v1, add /chat/completions
			baseURL = baseURL + "/chat/completions"
		} else {
			// Add the standard endpoint path
			baseURL = baseURL + "/v1/chat/completions"
		}
	}

	logf("Local LLM: Using final endpoint URL: %s", baseURL)

	// Create a client with the exact URL
	client := openai.NewClient(
		option.WithBaseURL(baseURL),
	)

	// For Ollama's native API format
	if strings.Contains(baseURL, "/api/chat") {
		// Create Ollama-specific request body
		type OllamaMessage struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}

		type OllamaRequest struct {
			Model    string          `json:"model"`
			Messages []OllamaMessage `json:"messages"`
			Stream   bool            `json:"stream"`
		}

		ollamaReq := OllamaRequest{
			Model: c.model,
			Messages: []OllamaMessage{
				{
					Role:    "user",
					Content: prompt,
				},
			},
			Stream: false, // Don't stream for simpler response handling
		}

		logf("Local LLM: Using Ollama-specific request format")
		jsonBody, err := json.Marshal(ollamaReq)
		if err != nil {
			return "", fmt.Errorf("failed to marshal Ollama request: %v", err)
		}

		// Create HTTP request
		req, err := http.NewRequestWithContext(ctx, "POST", baseURL, bytes.NewBuffer(jsonBody))
		if err != nil {
			return "", fmt.Errorf("failed to create HTTP request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		// Send request
		httpClient := &http.Client{
			Timeout: 120 * time.Second, // Set a longer timeout for LLM responses
		}

		logf("Local LLM: Sending request to Ollama API at %s", baseURL)
		resp, err := httpClient.Do(req)
		if err != nil {
			logf("Local LLM ERROR: API request failed: %v", err)
			return "", fmt.Errorf("Local LLM API error: %v", err)
		}
		defer resp.Body.Close()

		// Log response status
		logf("Local LLM: Received response with status: %s", resp.Status)

		// Check for non-200 status code
		if resp.StatusCode != http.StatusOK {
			// Read error response body
			errBody, _ := ioutil.ReadAll(resp.Body)
			logf("Local LLM ERROR: Bad status code: %d, response: %s", resp.StatusCode, string(errBody))
			return "", fmt.Errorf("Ollama API returned %s: %s", resp.Status, string(errBody))
		}

		// Read the full response body
		responseBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			logf("Local LLM ERROR: Failed to read response body: %v", err)
			return "", fmt.Errorf("failed to read Ollama response: %v", err)
		}

		// Log the raw response for debugging
		logf("Local LLM: Raw response from Ollama (%d bytes): %.500s...", len(responseBody), string(responseBody))

		// Parse response
		var result struct {
			Message struct {
				Content string `json:"content"`
				Role    string `json:"role"`
			} `json:"message"`
			Done bool `json:"done"`
		}

		if err := json.Unmarshal(responseBody, &result); err != nil {
			logf("Local LLM ERROR: Failed to parse Ollama response JSON: %v", err)
			logf("Local LLM ERROR: Response causing the error: %.500s...", string(responseBody))
			return "", fmt.Errorf("failed to parse Ollama response: %v", err)
		}

		responseContent := result.Message.Content
		responseRole := result.Message.Role
		logf("Local LLM: Response content length: %d characters, role: %s", len(responseContent), responseRole)

		// Log a substantial preview of the response
		if len(responseContent) > 0 {
			previewLength := 500
			if len(responseContent) < previewLength {
				previewLength = len(responseContent)
			}
			logf("Local LLM: Response preview: %s", responseContent[:previewLength])

			// Also log the end of the content if it's longer
			if len(responseContent) > previewLength {
				endPreviewStart := len(responseContent) - 100
				if endPreviewStart < previewLength {
					endPreviewStart = previewLength
				}
				logf("Local LLM: Response end: %s", responseContent[endPreviewStart:])
			}
		} else {
			logf("Local LLM WARNING: Received empty response content")
		}

		return responseContent, nil
	}

	// Standard OpenAI-compatible API for non-Ollama servers
	// Structure the request according to OpenAI's expectations
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage(prompt),
	}

	params := openai.ChatCompletionNewParams{
		Messages: openai.F(messages),
		Model:    openai.F(c.model),
	}

	logf("Local LLM: Sending request to model: %s with prompt: %.100s...", c.model, prompt)

	// Make the API call
	chatCompletion, err := client.Chat.Completions.New(ctx, params)

	if err != nil {
		logf("Local LLM ERROR: API request failed: %v", err)

		// Additional debugging information
		logf("Request details - URL: %s, Model: %s", baseURL, c.model)
		logf("Error details: %v", err)

		return "", fmt.Errorf("Local LLM API error: %v", err)
	}

	// Debug the response
	logf("Local LLM: Response received, choices: %d", len(chatCompletion.Choices))

	if len(chatCompletion.Choices) == 0 {
		return "", fmt.Errorf("No content returned from the LLM")
	}

	responseContent := chatCompletion.Choices[0].Message.Content
	logf("Local LLM: Response content length: %d", len(responseContent))
	logf("Local LLM: Response preview: %.100s...", responseContent)

	return responseContent, nil
}

// CreateLLMClient creates an appropriate client based on the model configuration
func CreateLLMClient(config ModelConfig) (LLMClient, error) {
	logf("Creating LLM client for provider: %s, model: %s", config.Provider, config.ModelName)

	switch config.Provider {
	case ProviderOpenAI:
		if config.APIKey == "" {
			logf("ERROR: OpenAI API key is missing")
			return nil, fmt.Errorf("OpenAI API key is required")
		}

		// Log key length and first/last characters for debugging
		keyLength := len(config.APIKey)
		logf("OpenAI: Using API key with length: %d characters", keyLength)

		if keyLength < 20 {
			logf("WARNING: OpenAI API key seems too short (length: %d), may be invalid", keyLength)
		}

		if keyLength >= 10 {
			firstChars := config.APIKey[:4]
			lastChars := config.APIKey[keyLength-4:]
			logf("OpenAI: Key prefix: %s..., suffix: ...%s", firstChars, lastChars)
		}

		return NewOpenAIClient(config.APIKey, config.ModelName), nil

	case ProviderAnthropic:
		if config.APIKey == "" {
			logf("ERROR: Claude API key is missing")
			return nil, fmt.Errorf("Claude API key is required")
		}

		keyLength := len(config.APIKey)
		logf("Claude: Using API key with length: %d characters", keyLength)

		if keyLength < 20 {
			logf("WARNING: Claude API key seems too short (length: %d), may be invalid", keyLength)
		}

		return NewClaudeClient(config.APIKey, config.ModelName), nil

	case ProviderLocal:
		if config.APIBaseURL == "" {
			logf("ERROR: Local LLM API base URL is missing")
			return nil, fmt.Errorf("API base URL is required for local models")
		}

		logf("Local LLM: Using API base URL: %s", config.APIBaseURL)

		// Validate model name
		modelName := config.ModelName
		if modelName == "" {
			logf("WARNING: Local LLM model name is empty, using default 'llama3'")
			modelName = "llama3"
		}

		logf("Local LLM: Using model name: %s", modelName)

		// Basic URL validation
		if !strings.HasPrefix(config.APIBaseURL, "http://") && !strings.HasPrefix(config.APIBaseURL, "https://") {
			logf("WARNING: Local LLM API URL doesn't start with http:// or https://: %s", config.APIBaseURL)
		}

		return NewLocalLLMClient(config.APIBaseURL, modelName), nil

	default:
		logf("ERROR: Unsupported provider: %s", config.Provider)
		return nil, fmt.Errorf("unsupported provider: %s", config.Provider)
	}
}

// ---[ Main ]------------------------------------------------------------
func main() {
	// Initialize logging
	if err := setupLogging(); err != nil {
		fmt.Printf("Warning: Failed to setup logging: %v\n", err)
	}
	defer closeLogging()

	logf("Starting TicketSummaryTool")

	p := tea.NewProgram(initialModel())
	if err := p.Start(); err != nil {
		logf("Error starting program: %v", err)
		fmt.Printf("Error starting program: %v\n", err)
		os.Exit(1)
	}

	logf("TicketSummaryTool completed successfully")
}

// renderStatusBar creates a status bar showing the current mode and other relevant information
func (m model) renderStatusBar() string {

	// Get the current mode name
	var modeName string
	switch m.currentMode {
	case selectionMode:
		modeName = "Selection"
	case questionMode:
		modeName = "Question"
	case displayMode:
		modeName = "Display"
	case apiKeyInputMode:
		modeName = "API Config"
	case modelSelectMode:
		modeName = "Model Select"
	case styleSelectMode:
		modeName = "Style Select"
	}

	// Create the mode indicator
	modeIndicator := m.styles.StatusMode.Render(modeName)
	
	// Create the model indicator
	modelInfo := m.styles.StatusText.Render(fmt.Sprintf(" Model: %s", m.config.ActiveModel))
	
	// Join the components
	bar := lipgloss.JoinHorizontal(lipgloss.Top,
		modeIndicator,
		modelInfo,
	)

	// Render the full bar with the theme's status bar style
	return m.styles.StatusBar.Width(m.width).Render(bar)
}
