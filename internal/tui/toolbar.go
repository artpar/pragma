package tui

import (
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/tui/render"
)

// toolbar renders a single-line status bar at the bottom of the viewport.
type toolbar struct {
	modelName    string
	provider     string
	workspace    string
	turnCount    int
	totalCost    float64
	status       string
	inputTokens  int
	outputTokens int
	contextSize  int
}

func newToolbar(modelName, provider, workspace string) toolbar {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: toolbar{\n\tmodelName:\tmodelName,\n\tprovider:\tprovider,\n\tworkspace:\tworkspace,\n\t...")
	return toolbar{
		modelName: modelName,
		provider:  provider,
		workspace: workspace,
		status:    "ready",
	}
}

// View renders the toolbar at the given width.
func (t toolbar) View(width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	left := fmt.Sprintf(" %s %s", render.BlackCircle, t.modelName)

	if width >= 80 && t.workspace != "" {
		observe.GlobalTrace("if: width >= 80 && t.workspace != \"\"")
		left += " | " + t.workspace
	}

	left += fmt.Sprintf(" | turns: %d | $%.2f", t.turnCount, t.totalCost)

	if width >= 60 && t.contextSize > 0 {
		observe.GlobalTrace("if: width >= 60 && t.contextSize > 0")
		total := t.inputTokens + t.outputTokens
		pct := float64(total) / float64(t.contextSize) * 100
		tokenStr := fmt.Sprintf(" | %s/%s (%d%%)",
			formatTokens(t.inputTokens),
			formatTokens(t.outputTokens),
			int(pct))

		tokenStyled := t.styleTokenStr(tokenStr, pct)
		left += tokenStyled
	}

	right := fmt.Sprintf(" %s ", t.status)
	styledRight := statusActiveStyle.Render(right)

	gap := max(width-len(left)-len(right), 0)
	bar := left + strings.Repeat(" ", gap) + styledRight
	observe.GlobalTrace("return: statusBarStyle.Render(bar)")

	return statusBarStyle.Render(bar)
}

// styleTokenStr applies color coding based on context usage percentage.
func (t toolbar) styleTokenStr(s string, pct float64) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch {
	case pct >= 80:
		observe.GlobalTrace("case: pct >= 80")
		return statusTokensRedStyle.Render(s)
	case pct >= 50:
		observe.GlobalTrace("case: pct >= 50")
		return statusTokensYellowStyle.Render(s)
	default:
		observe.GlobalTrace("default")
		return statusTokensGreenStyle.Render(s)
	}
}

// SetStatus updates the status text.
func (t *toolbar) SetStatus(status string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	t.status = status
}

// UpdateCost sets the total cost.
func (t *toolbar) UpdateCost(cost float64) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	t.totalCost = cost
}

// UpdateTokens updates the token display values.
func (t *toolbar) UpdateTokens(input, output, contextSize int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	t.inputTokens = input
	t.outputTokens = output
	t.contextSize = contextSize
}

// IncrementTurn adds one to the turn counter.
func (t *toolbar) IncrementTurn() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	t.turnCount++
}

// formatTokens formats a token count for display.
func formatTokens(n int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if n >= 1_000_000 {
		observe.GlobalTrace("if: n >= 1_000_000")
		observe.GlobalTrace("return: fmt.Sprintf(\"%.1fM\", float64(n)/1_000_000)")
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		observe.GlobalTrace("if: n >= 1_000")
		observe.GlobalTrace("return: fmt.Sprintf(\"%.1fK\", float64(n)/1_000)")
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	observe.GlobalTrace("return: fmt.Sprintf(\"%d\", n)")
	return fmt.Sprintf("%d", n)
}
