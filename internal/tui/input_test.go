package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInputComponentActiveState(t *testing.T) {
	ic := newInputComponent()

	if !ic.active {
		t.Error("input should be active by default")
	}

	ic.SetActive(false)
	if ic.active {
		t.Error("input should be inactive after SetActive(false)")
	}

	// When inactive, Update should return nil cmd
	cmd := ic.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("inactive input should not produce commands")
	}

	// View should show waiting indicator
	view := ic.View()
	if !strings.Contains(view, "waiting") {
		t.Error("inactive view should show waiting indicator")
	}

	ic.SetActive(true)
	if !ic.active {
		t.Error("input should be active after SetActive(true)")
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
