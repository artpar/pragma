package render

import (
	"strings"

	"github.com/artpar/pragma/internal/observe"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
)

// MarkdownRenderer wraps glamour for terminal markdown rendering.
// It handles width-aware rendering and dark/light mode detection.
type MarkdownRenderer struct {
	renderer *glamour.TermRenderer
	width    int
	darkMode bool
}

// NewMarkdownRenderer creates a renderer that detects terminal background.
func NewMarkdownRenderer(width int) *MarkdownRenderer {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	darkMode := lipgloss.HasDarkBackground()
	r := &MarkdownRenderer{
		width:    width,
		darkMode: darkMode,
	}
	r.renderer = r.createRenderer()
	observe.GlobalTrace("return: r")
	return r
}

// Render renders markdown text to styled terminal output.
// Returns the input unchanged if glamour fails (graceful degradation).
func (r *MarkdownRenderer) Render(text string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if text == "" {
		observe.GlobalTrace("if: text == \"\"")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	out, err := r.renderer.Render(text)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: text")
		return text
	}

	out = strings.TrimRight(out, "\n")
	observe.GlobalTrace("return: out")
	return out
}

// UpdateWidth recreates the renderer for the new terminal width.
func (r *MarkdownRenderer) UpdateWidth(width int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if width == r.width {
		observe.GlobalTrace("if: width == r.width")
		return
	}
	r.width = width
	r.renderer = r.createRenderer()
}

func (r *MarkdownRenderer) createRenderer() *glamour.TermRenderer {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	style := styles.DarkStyleConfig
	if !r.darkMode {
		observe.GlobalTrace("if: !r.darkMode")
		style = styles.LightStyleConfig
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(r.width),
	)
	if err != nil {
		observe.GlobalTrace("if: err != nil")

		renderer, _ = glamour.NewTermRenderer(
			glamour.WithWordWrap(r.width),
			glamour.WithAutoStyle(),
		)
	}
	observe.GlobalTrace("return: renderer")
	return renderer
}
