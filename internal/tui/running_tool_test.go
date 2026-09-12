package tui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/interactive"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/query"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// TUI-003 gate: a running tool call must show its elapsed time in the
// default view. Authentic shape: this session's 361-second silent Bash
// gap (18:57:27 → 19:03:28, log 2026-09-12T18-53-02.jsonl) rendered a
// bare animating spinner for six minutes — the operator's 19:02:27
// report: "the ui for human is quite bad and doesnt tell the human at
// all whats *really* going on."
func TestRunningToolShowsElapsedTime(t *testing.T) {
	m := newTestModel()
	m.streaming = true
	mResized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = mResized.(Model)

	// Verbatim: the session's real long-running Bash call.
	cmd := `go test ./internal/tui -count=1 2>&1 | tail -3; echo "==="; go build ./... 2>&1 | tail -5; echo "rc=$?"`
	input, err := json.Marshal(map[string]string{"command": cmd})
	if err != nil {
		t.Fatalf("marshal tool input: %v", err)
	}
	next, _ := m.Update(LoopEventMsg{Event: interactive.LoopEvent{Event: query.ToolCallEvent{
		Call: model.ToolCallPart{ID: "call-361s", Name: "Bash", Input: input},
	}}})
	m, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}
	if !m.spinnerActive {
		t.Fatal("ToolCallEvent must activate the running-tool spinner")
	}

	// The spinner's own tick carries the clock: 311s after the call began,
	// the default view must carry a human-readable elapsed (5m11s).
	ticked, _ := m.Update(spinner.TickMsg{Time: time.Now().Add(311 * time.Second)})
	m, ok = ticked.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", ticked)
	}

	plain := ansiCodes.ReplaceAllString(m.View(), "")
	if !strings.Contains(plain, "Bash...") {
		t.Fatalf("running-tool spinner line missing; view:\n%s", plain)
	}
	if !strings.Contains(plain, "5m11s") {
		t.Fatalf("running tool showed no elapsed time after 311s; view:\n%s", plain)
	}
}

// TUI-003 adjacent: the elapsed (and the spinner line) disappear when
// the tool result arrives — nothing lingers into the completed state.
func TestSpinnerElapsedClearsAfterResult(t *testing.T) {
	m := newTestModel()
	m.streaming = true
	mResized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = mResized.(Model)
	step := func(msg tea.Msg) {
		next, _ := m.Update(msg)
		m, _ = next.(Model)
	}

	input, _ := json.Marshal(map[string]string{"command": "sleep 361"})
	step(LoopEventMsg{Event: interactive.LoopEvent{Event: query.ToolCallEvent{
		Call: model.ToolCallPart{ID: "call-1", Name: "Bash", Input: input},
	}}})
	step(spinner.TickMsg{Time: time.Now().Add(311 * time.Second)})
	step(LoopEventMsg{Event: interactive.LoopEvent{Event: query.ToolResultEvent{
		Result: model.ToolResultPart{ToolCallID: "call-1", Content: "done"},
	}}})

	plain := ansiCodes.ReplaceAllString(m.View(), "")
	if strings.Contains(plain, "Bash...") {
		t.Fatalf("spinner line lingers after tool result; view:\n%s", plain)
	}
	if strings.Contains(plain, "5m11s") {
		t.Fatalf("elapsed lingers after tool result; view:\n%s", plain)
	}
}

// TUI-003 adjacent: an idle model renders no spinner line and no elapsed.
func TestNoSpinnerWhenIdle(t *testing.T) {
	m := newTestModel()
	mResized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = mResized.(Model)

	plain := ansiCodes.ReplaceAllString(m.View(), "")
	if strings.Contains(plain, "... 0s") || strings.Contains(plain, "... 1s") {
		t.Fatalf("spinner/elapsed rendered while idle; view:\n%s", plain)
	}
}
