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

// TestProviderToolsLoopAutoCompactPreservesPendingPrompt guards the
// CMP-001.2 F3 defect: auto-compaction fires at the TOP of an iteration,
// after the operator's prompt was appended but before any response exists.
// The compaction replacement used to swallow that unanswered prompt — the
// model's next request carried only the summary, so the operator's prompt
// never reached the model verbatim (only whatever the summarizer happened
// to retain). The 2e9f01b pragma loop never compacted a pending prompt:
// its trigger ran after the assistant response was appended, so the
// conversation tail was always answered.
func TestProviderToolsLoopAutoCompactPreservesPendingPrompt(t *testing.T) {
	// Call 1: compaction summary — deliberately does NOT mention the
	// pending prompt, so the only way the marker can reach the model is
	// the preserved prompt message itself.
	// Call 2: end-turn response over the compacted conversation.
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{
			Content:    []model.ContentPart{model.TextPart{Text: "Summary: routine question and answer exchanges."}},
			StopReason: model.StopEndTurn,
		},
		{
			Content:    []model.ContentPart{model.TextPart{Text: "Checklist delivered."}},
			StopReason: model.StopEndTurn,
		},
	}}

	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	bigText := strings.Repeat("word ", 3000)
	// The conversation ends on an assistant message, so the only message
	// after the last assistant response is the prompt the loop appends at
	// turn start — the pending prompt that compaction must not swallow.
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

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "RIVERGATE-7f3a: what is the deploy checklist status?"))

	var started, complete int
	for _, ev := range events {
		switch ev.(type) {
		case CompactionStartedEvent:
			started++
		case TurnCompleteEvent:
			complete++
		}
	}
	if started != 1 {
		t.Fatalf("CompactionStartedEvent count = %d, want 1", started)
	}
	if complete != 1 {
		t.Fatalf("TurnCompleteEvent count = %d, want 1", complete)
	}
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2 (summary + response)", prov.calls)
	}
	// The post-compaction model request must carry the pending prompt
	// verbatim alongside the summary — not the summary alone.
	var sawSummary, sawPrompt bool
	for _, msg := range prov.requests[1].Messages {
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok {
				if strings.Contains(tp.Text, "routine question and answer exchanges") {
					sawSummary = true
				}
				if strings.Contains(tp.Text, "RIVERGATE-7f3a") {
					sawPrompt = true
				}
			}
		}
	}
	if !sawSummary {
		t.Fatalf("post-compaction request does not contain the compaction summary")
	}
	if !sawPrompt {
		t.Fatalf("post-compaction request lost the pending prompt (RIVERGATE-7f3a): the unanswered operator prompt was compacted away and never reached the model")
	}
	// The durable conversation must retain the pending prompt after the
	// summary as well, not just the single replacement summary message.
	var storeHasPrompt bool
	for _, msg := range store.Snapshot().Conversation.Messages {
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok && strings.Contains(tp.Text, "RIVERGATE-7f3a") {
				storeHasPrompt = true
			}
		}
	}
	if !storeHasPrompt {
		t.Fatalf("compacted conversation dropped the pending prompt from the conversation store")
	}
}

// TestProviderToolsLoopAutoCompactCooldownBlocksImmediateRetrigger guards
// the MinTurnsCooldown death-spiral cooldown (#24179): after a successful
// compaction, the immediately following loop iteration must NOT compact
// again even when the compacted conversation is still over the threshold,
// and the cooldown must expire once two model-request iterations have
// passed. The compaction summary is deliberately huge so the post-
// compaction conversation stays above the 904-token threshold — only the
// cooldown can block the re-trigger.
func TestProviderToolsLoopAutoCompactCooldownBlocksImmediateRetrigger(t *testing.T) {
	hugeSummary := strings.Repeat("word ", 3000)
	callInput, err := json.Marshal(map[string]string{"cmd": "true"})
	if err != nil {
		t.Fatal(err)
	}
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		// Run 1, call 1: compaction summary (huge — stays over threshold).
		{Content: []model.ContentPart{model.TextPart{Text: hugeSummary}}, StopReason: model.StopEndTurn},
		// Run 1, call 2: one tool-use iteration so the loop iterates while
		// the cooldown should be holding.
		{Content: []model.ContentPart{model.ToolCallPart{ID: model.NewUUID(), Name: "Bash", Input: callInput}}, StopReason: model.StopToolUse},
		// Run 1, call 3: end the turn.
		{Content: []model.ContentPart{model.TextPart{Text: "done"}}, StopReason: model.StopEndTurn},
		// Run 2, call 4: compaction summary for the second turn.
		{Content: []model.ContentPart{model.TextPart{Text: hugeSummary}}, StopReason: model.StopEndTurn},
		// Run 2, call 5: end the turn.
		{Content: []model.ContentPart{model.TextPart{Text: "done again"}}, StopReason: model.StopEndTurn},
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

	countStarted := func(events []LoopEvent) int {
		var started int
		for _, ev := range events {
			if _, ok := ev.(CompactionStartedEvent); ok {
				started++
			}
		}
		return started
	}

	// First run: one compaction, then one tool-use iteration, then
	// end-turn. The compaction must not re-trigger on the immediately
	// following iteration even though the huge summary keeps the
	// conversation over the threshold.
	events1 := collectPragmaLoopEvents(engine.Run(t.Context(), "first turn"))
	for _, ev := range events1 {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		}
	}
	if n := countStarted(events1); n != 1 {
		t.Fatalf("run 1 CompactionStartedEvent count = %d, want 1 (MinTurnsCooldown=2 must block the immediately following iteration)", n)
	}
	if prov.calls != 3 {
		t.Fatalf("provider calls after run 1 = %d, want 3 (summary + tool-use request + final request)", prov.calls)
	}

	// Second run: the cooldown must have expired across the end-turn
	// boundary — run 1 made two model-request iterations, so the still-
	// over-threshold conversation compacts again. If the only per-
	// iteration increment sat at the end of the tool-use path, an
	// end-turn-only session would stay cooldown-locked forever after
	// its first successful compaction.
	events2 := collectPragmaLoopEvents(engine.Run(t.Context(), "second turn"))
	for _, ev := range events2 {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		}
	}
	if n := countStarted(events2); n != 1 {
		t.Fatalf("run 2 CompactionStartedEvent count = %d, want 1 (cooldown must expire after two iterations)", n)
	}
	if prov.calls != 5 {
		t.Fatalf("provider calls after run 2 = %d, want 5 (two summaries + three model requests)", prov.calls)
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
	// The conversation must be untouched — failed compaction never
	// replaces the big-text originals. This check runs unconditionally:
	// the loop legitimately grows the conversation past the 6 originals
	// (prompt, assistant turns, tool results, stamps), so guarding it
	// behind a message-count comparison made the assertion unreachable.
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
