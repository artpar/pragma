package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/interactive"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/query"
)

// INT-001 gates: plain-text input submitted while a turn is running is
// delivered into the running conversation (queued for the next request
// boundary) instead of being rejected-and-dropped — the night-session
// defect where the only delivery path was an interrupt that forfeited the
// in-flight request's input spend (~350K tokens, 2026-09-11/12). The gate
// compiles on the pre-change baseline and fails at runtime: the baseline
// rejects the prompt (RejectedPromptEvent) and the text never reaches the
// conversation.

type queueTestProvider struct{}

func (p *queueTestProvider) Name() string { return "test" }

func (p *queueTestProvider) Stream(context.Context, provider.RequestParams) (<-chan provider.StreamChunk, error) {
	return nil, fmt.Errorf("Stream is not used by this test provider")
}

func (p *queueTestProvider) Complete(context.Context, provider.RequestParams) (model.Response, error) {
	return model.Response{}, fmt.Errorf("no request expected while a turn is active")
}

func (p *queueTestProvider) SupportsFeature(provider.Feature) bool { return true }

func (p *queueTestProvider) Pricing(string) (model.Pricing, bool) { return model.Pricing{}, false }

func (p *queueTestProvider) ContextWindow(string) (int, bool) { return 200_000, true }

func newQueueTestRuntime(t *testing.T) (*InteractiveRuntime, *app.StateStore) {
	t.Helper()
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          t.TempDir(),
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := query.NewEngine(&queueTestProvider{}, store, model.NewCostTracker(0), observe.NewEventBus(64), query.EngineConfig{
		Model:     "test-model",
		LoopMode:  query.LoopModeProviderTools,
		MaxTokens: 4096,
		MaxTurns:  5,
	})
	rt := &InteractiveRuntime{
		Deps:   &Deps{Store: store},
		Engine: engine,
	}
	return rt, store
}

func collectInteractiveEvents(ch <-chan interactive.Event) []interactive.Event {
	var events []interactive.Event
	for ev := range ch {
		events = append(events, ev)
	}
	return events
}

func conversationContainsText(store *app.StateStore, needle string) bool {
	snap := store.Snapshot()
	for _, msg := range snap.Conversation.Messages {
		if msg.Role != model.RoleUser {
			continue
		}
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok && strings.Contains(tp.Text, needle) {
				return true
			}
		}
	}
	return false
}

func TestRunInputQueuesPlainTextWhileTurnActive(t *testing.T) {
	rt, store := newQueueTestRuntime(t)
	rt.turnActive = true // a turn is running (request in flight)

	events := collectInteractiveEvents(rt.RunInput(t.Context(), "first queued note"))

	for _, ev := range events {
		if _, rejected := ev.(interactive.RejectedPromptEvent); rejected {
			t.Fatalf("plain-text input while a turn is active was rejected (%v); it must be queued into the conversation", events)
		}
	}
	if !conversationContainsText(store, "first queued note") {
		t.Fatalf("queued input never reached the conversation; events: %v", events)
	}

	// A second mid-turn input must also queue — admission (one accepted
	// turn at a time) is unchanged; the messages stack in call order.
	events2 := collectInteractiveEvents(rt.RunInput(t.Context(), "second queued note"))
	for _, ev := range events2 {
		if _, rejected := ev.(interactive.RejectedPromptEvent); rejected {
			t.Fatalf("second plain-text input while a turn is active was rejected (%v); it must queue like the first", events2)
		}
	}
	if !conversationContainsText(store, "second queued note") {
		t.Fatalf("second queued input never reached the conversation; events: %v", events2)
	}
}

func TestRunInputStillRejectsSlashWhileTurnActive(t *testing.T) {
	rt, _ := newQueueTestRuntime(t)
	rt.turnActive = true

	events := collectInteractiveEvents(rt.RunInput(t.Context(), "/model other-model"))

	var rejected bool
	for _, ev := range events {
		if _, ok := ev.(interactive.RejectedPromptEvent); ok {
			rejected = true
		}
	}
	if !rejected {
		t.Fatalf("slash command while a turn is active must stay rejected (runtime side effects cannot run mid-turn); events: %v", events)
	}
}
