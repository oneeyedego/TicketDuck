<p align="center">
  <img width="250" height="250" src="images/icon.png">
</p>

### TicketDuck
Like a rubber ducky that talks back, TicketDuck is a lightweight tool that helps you make your messy stream-of-consciousness more coherent (your friends and colleagues will thank you). 

Will it literally debug your life? Absolutely not. How about make you a better leader and team member? Maybe, but that's a lot to ask of a tiny program. Will it make documenting your troubleshooting and development thought process easier? Hopefully yes!

There's no one right way to use this tool, but here's a suggestion:
1. Incident Reports: What happened? What did you do? Why did you do it? Did it work? What did you learn?
2. Commit Messages: What did you do? Why did you do it? What did you learn?
3. Service Requests: What do you want? Why do you want it? What will you do with it?
4. Development Ticket: Is this a feature, bug or chore? What is the current behavior? How do you want to change, modify, or add behavior? Why do you want this change? What are the benefits? What are the acceptance criteria for this change?



Built using Charmbracelet's tools:

- https://github.com/charmbracelet/bubbletea (TUI framework)
- https://github.com/charmbracelet/bubbles (TUI components)
- https://github.com/charmbracelet/glamour (Markdown rendering)
- https://github.com/charmbracelet/lipgloss (CSS-like styling)

Other non-standard dependencies:

- "github.com/liushuangls/go-anthropic" (Community Claude API wrapper for Go) 
- "github.com/openai/openai-go" (OpenAI API for Go)
- "github.com/acarl005/stripansi" (Helps format TUI output)
- "github.com/atotto/clipboard" (Takes that output and pipes it to the clipboard)

Supported platforms: GNU/Linux, MacOS, BSD

Knock yourself out platforms: Windows 
