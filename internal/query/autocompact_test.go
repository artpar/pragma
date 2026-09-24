package query

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

// CMP-001 gates: the provider-tools loop must consult the auto-compact
// tracker before each model request and replace the conversation with the
// compaction summary when the token count crosses the configured threshold.

func TestProviderToolsLoopAutoCompactTriggersAndReplaces(t *testing.T) {
	// Call 1: compaction summary (compact.Service calls provider.Complete).
	// Call 2: normal end-turn response over the compacted conversation.
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{
			Content:    []model.ContentPart{model.TextPart{Text: "Summary: user asked questions, assistant answered."}},
			StopReason: model.StopEndTurn,
		},
		{
			Content:    []model.ContentPart{model.TextPart{Text: "OK, noted."}},
			StopReason: model.StopEndTurn,
		},
	}}

	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	// ~15000 chars = ~3750 heuristic tokens per message; six messages put
	// the conversation far above the 904-token threshold configured below.
	bigText := strings.Repeat("word ", 3000)
	conv.Messages = []model.Message{
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "more questions"}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: "more answers"}}},
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
		LoopMode:  LoopModeProviderTools,
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

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "Tell me more"))

	var started, compacted, failed, complete int
	var postTokens, preTokens int
	for _, ev := range events {
		switch e := ev.(type) {
		case CompactionStartedEvent:
			started++
		case CompactionEvent:
			compacted++
			preTokens, postTokens = e.PreTokens, e.PostTokens
		case CompactionFailedEvent:
			failed++
		case TurnCompleteEvent:
			complete++
		}
	}
	if started != 1 {
		t.Fatalf("CompactionStartedEvent count = %d, want 1", started)
	}
	if compacted != 1 {
		t.Fatalf("CompactionEvent count = %d, want 1", compacted)
	}
	if failed != 0 {
		t.Fatalf("CompactionFailedEvent count = %d, want 0", failed)
	}
	if complete != 1 {
		t.Fatalf("TurnCompleteEvent count = %d, want 1", complete)
	}
	if postTokens >= preTokens {
		t.Fatalf("post tokens %d not below pre tokens %d", postTokens, preTokens)
	}
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2 (summary + response)", prov.calls)
	}
	// The post-compaction model request must carry the replacement
	// conversation (smaller than the original 7 messages, containing the
	// summary text) rather than the full history.
	if len(prov.requests[1].Messages) >= len(conv.Messages)+1 {
		t.Fatalf("post-compaction request carries %d messages, want fewer than the %d-message original",
			len(prov.requests[1].Messages), len(conv.Messages)+1)
	}
	var sawSummary bool
	for _, msg := range prov.requests[1].Messages {
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok && strings.Contains(tp.Text, "Summary: user asked questions") {
				sawSummary = true
			}
		}
	}
	if !sawSummary {
		t.Fatalf("post-compaction request does not contain the compaction summary")
	}
}

// failingCompactProvider passes model requests through to the scripted
// provider but fails every compaction (summary) request, to exercise the
// CMP-001 failure path and the circuit breaker.
type failingCompactProvider struct {
	pragmaLoopTestProvider
	compactCalls int
}

func (p *failingCompactProvider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	if len(params.Messages) > 0 && len(params.Messages[0].Content) > 0 {
		if tp, ok := params.Messages[0].Content[0].(model.TextPart); ok &&
			strings.Contains(tp.Text, "Here is the conversation to summarize") {
			p.compactCalls++
			return model.Response{}, errors.New("summary backend unavailable")
		}
	}
	return p.pragmaLoopTestProvider.Complete(ctx, params)
}

func TestProviderToolsLoopAutoCompactCircuitBreaker(t *testing.T) {
	callInput, err := json.Marshal(map[string]string{"cmd": "true"})
	if err != nil {
		t.Fatal(err)
	}
	toolUse := func() model.Response {
		return model.Response{
			Content:    []model.ContentPart{model.ToolCallPart{ID: model.NewUUID(), Name: "Bash", Input: callInput}},
			StopReason: model.StopToolUse,
		}
	}
	prov := &failingCompactProvider{}
	prov.responses = []model.Response{toolUse(), toolUse(), toolUse(), {
		Content:    []model.ContentPart{model.TextPart{Text: "done"}},
		StopReason: model.StopEndTurn,
	}}

	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
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
		LoopMode:  LoopModeProviderTools,
		MaxTokens: 4096,
	})
	engine.SetCompaction(CompactionDeps{
		Compactor:   compact.NewService(prov, observe.NewEventBus(64), model.NewCostTracker(0), "test-model"),
		AutoTracker: compact.NewAutoTracker(false),
		WindowConfig: compact.WindowConfig{
			ContextWindow:   20_000,
			MaxOutput:       4096,
			SystemPromptEst: 2000,
		},
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "work"))

	var failures, disabled, complete int
	var attempts []int
	for _, ev := range events {
		switch e := ev.(type) {
		case CompactionFailedEvent:
			failures++
			attempts = append(attempts, e.Attempt)
		case CompactionDisabledEvent:
			disabled++
		case TurnCompleteEvent:
			complete++
		case CompactionEvent:
			t.Fatalf("unexpected successful CompactionEvent despite failing backend")
		}
	}
	if prov.compactCalls != 3 {
		t.Fatalf("compaction attempts = %d, want 3 (breaker stops the 4th)", prov.compactCalls)
	}
	if failures != 3 {
		t.Fatalf("CompactionFailedEvent count = %d, want 3", failures)
	}
	if disabled != 1 {
		t.Fatalf("CompactionDisabledEvent count = %d, want 1", disabled)
	}
	if complete != 1 {
		t.Fatalf("TurnCompleteEvent count = %d, want 1", complete)
	}
	if len(attempts) != 3 || attempts[0] != 1 || attempts[1] != 2 || attempts[2] != 3 {
		t.Fatalf("failure attempts = %v, want [1 2 3]", attempts)
	}
	// The conversation must be untouched — failed compaction never replaces.
	if len(store.Snapshot().Conversation.Messages) <= len(conv.Messages) {
		// messages grew only by prompt/tools/results, but the big-text
		// originals must still be present
		var hasOriginal bool
		for _, msg := range store.Snapshot().Conversation.Messages {
			for _, part := range msg.Content {
				if tp, ok := part.(model.TextPart); ok && tp.Text == bigText {
					hasOriginal = true
				}
			}
		}
		if !hasOriginal {
			t.Fatalf("original messages lost after failed compactions")
		}
	}
}
