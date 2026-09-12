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

// Scratch review probe (overlay-injected, not a repo file): the uncovered
// adversarial case — a queued user prompt lands between two thinking
// segments of the same final response.
func TestScratchQueuedMidThinking(t *testing.T) {
	m := newTestModel()
	m.streaming = true
	mResized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = mResized.(Model)
	step := func(ev interactive.Event) {
		next, _ := m.Update(LoopEventMsg{Event: ev})
		m, _ = next.(Model)
	}
	step(interactive.LoopEvent{Event: query.ThinkingEvent{Text: "EARLIERSAME thinking A: internal plan"}})
	step(interactive.QueuedPromptEvent{Prompt: "mid-turn note"})
	step(interactive.LoopEvent{Event: query.ThinkingEvent{Text: "TRAILING thinking B: open a FRESH session"}})
	step(interactive.LoopEvent{Event: query.TurnCompleteEvent{StopReason: model.StopEndTurn}})

	plain := ansiCodes.ReplaceAllString(m.View(), "")
	t.Logf("VIEW:\n%s", plain)
	if !strings.Contains(plain, "FRESH session") {
		t.Errorf("trailing thinking B not promoted")
	}
	if !strings.Contains(plain, "mid-turn note") || !strings.Contains(plain, "queued behind the running turn") {
		t.Errorf("queued prompt text not visible")
	}
	if strings.Contains(plain, "EARLIERSAME thinking A") {
		t.Errorf("earlier same-response thinking A was promoted")
	}
	if !strings.Contains(plain, "ctrl+o to expand") {
		t.Errorf("collapsed thinking hint missing")
	}
}

// Scratch review probe: claim-2 boundary inputs through reloadConversationFromStore.
func TestScratchReloadBoundaries(t *testing.T) {
	build := func(msgs ...model.Message) Model {
		conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", "/tmp")
		for _, msg := range msgs {
			conv.Append(msg)
		}
		store := app.NewStateStore(app.AppState{
			Conversation: conv, CWD: "/tmp", Model: "test-model", Provider: "test",
		})
		m := newTestModel()
		m.store = store
		m.reloadConversationFromStore()
		return m
	}
	assistant := func(parts ...model.ContentPart) model.Message {
		return model.Message{ID: model.NewUUID(), Role: model.RoleAssistant, Content: parts}
	}

	// Boundary 1: last message is an assistant message with EMPTY content.
	mEmpty := build(assistant())
	plain := ansiCodes.ReplaceAllString(mEmpty.viewportContent(), "")
	t.Logf("EMPTY-CONTENT VIEW:\n%s", plain)

	// Boundary 2: last message is a USER message whose parts are all thinking.
	mUser := build(model.Message{
		ID:   model.NewUUID(),
		Role: model.RoleUser,
		Content: []model.ContentPart{
			model.ThinkingPart{Text: "USERROLE thinking must not be promoted"},
		},
	})
	plain = ansiCodes.ReplaceAllString(mUser.viewportContent(), "")
	t.Logf("USER-ROLE-LAST VIEW:\n%s", plain)
	if strings.Contains(plain, "USERROLE thinking must not be promoted") {
		t.Errorf("user-role thinking-only last message was promoted")
	}

	// Boundary 3: two consecutive thinking-only assistant messages — does
	// the scan-wide promoteTrailingThinking cross the whitespace-only
	// inter-message boundary?
	mTwo := build(
		assistant(model.ThinkingPart{Text: "FIRSTMSG thinking one"}),
		assistant(model.ThinkingPart{Text: "SECONDMSG thinking two"}),
	)
	plain = ansiCodes.ReplaceAllString(mTwo.viewportContent(), "")
	t.Logf("TWO-ASSISTANT VIEW:\n%s", plain)
	t.Logf("first promoted: %v", strings.Contains(plain, "FIRSTMSG thinking one"))
}
