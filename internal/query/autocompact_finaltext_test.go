package query

import (
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// CMP-001.3.F3 gates: the FinalTextOnly sub-loop of the pragma loop is
// the path orchestration final_text-capture states run
// (RunStateEvents builds PragmaLoopRunOptions{FinalTextOnly:
// stateCapturesFinalText(state), IncludePriorConversation:
// stateUsesPersistentConversation(state)}). The two predicates are
// independent, so a state can be persistent AND final_text-capture:
// its runs are UNSCOPED (IncludePriorConversation means no
// MessageStartIndexes) yet they take the FinalTextOnly sub-loop, which
// never consulted the auto-compact tracker — the CMP-001.3 record's
// "unscoped pragma-loop runs ... orchestration persistent-conversation
// states ... carry the trigger" overstated coverage for exactly this
// combination.

// TestPragmaLoopFinalTextOnlyUnscopedAutoCompactTriggersAndReplaces: an
// UNSCOPED FinalTextOnly run (the persistent-capture-state options
// shape) over a live-deps engine with a far-over-threshold conversation
// must compact before its next model request — the sub-loop shares the
// CMP-001.3 unscoped trigger — and that request must carry the summary
// plus the pending (unanswered) turn prompt verbatim, then complete
// normally through the final-text check.
func TestPragmaLoopFinalTextOnlyUnscopedAutoCompactTriggersAndReplaces(t *testing.T) {
	// Call 0: compaction summary (compact.Service calls provider.Complete)
	// — deliberately does not mention the marker, so the only way the
	// marker can reach the model is the preserved pending prompt.
	// Call 1: the accepted final verdict.
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{
			Content:    []model.ContentPart{model.TextPart{Text: "Summary: user asked questions, assistant answered."}},
			StopReason: model.StopEndTurn,
		},
		{
			Content:    []model.ContentPart{model.TextPart{Text: "VERDICT-7c31"}},
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
		LoopMode:  LoopModePragma,
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

	completionCheckCalled := false
	checkCalls := 0
	events := collectPragmaLoopEvents(engine.RunPragmaLoopWithSystemCompletionCheckOptions(
		t.Context(),
		model.SystemPrompt{},
		"RIVERMARK-9d21: return the verdict",
		func() (bool, string, error) {
			completionCheckCalled = true
			return false, "must not run in final-text-only mode", nil
		},
		// The exact RunStateEvents options shape for a state that is both
		// persistent-conversation AND final_text-capture: unscoped
		// (IncludePriorConversation) and on the FinalTextOnly sub-loop.
		PragmaLoopRunOptions{
			IncludePriorConversation: true,
			FinalTextOnly:            true,
			FinalTextCheck: func(text string) (bool, string, error) {
				checkCalls++
				return strings.TrimSpace(text) == "VERDICT-7c31", "", nil
			},
		},
	))

	var started, compacted, failed, disabled, complete int
	var preTokens, postTokens int
	for _, ev := range events {
		switch e := ev.(type) {
		case CompactionStartedEvent:
			started++
		case CompactionEvent:
			compacted++
			preTokens, postTokens = e.PreTokens, e.PostTokens
		case CompactionFailedEvent:
			failed++
		case CompactionDisabledEvent:
			disabled++
		case ErrorEvent:
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		case TurnCompleteEvent:
			complete++
		}
	}
	if started != 1 {
		t.Fatalf("CompactionStartedEvent count = %d, want 1 — the unscoped FinalTextOnly sub-loop (the orchestration persistent-capture-state shape) never consulted the auto-compact tracker even over threshold (CMP-001.3.F3)", started)
	}
	if compacted != 1 {
		t.Fatalf("CompactionEvent count = %d, want 1", compacted)
	}
	if failed != 0 || disabled != 0 {
		t.Fatalf("CompactionFailedEvent = %d, CompactionDisabledEvent = %d, want 0/0", failed, disabled)
	}
	if complete != 1 {
		t.Fatalf("TurnCompleteEvent count = %d, want 1 — the final-text flow must complete normally through the compaction", complete)
	}
	if postTokens >= preTokens {
		t.Fatalf("post tokens %d not below pre tokens %d", postTokens, preTokens)
	}
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2 (summary + final-text model request)", prov.calls)
	}
	// The post-compaction model request must carry the replacement
	// conversation (smaller than the 7-message original incl. the appended
	// prompt) with the summary AND the pending prompt verbatim.
	if len(prov.requests[1].Messages) >= len(conv.Messages)+1 {
		t.Fatalf("post-compaction request carries %d messages, want fewer than the %d-message original",
			len(prov.requests[1].Messages), len(conv.Messages)+1)
	}
	var sawSummary, sawPendingPrompt bool
	for _, msg := range prov.requests[1].Messages {
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok {
				if strings.Contains(tp.Text, "Summary: user asked questions") {
					sawSummary = true
				}
				if strings.Contains(tp.Text, "RIVERMARK-9d21") {
					sawPendingPrompt = true
				}
			}
		}
	}
	if !sawSummary {
		t.Fatalf("post-compaction request does not contain the compaction summary")
	}
	if !sawPendingPrompt {
		t.Fatalf("post-compaction request lost the pending prompt (RIVERMARK-9d21): the unanswered turn prompt was compacted away and never reached the model")
	}
	if completionCheckCalled {
		t.Fatal("completion check should not run for final-text-only mode")
	}
	if checkCalls != 1 {
		t.Fatalf("final text check calls = %d, want 1", checkCalls)
	}
}

// TestPragmaLoopFinalTextOnlyScopedSkipsCompaction: a SCOPED
// FinalTextOnly run (no IncludePriorConversation — the shape of a
// root-engine capture state) must NEVER compact, even with live deps
// over an over-threshold conversation: compaction replaces the WHOLE
// conversation and would invalidate the scope's start index (the
// load-bearing CMP-001.3 scope gate). The sub-loop port must extend
// the trigger, not the scope exception.
func TestPragmaLoopFinalTextOnlyScopedSkipsCompaction(t *testing.T) {
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{
			Content:    []model.ContentPart{model.TextPart{Text: "VERDICT-7c31"}},
			StopReason: model.StopEndTurn,
		},
	}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
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
		LoopMode:  LoopModePragma,
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

	events := collectPragmaLoopEvents(engine.RunPragmaLoopWithSystemCompletionCheckOptions(
		t.Context(),
		model.SystemPrompt{},
		"return the verdict",
		nil,
		PragmaLoopRunOptions{
			FinalTextOnly: true,
			FinalTextCheck: func(text string) (bool, string, error) {
				return strings.TrimSpace(text) == "VERDICT-7c31", "", nil
			},
		},
	))

	var started, compacted, failed, complete int
	for _, ev := range events {
		switch e := ev.(type) {
		case CompactionStartedEvent:
			started++
		case CompactionEvent:
			compacted++
		case CompactionFailedEvent:
			failed++
		case ErrorEvent:
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		case TurnCompleteEvent:
			complete++
		}
	}
	if started != 0 || compacted != 0 || failed != 0 {
		t.Fatalf("scoped FinalTextOnly run compacted: started = %d, compacted = %d, failed = %d, want all 0 — compaction would invalidate the scope's start index (CMP-001.3 scope gate)", started, compacted, failed)
	}
	if complete != 1 {
		t.Fatalf("TurnCompleteEvent count = %d, want 1", complete)
	}
	if prov.calls != 1 {
		t.Fatalf("provider calls = %d, want 1 (no compaction summary for a scoped run)", prov.calls)
	}
	var sawBig, sawPrompt bool
	for _, msg := range prov.requests[0].Messages {
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok {
				if strings.Contains(tp.Text, bigText) {
					sawBig = true
				}
				if strings.Contains(tp.Text, "return the verdict") {
					sawPrompt = true
				}
			}
		}
	}
	if sawBig {
		t.Fatal("scoped FinalTextOnly request carries the prior conversation (it must be sliced to the state's own segment)")
	}
	if !sawPrompt {
		t.Fatal("scoped FinalTextOnly request lost the turn prompt")
	}
}

// TestPragmaLoopFinalTextOnlyCooldownBlocksImmediateRetrigger pins the
// CMP-001.1 F4 single-increment semantics for the FinalTextOnly
// sub-loop port: the sub-loop keeps its ONE per-iteration IncrementTurn
// (after the assistant append) and the ported trigger adds none, so a
// successful compaction at iteration k is followed by exactly one
// increment inside iteration k — MinTurnsCooldown=2 still blocks the
// re-trigger at iteration k+1 even though the huge scripted summary
// leaves the conversation over threshold. A ported block carrying its
// own increment (the CMP-001.1 F4 defect class) would expire the
// cooldown inside iteration k and compact again at k+1.
func TestPragmaLoopFinalTextOnlyCooldownBlocksImmediateRetrigger(t *testing.T) {
	// Call 0: a deliberately huge summary (~1250 heuristic tokens — above
	// the 904 threshold, far below the ~22.5k original) so the
	// post-compaction conversation stays over threshold and ONLY the
	// cooldown can block the re-trigger.
	// Call 1: rejected draft. Call 2: accepted verdict.
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{
			Content:    []model.ContentPart{model.TextPart{Text: "Summary: " + strings.Repeat("word ", 1000)}},
			StopReason: model.StopEndTurn,
		},
		{
			Content:    []model.ContentPart{model.TextPart{Text: "DRAFT-1"}},
			StopReason: model.StopEndTurn,
		},
		{
			Content:    []model.ContentPart{model.TextPart{Text: "VERDICT-7c31"}},
			StopReason: model.StopEndTurn,
		},
	}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
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
		LoopMode:  LoopModePragma,
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

	events := collectPragmaLoopEvents(engine.RunPragmaLoopWithSystemCompletionCheckOptions(
		t.Context(),
		model.SystemPrompt{},
		"return the verdict",
		nil,
		PragmaLoopRunOptions{
			IncludePriorConversation: true,
			FinalTextOnly:            true,
			FinalTextCheck: func(text string) (bool, string, error) {
				return strings.TrimSpace(text) == "VERDICT-7c31", "", nil
			},
		},
	))

	var started, compacted, complete int
	for _, ev := range events {
		switch e := ev.(type) {
		case CompactionStartedEvent:
			started++
		case CompactionEvent:
			compacted++
		case ErrorEvent:
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		case TurnCompleteEvent:
			complete++
		}
	}
	if started != 1 || compacted != 1 {
		t.Fatalf("CompactionStartedEvent = %d, CompactionEvent = %d, want 1/1 (MinTurnsCooldown=2 must block the immediately following iteration — a double increment inside the compaction's own iteration is the CMP-001.1 F4 defect class)", started, compacted)
	}
	if complete != 1 {
		t.Fatalf("TurnCompleteEvent count = %d, want 1", complete)
	}
	if prov.calls != 3 {
		t.Fatalf("provider calls = %d, want 3 (summary + rejected draft + verdict)", prov.calls)
	}
}
