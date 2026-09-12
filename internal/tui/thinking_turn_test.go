package tui

import (
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/interactive"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/query"
	tea "github.com/charmbracelet/bubbletea"
)

// TUI-001 gate: a final response whose entire content is one thinking block
// and an empty text body must render its thinking in the default view. The
// authentic shape is the 2026-09-12T12-27-56 session's closing request
// (15:37:39, stop end_turn): the operator's follow-up instruction lived only
// in the thinking block and was never delivered to a default-mode operator.
func TestThinkingOnlyEndTurnShowsInstruction(t *testing.T) {
	m := newTestModel()
	m.streaming = true
	// The viewport needs dimensions before View renders content.
	mResized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = mResized.(Model)

	// Verbatim excerpt of the authentic closing thinking block.
	const instruction = `The numbers: 49 requests completed, latest request 171,946 input tokens, zero failures.

If you want a status check, open a FRESH session and say "continue" — it reads the roadmap and either picks up a fired trigger or reports "nothing new, holds stand." Cheap and clean.`

	m2, _ := m.Update(LoopEventMsg{Event: interactive.LoopEvent{Event: query.ThinkingEvent{Text: instruction}}})
	m3, _ := m2.Update(LoopEventMsg{Event: interactive.LoopEvent{Event: query.TurnCompleteEvent{StopReason: model.StopEndTurn}}})
	final, ok := m3.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", m3)
	}
	if final.streaming {
		t.Fatal("TurnComplete must end the streaming turn")
	}

	out := final.View()
	plain := ansiCodes.ReplaceAllString(out, "")
	if !strings.Contains(plain, "FRESH session") {
		t.Fatalf("thinking-only end_turn rendered no operator-readable output; view:\n%s", out)
	}
}

// TUI-001 adjacent: a final response that carries text keeps its thinking
// collapsed in the default view — the promotion is scoped to textless
// final responses only.
func TestThinkingThenTextStaysCollapsed(t *testing.T) {
	m := newTestModel()
	m.streaming = true
	mResized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = mResized.(Model)
	step := func(ev query.LoopEvent) {
		next, _ := m.Update(LoopEventMsg{Event: interactive.LoopEvent{Event: ev}})
		m, _ = next.(Model)
	}

	step(query.ThinkingEvent{Text: "internal reasoning that must stay hidden by default"})
	step(query.TextEvent{Text: "Final answer: ship it."})
	step(query.TurnCompleteEvent{StopReason: model.StopEndTurn})

	plain := ansiCodes.ReplaceAllString(m.View(), "")
	if !strings.Contains(plain, "Final answer: ship it.") {
		t.Fatalf("response text not rendered; view:\n%s", plain)
	}
	if strings.Contains(plain, "internal reasoning that must stay hidden") {
		t.Fatalf("thinking rendered expanded for a text-bearing response; view:\n%s", plain)
	}
	if !strings.Contains(plain, "ctrl+o to expand") {
		t.Fatalf("collapsed thinking hint missing; view:\n%s", plain)
	}
}

// TUI-001 adjacent: in a multi-request turn, text and tool output from
// earlier requests does not block promotion of a textless final response —
// only the trailing thinking run (the final response's) is promoted.
func TestTextlessFinalResponseAfterToolWorkIsPromoted(t *testing.T) {
	m := newTestModel()
	m.streaming = true
	mResized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = mResized.(Model)
	step := func(ev query.LoopEvent) {
		next, _ := m.Update(LoopEventMsg{Event: interactive.LoopEvent{Event: ev}})
		m, _ = next.(Model)
	}

	// Request 1: thinking, text, a Bash tool call and its result.
	step(query.ThinkingEvent{Text: "First I should inspect the file."})
	step(query.TextEvent{Text: "Inspecting the file before answering."})
	step(query.ToolCallEvent{Call: model.ToolCallPart{
		ID: "call-1", Name: "Bash", Input: []byte(`{"cmd":"cat a.go"}`),
	}})
	step(query.ToolResultEvent{Result: model.ToolResultPart{
		ToolCallID: "call-1", Content: "file contents",
	}})
	// Request 2 (final): thinking-only, end_turn — the TUI-001 shape.
	step(query.ThinkingEvent{Text: "If you want a status check, open a FRESH session and say \"continue\"."})
	step(query.TurnCompleteEvent{StopReason: model.StopEndTurn})

	plain := ansiCodes.ReplaceAllString(m.View(), "")
	if !strings.Contains(plain, "FRESH session") {
		t.Fatalf("textless final response not promoted after tool work; view:\n%s", plain)
	}
	if !strings.Contains(plain, "Inspecting the file before answering.") {
		t.Fatalf("earlier request text not rendered; view:\n%s", plain)
	}
	if strings.Contains(plain, "First I should inspect the file.") {
		t.Fatalf("earlier request thinking rendered expanded; view:\n%s", plain)
	}
}

// TUI-001 adjacent (resume path): reloading a conversation that ends with a
// thinking-only assistant message surfaces its thinking; a conversation
// ending in a normal text response keeps thinking collapsed.
func TestReloadShowsTrailingThinkingOnlyMessage(t *testing.T) {
	build := func(last model.Message) Model {
		conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", "/tmp")
		conv.Append(model.Message{
			ID:   model.NewUUID(),
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.TextPart{Text: "you tell me what to do"},
			},
		})
		conv.Append(last)
		store := app.NewStateStore(app.AppState{
			Conversation: conv, CWD: "/tmp", Model: "test-model", Provider: "test",
		})
		m := newTestModel()
		m.store = store
		m.reloadConversationFromStore()
		return m
	}

	thinkingOnly := build(model.Message{
		ID:   model.NewUUID(),
		Role: model.RoleAssistant,
		Content: []model.ContentPart{
			model.ThinkingPart{Text: "Open a FRESH session and say \"continue\""},
		},
	})
	plain := ansiCodes.ReplaceAllString(thinkingOnly.viewportContent(), "")
	if !strings.Contains(plain, "FRESH session") {
		t.Fatalf("trailing thinking-only message not promoted on reload; content:\n%s", plain)
	}

	textBearing := build(model.Message{
		ID:   model.NewUUID(),
		Role: model.RoleAssistant,
		Content: []model.ContentPart{
			model.ThinkingPart{Text: "reasoning hidden on reload"},
			model.TextPart{Text: "Here is the answer."},
		},
	})
	plain = ansiCodes.ReplaceAllString(textBearing.viewportContent(), "")
	if !strings.Contains(plain, "Here is the answer.") {
		t.Fatalf("text response not rendered on reload; content:\n%s", plain)
	}
	if strings.Contains(plain, "reasoning hidden on reload") {
		t.Fatalf("thinking rendered expanded for a text-bearing message on reload; content:\n%s", plain)
	}
}
