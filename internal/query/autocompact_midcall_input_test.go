package query

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

// CMP-001.2.F1 gates: operator input delivered while the compaction
// summary call is in flight (between the loop's pre-compaction snapshot
// and the wholesale replacement) must survive the replacement — from the
// store and from the model request. The busy-turn CLI path delivers
// exactly this shape (InteractiveRuntime.RunInput -> Engine.
// AppendUserInput while a turn is running), and during the summary call
// the conversation tail is the unanswered prompt (not a dangling
// tool_use), so the input appends directly to the store inside the
// window. The CMP-001.2 F3 fix re-appended pending prompts derived only
// from the stale pre-compaction snapshot, destroying such mid-call input.

// gatedSummaryProvider behaves like pragmaLoopTestProvider except its
// first Complete call — the compaction summary — parks until the test
// releases it, after signalling that it started. That park is exactly
// the in-flight-summary window: the loop goroutine has taken its
// pre-compaction snapshot (compSnap) and is inside Compact, before the
// replacement Update.
type gatedSummaryProvider struct {
	mu        sync.Mutex
	responses []model.Response
	calls     int
	requests  []provider.RequestParams

	summaryStarted chan struct{} // closed once, when the summary call begins
	release        chan struct{} // the test closes it to let the summary return
}

func (p *gatedSummaryProvider) Name() string { return "test" }

func (p *gatedSummaryProvider) Stream(context.Context, provider.RequestParams) (<-chan provider.StreamChunk, error) {
	return nil, fmt.Errorf("Stream is not used by this test provider")
}

func (p *gatedSummaryProvider) Complete(_ context.Context, params provider.RequestParams) (model.Response, error) {
	p.mu.Lock()
	idx := p.calls
	if idx >= len(p.responses) {
		p.mu.Unlock()
		return model.Response{}, fmt.Errorf("no response configured for call %d", idx+1)
	}
	p.requests = append(p.requests, params)
	p.calls++
	p.mu.Unlock()
	if idx == 0 {
		close(p.summaryStarted)
		<-p.release
	}
	return p.responses[idx], nil
}

func (p *gatedSummaryProvider) SupportsFeature(provider.Feature) bool { return true }
func (p *gatedSummaryProvider) Pricing(string) (model.Pricing, bool)  { return model.Pricing{}, false }
func (p *gatedSummaryProvider) ContextWindow(string) (int, bool)      { return 200_000, true }

func (p *gatedSummaryProvider) snapshotRequests() []provider.RequestParams {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.RequestParams{}, p.requests...)
}

// newMidCallEngine builds an engine whose seeded conversation sits far
// above the 904-token compaction threshold, mirroring the CMP-001
// autocompact gates: the loop's turn prompt is the only unanswered
// message, so the trigger fires at iteration 0 before any response.
func newMidCallEngine(t *testing.T, prov provider.Provider, loopMode string) (*Engine, *app.StateStore) {
	t.Helper()
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	// ~15000 chars = ~3750 heuristic tokens per message; six messages put
	// the conversation far above the 904-token threshold configured below.
	bigText := strings.Repeat("word ", 3000)
	conv.Messages = []model.Message{
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
	}
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          t.TempDir(),
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(64), EngineConfig{
		Model:     "test-model",
		LoopMode:  loopMode,
		MaxTokens: 4096,
	})
	// EffectiveWindow = 20000 - 4096 - 2000 = 13904; threshold = 904.
	engine.SetCompaction(CompactionDeps{
		Compactor:   compact.NewService(prov, observe.NewEventBus(64), model.NewCostTracker(0), "test-model"),
		AutoTracker: compact.NewAutoTracker(false),
		WindowConfig: compact.WindowConfig{
			ContextWindow:   20_000,
			MaxOutput:       4096,
			SystemPromptEst: 2000,
		},
	})
	return engine, store
}

// runMidCallProbe starts a compaction turn, waits until the loop is
// parked inside the gated summary call, delivers operator input through
// the same entry point the busy-turn CLI path uses (Engine.
// AppendUserInput), then releases the summary. It returns the loop
// events.
func runMidCallProbe(t *testing.T, prov *gatedSummaryProvider, engine *Engine, turnPrompt, midCallInput, midCallMarker string) []LoopEvent {
	t.Helper()
	eventsCh := engine.Run(t.Context(), turnPrompt)
	done := make(chan []LoopEvent, 1)
	go func() { done <- collectPragmaLoopEvents(eventsCh) }()

	<-prov.summaryStarted

	// Deliver the operator input while the summary call is in flight.
	// AppendUserInput must append it directly (the tail is the unanswered
	// turn prompt, not a dangling tool_use) — verify that precondition so
	// a later loss can only be the replacement's doing.
	if err := engine.AppendUserInput(midCallInput); err != nil {
		t.Fatalf("AppendUserInput: %v", err)
	}
	// Precondition pin: the input must be in the store NOW, while the
	// summary is still parked — the direct-append path (the tail is the
	// unanswered prompt, not a dangling tool_use). This proves the input
	// reached the store before the replacement ran, so any later loss
	// can only be the replacement's doing.
	snapNow := engine.store.Snapshot()
	if !conversationTextContains(snapNow.Conversation.Messages, midCallMarker) {
		t.Fatalf("mid-call input (%s) was not in the store while the summary call was parked — the direct-append precondition did not hold", midCallMarker)
	}
	close(prov.release)

	return <-done
}

func conversationTextContains(messages []model.Message, marker string) bool {
	for _, msg := range messages {
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok && strings.Contains(tp.Text, marker) {
				return true
			}
		}
	}
	return false
}

func assertMidCallPreservation(t *testing.T, prov *gatedSummaryProvider, engine *Engine, events []LoopEvent, turnPromptMarker, midCallMarker string) {
	t.Helper()
	var started, complete int
	for _, ev := range events {
		switch ev.(type) {
		case CompactionStartedEvent:
			started++
		case TurnCompleteEvent:
			complete++
		case ErrorEvent:
			t.Fatalf("unexpected ErrorEvent: %v", ev.(ErrorEvent).Err)
		}
	}
	if started != 1 {
		t.Fatalf("CompactionStartedEvent count = %d, want 1", started)
	}
	if complete != 1 {
		t.Fatalf("TurnCompleteEvent count = %d, want 1", complete)
	}
	requests := prov.snapshotRequests()
	if len(requests) != 2 {
		t.Fatalf("provider calls = %d, want 2 (summary + model request)", len(requests))
	}
	// Leg 1 — the store: the mid-call input was in the store when the
	// summary returned; the compaction replacement must not destroy it.
	snap := engine.store.Snapshot()
	if !conversationTextContains(snap.Conversation.Messages, midCallMarker) {
		t.Fatalf("mid-turn operator input (%s) was destroyed from the store by the compaction replacement (CMP-001.2.F1)", midCallMarker)
	}
	if !conversationTextContains(snap.Conversation.Messages, turnPromptMarker) {
		t.Fatalf("the pending turn prompt (%s) was lost from the store", turnPromptMarker)
	}
	// Leg 2 — the model request: the post-compaction request must carry
	// the mid-call input verbatim alongside the summary and the pending
	// turn prompt.
	var sawSummary, sawPrompt, sawMidCall bool
	for _, msg := range requests[1].Messages {
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok {
				if strings.Contains(tp.Text, "Summary: routine question and answer exchanges") {
					sawSummary = true
				}
				if strings.Contains(tp.Text, turnPromptMarker) {
					sawPrompt = true
				}
				if strings.Contains(tp.Text, midCallMarker) {
					sawMidCall = true
				}
			}
		}
	}
	if !sawSummary {
		t.Fatalf("post-compaction request does not contain the compaction summary")
	}
	if !sawPrompt {
		t.Fatalf("post-compaction request lost the pending prompt (%s)", turnPromptMarker)
	}
	if !sawMidCall {
		t.Fatalf("post-compaction request lost the mid-summary operator input (%s): it was destroyed from the store by the compaction replacement and never reached the model (CMP-001.2.F1)", midCallMarker)
	}
}

func TestProviderToolsLoopAutoCompactPreservesMidCallOperatorInput(t *testing.T) {
	prov := &gatedSummaryProvider{
		responses: []model.Response{
			// Call 1 (summary): deliberately does NOT mention either
			// marker, so the only way they can reach the model is the
			// preserved pending messages themselves.
			{
				Content:    []model.ContentPart{model.TextPart{Text: "Summary: routine question and answer exchanges."}},
				StopReason: model.StopEndTurn,
			},
			// Call 2: end-turn response over the compacted conversation.
			{
				Content:    []model.ContentPart{model.TextPart{Text: "Checklist delivered."}},
				StopReason: model.StopEndTurn,
			},
		},
		summaryStarted: make(chan struct{}),
		release:        make(chan struct{}),
	}
	engine, _ := newMidCallEngine(t, prov, LoopModeProviderTools)
	events := runMidCallProbe(t, prov, engine,
		"RIVERGATE-9c21: what is the deploy checklist status?",
		"MIDGATE-9c21: also check the staging queue before deciding",
		"MIDGATE-9c21")
	assertMidCallPreservation(t, prov, engine, events,
		"RIVERGATE-9c21", "MIDGATE-9c21")
}

func TestPragmaLoopAutoCompactPreservesMidCallOperatorInput(t *testing.T) {
	prov := &gatedSummaryProvider{
		responses: []model.Response{
			{
				Content:    []model.ContentPart{model.TextPart{Text: "Summary: routine question and answer exchanges."}},
				StopReason: model.StopEndTurn,
			},
			// Call 2: plain text with no bash block ends a pragma-loop turn.
			{
				Content:    []model.ContentPart{model.TextPart{Text: "OK, noted."}},
				StopReason: model.StopEndTurn,
			},
		},
		summaryStarted: make(chan struct{}),
		release:        make(chan struct{}),
	}
	engine, _ := newMidCallEngine(t, prov, LoopModePragma)
	events := runMidCallProbe(t, prov, engine,
		"RIVERMARK-9c21: finish the task",
		"MIDGATE-9c22: also include the staging queue in the report",
		"MIDGATE-9c22")
	assertMidCallPreservation(t, prov, engine, events,
		"RIVERMARK-9c21", "MIDGATE-9c22")
}
