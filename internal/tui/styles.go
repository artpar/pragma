package tui

import "github.com/charmbracelet/lipgloss"

// All colors use AdaptiveColor to work on both light and dark terminals.
// This prevents GitHub issues #1302, #16514, #34905, #41098, #45084
// where hardcoded truecolor was unreadable on certain backgrounds.
//
// Styles used directly in the tui package (handlers, model, permission, ask, input).
// Rendering styles for tool calls, diffs, brackets, etc. live in render/content.go
// and render/toolrender.go (the render sub-package owns its own styles).

var (
	// Message roles — used by handlers.go for label rendering
	userLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "30", Dark: "86"}) // cyan

	// Content types — used by handlers.go for inline status messages
	thinkingStyle = lipgloss.NewStyle().
			Faint(true).
			Italic(true)

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "124", Dark: "196"}) // red

	// Status bar — used by toolbar.go
	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "252", Dark: "252"}).
			Background(lipgloss.AdaptiveColor{Light: "236", Dark: "236"})

	statusActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "114"}). // green
				Background(lipgloss.AdaptiveColor{Light: "236", Dark: "236"})

	statusTokensGreenStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "114"}).
				Background(lipgloss.AdaptiveColor{Light: "236", Dark: "236"})

	statusTokensYellowStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "214"}).
				Background(lipgloss.AdaptiveColor{Light: "236", Dark: "236"})

	statusTokensRedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "124", Dark: "196"}).
				Background(lipgloss.AdaptiveColor{Light: "236", Dark: "236"})

	// Permission dialog — used by permission.go
	// Top-only border matching TS PermissionDialog (borderBottom=false borderLeft=false borderRight=false)
	permDialogBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.AdaptiveColor{Light: "130", Dark: "214"}). // yellow
				BorderBottom(false).
				BorderLeft(false).
				BorderRight(false).
				Padding(0, 1)

	permTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "214"})

	permSubtitleStyle = lipgloss.NewStyle().
				Faint(true)

	permSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "114"}) // green

	permUnselectedStyle = lipgloss.NewStyle().
				Faint(true)

	// Diff styles for permission previews
	permDiffAdd = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "114"}) // green

	permDiffRemove = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "124", Dark: "210"}) // red

	permCommandStyle = lipgloss.NewStyle().
				Bold(true)

	// Input — used by input.go
	inputPromptStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "30", Dark: "86"}).
				Bold(true)

	// inputPromptDimStyle is the streaming-state variant of the input prompt.
	// Same cyan color but faint, giving visual feedback that the model is responding.
	inputPromptDimStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "30", Dark: "86"}).
				Faint(true)
)
