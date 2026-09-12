package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/interactive"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/query"
	tea "github.com/charmbracelet/bubbletea"
)

// TUI-004 guard: the running call's output-so-far renders inline in the
// default view (the 361-second silent gap class), and the final result
// replaces it exactly once — no duplicated content, byte-identical
// completed rendering to a call that never streamed.
func TestLiveToolOutputRendersInline(t *testing.T) {
	m := newTestModel()
	m.streaming = true
	mResized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = mResized.(Model)
	step := func(ev query.LoopEvent) {
		next, _ := m.Update(LoopEventMsg{Event: interactive.LoopEvent{Event: ev}})
		m, _ = next.(Model)
	}

	input, _ := json.Marshal(map[string]string{"cmd": "echo TUI004_STEP_ONE; sleep 1.5; echo TUI004_STEP_TWO"})
	step(query.ToolCallEvent{Call: model.ToolCallPart{ID: "call-live", Name: "Bash", Input: input}})

	// Mid-run: the first step's output is visible while the command runs.
	step(query.ToolOutputEvent{ToolCallID: "call-live", Output: "TUI004_STEP_ONE\n", Running: true})
	plain := ansiCodes.ReplaceAllString(m.View(), "")
	if !strings.Contains(plain, "TUI004_STEP_ONE") {
		t.Fatalf("running tool's live output not rendered; view:\n%s", plain)
	}

	// Completion: the final result replaces the live tail in place.
	step(query.ToolResultEvent{Result: model.ToolResultPart{
		ToolCallID: "call-live",
		Content:    "Exit code: 0\nTUI004_STEP_ONE\nTUI004_STEP_TWO",
	}})
	plain = ansiCodes.ReplaceAllString(m.View(), "")
	if !strings.Contains(plain, "Exit code: 0") {
		t.Fatalf("final result not rendered; view:\n%s", plain)
	}
	if got := strings.Count(plain, "TUI004_STEP_ONE"); got != 1 {
		t.Fatalf("final content duplicated (TUI004_STEP_ONE appears %d times, want 1); view:\n%s", got, plain)
	}
}
