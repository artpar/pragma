package tui

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/interactive"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/query"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// siblingElapsedRe matches the spinner line with a live elapsed suffix
// (any value — the exact formatting is pinned by the TUI-003 gate; this
// gate pins survival across partial results, robust to sub-second
// stamp jitter between the two sibling ToolCallEvents).
var siblingElapsedRe = regexp.MustCompile(`Bash\.\.\. \d+[sm]`)

// TUI-005 gate: with parallel sibling calls (PAR-001), the first
// result must not kill the running-tool spinner and its elapsed time
// while the second call still executes. Baseline: the spinner died on
 // every result.
func TestSiblingSpinnerSurvivesPartialResults(t *testing.T) {
	m := newTestModel()
	m.streaming = true
	mResized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = mResized.(Model)
	step := func(msg tea.Msg) {
		next, _ := m.Update(msg)
		m, _ = next.(Model)
	}

	inA, _ := json.Marshal(map[string]string{"cmd": "sleep 2; echo A_DONE"})
	inB, _ := json.Marshal(map[string]string{"cmd": "sleep 3; echo B_DONE"})
	step(LoopEventMsg{Event: interactive.LoopEvent{Event: query.ToolCallEvent{
		Call: model.ToolCallPart{ID: "sib-a", Name: "Bash", Input: inA},
	}}})
	step(LoopEventMsg{Event: interactive.LoopEvent{Event: query.ToolCallEvent{
		Call: model.ToolCallPart{ID: "sib-b", Name: "Bash", Input: inB},
	}}})
	step(spinner.TickMsg{Time: time.Now().Add(45 * time.Second)})

	// First sibling completes while the second still runs.
	step(LoopEventMsg{Event: interactive.LoopEvent{Event: query.ToolResultEvent{
		Result: model.ToolResultPart{ToolCallID: "sib-a", Content: "A_DONE"},
	}}})
	step(spinner.TickMsg{Time: time.Now().Add(60 * time.Second)})

	plain := ansiCodes.ReplaceAllString(m.View(), "")
	if !strings.Contains(plain, "Bash...") {
		t.Fatalf("spinner died on the first sibling result while a call still runs; view:\n%s", plain)
	}
	if !siblingElapsedRe.MatchString(plain) {
		t.Fatalf("elapsed time died with the spinner; view:\n%s", plain)
	}

	// Last sibling completes: the spinner and elapsed clear.
	step(LoopEventMsg{Event: interactive.LoopEvent{Event: query.ToolResultEvent{
		Result: model.ToolResultPart{ToolCallID: "sib-b", Content: "B_DONE"},
	}}})
	plain = ansiCodes.ReplaceAllString(m.View(), "")
	if strings.Contains(plain, "Bash...") {
		t.Fatalf("spinner lingers after all sibling results; view:\n%s", plain)
	}
}

// TUI-005 adjacent: with distinct sibling names, the completed call's
// name is replaced by the survivor's.
func TestSiblingSpinnerShowsRemainingName(t *testing.T) {
	m := newTestModel()
	m.streaming = true
	mResized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = mResized.(Model)
	step := func(msg tea.Msg) {
		next, _ := m.Update(msg)
		m, _ = next.(Model)
	}

	step(LoopEventMsg{Event: interactive.LoopEvent{Event: query.ToolCallEvent{
		Call: model.ToolCallPart{ID: "sib-x", Name: "Bash", Input: []byte(`{"cmd":"echo x"}`)},
	}}})
	step(LoopEventMsg{Event: interactive.LoopEvent{Event: query.ToolCallEvent{
		Call: model.ToolCallPart{ID: "sib-y", Name: "Grep", Input: []byte(`{"pattern":"live"}`)},
	}}})
	step(LoopEventMsg{Event: interactive.LoopEvent{Event: query.ToolResultEvent{
		Result: model.ToolResultPart{ToolCallID: "sib-x", Content: "x done"},
	}}})

	plain := ansiCodes.ReplaceAllString(m.View(), "")
	if !strings.Contains(plain, "Grep...") {
		t.Fatalf("surviving sibling's name not shown on the spinner; view:\n%s", plain)
	}
	if strings.Contains(plain, "Bash...") {
		t.Fatalf("completed call's name still shown; view:\n%s", plain)
	}
}
