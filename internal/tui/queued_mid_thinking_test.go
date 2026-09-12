package tui

import (
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/interactive"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/query"
	tea "github.com/charmbracelet/bubbletea"
)

// TUI-001 family, ORCH-002 finding 1 — the queued-mid-thinking shape,
// decided 2026-09-12: ThinkingEvent(A) → queued operator prompt →
// ThinkingEvent(B) → end_turn-without-text promotes exactly the trailing
// run (B). Thinking A is part of the same final response but stays
// collapsed: the queued prompt is the operator's own interjection,
// echoed in place — auto-expanding model thinking across the operator's
// words would interleave worse than the ctrl+o hint. This gate pins the
// decided behavior so a regression that silently drops the promotion or
// over-promotes across the queued text is caught. Gate body recorded in
// the ORCH-002 implementer report.
func TestQueuedMidThinkingPromotesTrailingRunOnly(t *testing.T) {
	m := newTestModel()
	m.streaming = true
	mResized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = mResized.(Model)
	step := func(msg tea.Msg) {
		next, _ := m.Update(msg)
		m, _ = next.(Model)
	}

	step(LoopEventMsg{Event: interactive.LoopEvent{Event: query.ThinkingEvent{Text: "PRE-QUEUE reasoning that stays behind the hint"}}})
	step(LoopEventMsg{Event: interactive.QueuedPromptEvent{Prompt: "mid-turn note from the operator"}})
	step(LoopEventMsg{Event: interactive.LoopEvent{Event: query.ThinkingEvent{Text: "POST-QUEUE closing instruction: open a FRESH session and say continue"}}})
	step(LoopEventMsg{Event: interactive.LoopEvent{Event: query.TurnCompleteEvent{StopReason: model.StopEndTurn}}})

	plain := ansiCodes.ReplaceAllString(m.View(), "")
	if !strings.Contains(plain, "POST-QUEUE closing instruction") {
		t.Fatalf("trailing thinking (post-queue) not promoted; view:\n%s", plain)
	}
	if strings.Contains(plain, "PRE-QUEUE reasoning") {
		t.Fatalf("pre-queue thinking promoted across the operator's queued prompt; view:\n%s", plain)
	}
	if !strings.Contains(plain, "ctrl+o to expand") {
		t.Fatalf("collapsed hint for the pre-queue thinking missing; view:\n%s", plain)
	}
	if !strings.Contains(plain, "mid-turn note from the operator") {
		t.Fatalf("queued prompt not echoed; view:\n%s", plain)
	}
	if !strings.Contains(plain, "queued behind the running turn") {
		t.Fatalf("queued indicator missing; view:\n%s", plain)
	}
}
