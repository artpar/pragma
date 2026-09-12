package tui

import (
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/interactive"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/query"
	tea "github.com/charmbracelet/bubbletea"
)

// TUI-002 gate: a paused stop reason (pause_turn — a reachable terminal
// stop from the provider tools loop, anthropic-mapped at
// translate_in.go:118-120) has no TUI notice branch, and a thinking-only
// paused response renders only the collapsed hint: the TUI-001 delivery
// class must cover the pause stop. The paused response's thinking is its
// only operator-facing content and must render by default, with a notice
// stating the turn is paused and how to continue.
func TestThinkingOnlyPauseTurnShowsInstruction(t *testing.T) {
	m := newTestModel()
	m.streaming = true
	// The viewport needs dimensions before View renders content.
	mResized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = mResized.(Model)

	// Same-response shape as TUI-001's authentic closing block, delivered
	// under the pause stop instead of end_turn.
	const instruction = `Paused mid-turn. If you want to resume, send any message — say "continue the paused turn".`

	m2, _ := m.Update(LoopEventMsg{Event: interactive.LoopEvent{Event: query.ThinkingEvent{Text: instruction}}})
	m3, _ := m2.Update(LoopEventMsg{Event: interactive.LoopEvent{Event: query.TurnCompleteEvent{StopReason: model.StopPauseTurn}}})
	final, ok := m3.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", m3)
	}
	if final.streaming {
		t.Fatal("TurnComplete must end the streaming turn")
	}

	plain := ansiCodes.ReplaceAllString(final.View(), "")
	if !strings.Contains(plain, "resume, send any message") {
		t.Fatalf("thinking-only pause_turn rendered no operator-readable output; view:\n%s", final.View())
	}
	if !strings.Contains(plain, "turn paused") {
		t.Fatalf("pause_turn rendered no pause notice; view:\n%s", final.View())
	}
	if strings.Index(plain, "resume, send any message") > strings.Index(plain, "turn paused") {
		t.Fatalf("pause notice rendered before the response's own content; view:\n%s", final.View())
	}
}

// TUI-002 adjacent: a text-bearing paused response shows the pause notice
// (the turn did not complete normally — the operator must know), while its
// thinking stays collapsed in the default view: the promotion stays scoped
// to textless responses.
func TestPausedTextBearingTurnShowsNotice(t *testing.T) {
	m := newTestModel()
	m.streaming = true
	mResized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = mResized.(Model)
	step := func(ev query.LoopEvent) {
		next, _ := m.Update(LoopEventMsg{Event: interactive.LoopEvent{Event: ev}})
		m, _ = next.(Model)
	}

	step(query.ThinkingEvent{Text: "paused-response reasoning that must stay hidden by default"})
	step(query.TextEvent{Text: "Partial answer before the provider paused."})
	step(query.TurnCompleteEvent{StopReason: model.StopPauseTurn})

	plain := ansiCodes.ReplaceAllString(m.View(), "")
	if !strings.Contains(plain, "Partial answer before the provider paused.") {
		t.Fatalf("response text not rendered; view:\n%s", plain)
	}
	if !strings.Contains(plain, "turn paused") {
		t.Fatalf("text-bearing pause_turn rendered no pause notice; view:\n%s", plain)
	}
	if strings.Contains(plain, "paused-response reasoning that must stay hidden") {
		t.Fatalf("thinking rendered expanded for a text-bearing paused response; view:\n%s", plain)
	}
}
