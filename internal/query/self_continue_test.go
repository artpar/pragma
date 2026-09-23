package query

import (
	"fmt"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

func newSelfContinueEngine(t *testing.T, prov *pragmaLoopTestProvider, maxTurns int) *Engine {
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          t.TempDir(),
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    512,
	})
	return NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(64), EngineConfig{
		Model:     "test-model",
		LoopMode:  LoopModeProviderTools,
		MaxTokens: 512,
		MaxTurns:  maxTurns,
	})
}

func textResponse(text string, outputTokens int) model.Response {
	return model.Response{
		ID:      model.NewUUID(),
		Model:   "test-model",
		Content: []model.ContentPart{model.TextPart{Text: text}},
		Usage:   model.TokenUsage{InputTokens: 10, OutputTokens: outputTokens},
	}
}

// HMB-003 gate: a terminal text response ending with the marker continues
// the turn with a self-continue notice; a later text without the marker
// ends the turn normally.
func TestSelfContinueMarkerKeepsTurnAlive(t *testing.T) {
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		textResponse("partial report, queue remains\n"+selfContinueMarker, 10),
		textResponse("done, queue empty", 10),
	}}
	engine := newSelfContinueEngine(t, prov, 10)
	events := collectPragmaLoopEvents(engine.Run(t.Context(), "work"))
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", prov.calls)
	}
	var noticeText string
	for _, msg := range prov.requests[1].Messages {
		if msg.Role != model.RoleUser {
			continue
		}
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok && strings.Contains(tp.Text, selfContinueMarker) {
				noticeText = tp.Text
			}
		}
	}
	if noticeText == "" {
		t.Fatal("second request carries no self-continue notice")
	}
	if !strings.Contains(noticeText, "self-continue 1") {
		t.Fatalf("notice missing count: %q", noticeText)
	}
	ended := false
	for _, ev := range events {
		if _, ok := ev.(TurnCompleteEvent); ok {
			ended = true
		}
	}
	if !ended {
		t.Fatal("turn never completed normally")
	}
}

// Regression: terminal text WITHOUT the marker ends the turn immediately
// (the pre-HMB-003 behavior is preserved byte-for-byte).
func TestTextWithoutMarkerEndsTurn(t *testing.T) {
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		textResponse("a normal final answer", 10),
		textResponse("never reached", 10),
	}}
	engine := newSelfContinueEngine(t, prov, 10)
	collectPragmaLoopEvents(engine.Run(t.Context(), "work"))
	if prov.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", prov.calls)
	}
}

// The cap bounds runaway self-continuation: a model that always marks
// continues at most maxSelfContinues times, then the turn ends.
func TestSelfContinueCap(t *testing.T) {
	responses := make([]model.Response, 0, 20)
	for i := 0; i < 20; i++ {
		responses = append(responses, textResponse(fmt.Sprintf("still working %d\n"+selfContinueMarker, i), 10))
	}
	prov := &pragmaLoopTestProvider{responses: responses}
	engine := newSelfContinueEngine(t, prov, 0) // uncapped turns
	events := collectPragmaLoopEvents(engine.Run(t.Context(), "work"))
	if prov.calls != maxSelfContinues+1 {
		t.Fatalf("provider calls = %d, want %d", prov.calls, maxSelfContinues+1)
	}
	ended := false
	for _, ev := range events {
		if _, ok := ev.(TurnCompleteEvent); ok {
			ended = true
		}
	}
	if !ended {
		t.Fatal("turn never completed after cap")
	}
}
