package tui

import (
	"fmt"
	"github.com/artpar/gogent/internal/observe"
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: toolbar{\n\tmodelName:\tmodelName,\n\tprovider:\tprovider,\n\tstatus:\t\t\"ready\",\n}")
	observe.GlobalTrace("return: toolbar{\n\tmodelName:\tmodelName,\n\tprovider:\tprovider,\n\tstatus:\t\t\"ready\",\n}")
	return toolbar{
		modelName: modelName,
		provider:  provider,
		status:    "ready",
	}
}

// View renders the toolbar at the given width.
func (t toolbar) View(width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	left := fmt.Sprintf(" %s | %s | turns: %d | $%.4f",
		t.modelName, t.provider, t.turnCount, t.totalCost)

	right := fmt.Sprintf(" %s ", t.status)

	styledRight := statusActiveStyle.Render(right)

	gap := max(width-len(left)-len(right), 0)

	bar := left + strings.Repeat(" ", gap) + styledRight
	observe.GlobalTrace("return: statusBarStyle.Render(bar)")
	observe.GlobalTrace("return: statusBarStyle.Render(bar)")
	return statusBarStyle.Render(bar)
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

// IncrementTurn adds one to the turn counter.
func (t *toolbar) IncrementTurn() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	t.turnCount++
}
