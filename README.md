<p align="center">
  <img width="250" height="250" src="images/icon.png">
</p>

## TicketDuck
Like a rubber ducky that talks back, TicketDuck is a lightweight tool that helps you make your messy stream-of-consciousness more coherent (your friends and colleagues will thank you). 

Will it literally debug your life? Absolutely not. How about make you a better leader and team member? Maybe, but that's a lot to ask of a tiny program. Will it make documenting your troubleshooting and development thought process easier? Hopefully yes!

There's no one right way to use this tool, but here's a suggestion:
1. Incident Reports: What happened? What did you do? Why did you do it? Did it work? What did you learn?
2. Commit Messages: What did you do? Why did you do it? What did you learn?
3. Service Requests: What do you want? Why do you want it? What will you do with it?
4. Development Ticket: Is this a feature, bug or chore? What is the current behavior? How do you want to change, modify, or add behavior? Why do you want this change? What are the benefits? What are the acceptance criteria for this change?

### Getting started

  - After launching the application, configure the model that you'd like to use.
  - Once that's done, select your form type from the main menu.
  - Answer each question in the form, or skip the ones that you don't like. 
  - Submit the form, copy the output, and edit it down to what makes sense.
  - Did you save time? Maybe not, but the words were put to the page, and the task of documenting your work has been split into smaller chunks!

### Key bindings

#### Global Key Bindings
- `Ctrl+q`: Quit the application
- `Esc`: Return to main menu (from any mode except selection mode)
- `~`: Switch to model selection mode
- `Ctrl+t`: Switch to style selection mode

#### Selection Mode
- `↑/↓` or `j/k`: Navigate through form types
- `Enter` or `Space`: Select a form type

#### Question Mode
- `Enter`: Submit answer and move to next question
- `Ctrl+s`: Skip current question
- `Backspace`: Delete last character
- `Esc`: Return to main menu

#### Display Mode
- `↑/↓` or `j/k`: Scroll up/down one line
- `PgUp/PgDown`: Scroll up/down one page
- `g`: Press twice to jump to top
- `G`: Jump to bottom
- `Ctrl+y`: Copy plain text to clipboard
- `Esc`: Return to main menu

#### Model Selection Mode
- `↑/↓` or `j/k`: Navigate through model options
- `Enter` or `Space`: Select a model
- `c`: Configure the selected model
- `Esc`: Return to main menu

#### Style Selection Mode
- `↑/↓` or `j/k`: Navigate through style themes
- `Enter`: Apply selected theme
- `Esc`: Return to main menu

#### API Key Input Mode
- `↑/↓`: Cycle through input fields
- `Space`: Toggle save configuration checkbox
- `Enter`: Save configuration and return to menu
- `Esc`: Return to main menu

Built using Charmbracelet's tools:

- https://github.com/charmbracelet/bubbletea (TUI framework)
- https://github.com/charmbracelet/bubbles (TUI components)
- https://github.com/charmbracelet/glamour (Markdown rendering)
- https://github.com/charmbracelet/lipgloss (CSS-like styling)

Other non-standard dependencies:

- https://github.com/liushuangls/go-anthropic (Community Claude API wrapper for Go) 
- https://github.com/openai/openai-go (OpenAI API for Go)
- https://github.com/acarl005/stripansi (Helps format TUI output)
- https://github.com/atotto/clipboard (Takes that output and pipes it to the clipboard)

Supported platforms: GNU/Linux, MacOS, BSD


For submitting issues, we ask that you try using the tool to do so, and if you run into any unexpected behavior, we ask that you attach the client logs, which should be located in ```~/.ticketduck/logs/``` .

