package tui

import (
	"fmt"
	"strings"
)

// toolbar renders a single-line status bar at the bottom of the viewport.
type toolbar struct {
	modelName string
	provider  string
	turnCount int
	totalCost float64
	status    string
}

func newToolbar(modelName, provider string) toolbar {
	return toolbar{
		modelName: modelName,
		provider:  provider,
		status:    "ready",
	}
}

// View renders the toolbar at the given width.
func (t toolbar) View(width int) string {
	left := fmt.Sprintf(" %s | %s | turns: %d | $%.4f",
		t.modelName, t.provider, t.turnCount, t.totalCost)

	right := fmt.Sprintf(" %s ", t.status)

	// Style the status indicator
	styledRight := statusActiveStyle.Render(right)

	gap := max(width-len(left)-len(right), 0)

	bar := left + strings.Repeat(" ", gap) + styledRight
	return statusBarStyle.Render(bar)
}

// SetStatus updates the status text.
func (t *toolbar) SetStatus(status string) {
	t.status = status
}

// UpdateCost sets the total cost.
func (t *toolbar) UpdateCost(cost float64) {
	t.totalCost = cost
}

// IncrementTurn adds one to the turn counter.
func (t *toolbar) IncrementTurn() {
	t.turnCount++
}
