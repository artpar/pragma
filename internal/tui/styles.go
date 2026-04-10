package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Message roles
	userLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("6")) // cyan

	assistantLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("5")) // magenta

	// Content types
	thinkingStyle = lipgloss.NewStyle().
			Faint(true).
			Italic(true)

	toolCallStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3")) // yellow

	toolResultStyle = lipgloss.NewStyle().
			Faint(true)

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("1")) // red

	// Permission dialog
	permDialogBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("3")). // yellow border
				Padding(1, 2)

	permTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("3"))

	permSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("2")) // green

	permUnselectedStyle = lipgloss.NewStyle().
				Faint(true)

	// Status bar
	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")).
			Background(lipgloss.Color("0"))

	statusActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("2")). // green
				Background(lipgloss.Color("0"))

	// Input
	inputPromptStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("6")).
				Bold(true)

)
