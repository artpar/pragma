package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/interactive"
	tea "github.com/charmbracelet/bubbletea"
)

// INT-001 render gate: a queued prompt renders as the operator's user
// message with the queued-behind indicator, while the turn keeps streaming.
func TestQueuedPromptEventRenders(t *testing.T) {
	m := newTestModel()
	m.streaming = true
	// The viewport needs dimensions before View renders content.
	mResized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = mResized.(Model)

	m2, _ := m.Update(LoopEventMsg{Event: interactive.QueuedPromptEvent{Prompt: "mid-turn note"}})
	model2, ok := m2.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", m2)
	}

	out := model2.View()
	// The markdown renderer styles spans inside the prompt text, so strip
	// SGR sequences before substring assertions.
	plain := ansiCodes.ReplaceAllString(out, "")
	if !strings.Contains(plain, "mid-turn note") {
		t.Fatalf("queued prompt not rendered; view:\n%s", out)
	}
	if !strings.Contains(plain, "queued behind the running turn") {
		t.Fatalf("queued indicator not rendered; view:\n%s", out)
	}
	if !model2.streaming {
		t.Fatal("queued prompt must not end the streaming turn")
	}
}

// ansiCodes strips SGR escape sequences so rendered-text assertions match
// across styling spans.
var ansiCodes = regexp.MustCompile(`\x1b\[[0-9;]*m`)
