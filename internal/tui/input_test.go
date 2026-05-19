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

func TestInputComponentPromptHistoryRing(t *testing.T) {
	ic := newInputComponent()
	submitInput(t, &ic, "first prompt")
	submitInput(t, &ic, "second prompt")

	ic.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := ic.textarea.Value(); got != "second prompt" {
		t.Fatalf("first up = %q, want second prompt", got)
	}

	ic.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := ic.textarea.Value(); got != "first prompt" {
		t.Fatalf("second up = %q, want first prompt", got)
	}

	ic.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := ic.textarea.Value(); got != "first prompt" {
		t.Fatalf("up at oldest = %q, want first prompt", got)
	}

	ic.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := ic.textarea.Value(); got != "second prompt" {
		t.Fatalf("down = %q, want second prompt", got)
	}
}

func TestInputComponentPromptHistoryRestoresDraft(t *testing.T) {
	ic := newInputComponent()
	submitInput(t, &ic, "previous prompt")

	ic.textarea.SetValue("draft prompt")
	ic.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := ic.textarea.Value(); got != "previous prompt" {
		t.Fatalf("up = %q, want previous prompt", got)
	}

	ic.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := ic.textarea.Value(); got != "draft prompt" {
		t.Fatalf("down restored = %q, want draft prompt", got)
	}
}

func TestInputComponentPromptHistoryLimit(t *testing.T) {
	ic := newInputComponent()
	for i := 0; i < inputHistoryLimit+5; i++ {
		ic.remember(string(rune('a' + i%26)))
	}
	if len(ic.history) != inputHistoryLimit {
		t.Fatalf("history len = %d, want %d", len(ic.history), inputHistoryLimit)
	}
}

func submitInput(t *testing.T, ic *inputComponent, text string) {
	t.Helper()
	ic.textarea.SetValue(text)
	cmd := ic.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("submit %q returned nil command", text)
	}
	msg := cmd()
	submitted, ok := msg.(InputSubmittedMsg)
	if !ok {
		t.Fatalf("submit %q message type = %T, want InputSubmittedMsg", text, msg)
	}
	if submitted.Text != text {
		t.Fatalf("submitted text = %q, want %q", submitted.Text, text)
	}
}
