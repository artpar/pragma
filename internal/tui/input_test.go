package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInputComponentAlwaysActive(t *testing.T) {
	ic := newInputComponent()

	// Enter with no text should not produce a message
	cmd := ic.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("empty enter should not produce a command")
	}

	// View always renders the textarea (never disabled)
	view := ic.View()
	if view == "" {
		t.Error("view should not be empty")
	}
}

func TestInputComponentEmptyEnter(t *testing.T) {
	ic := newInputComponent()

	// Enter with no text should not produce a message
	cmd := ic.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("empty enter should not produce a command")
	}
}

func TestInputComponentReset(t *testing.T) {
	ic := newInputComponent()
	ic.Reset()
	// Should not panic, and value should be empty
	if ic.textarea.Value() != "" {
		t.Error("reset should clear textarea value")
	}
}
