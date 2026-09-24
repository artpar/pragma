package query

import (
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// CMP-001.3.F2 gate (critic C-2). The CMP-001.3 port moved the pragma
// loop's per-iteration IncrementTurn inside the unscoped-only trigger
// gate, so scoped activations — run.MessageStartIndexes set, the
// orchestration non-persistent state shape (RunStateEvents →
// RunPragmaLoopWithSystemCompletionCheckOptions with
// IncludePriorConversation=false) — advanced the cooldown ZERO turns per
// model-request iteration while the FinalTextOnly sub-loop of the SAME
// function still incremented unconditionally. That falsifies the
// documented invariant "IncrementTurn must still run every iteration ...
// the cooldown expires by counting model-request iterations"
// (internal/compact/auto.go) for scoped iterations: a scoped iteration
// still appends its turns to the engine's WHOLE conversation, so the
// shared cooldown must count it — exactly what the removed pre-port
// post-assistant site did (1b4fb1b^).
//
// The engine shape is the authentic one for a live-tracker scoped run:
// engineForOrchestrationState returns the ROOT engine (live compaction
// deps) for a persona-less control state, whose foreach_next persona
// handoff then runs the pragma loop on it (runner.go runNodeEvents
// → runPersonaForState → RunStateEvents).
//
// Boundary pinning: the scoped run itself must emit ZERO compaction
// events — the TRIGGER stays scope-gated (compaction would invalidate
// the scope's start index) — and the unscoped runs bracket the scoped one
// so the cooldown boundary isolates the scoped iterations' increments:
// run 1 compacts and ends one iteration later (cooldown 1/2); run 2 is
// the scoped run (two model-request iterations); run 3 must compact on
// its FIRST iteration because the scoped run's two increments expired
// the cooldown. Without scoped increments run 3's first iteration still
// sits inside the cooldown and re-compaction is delayed one unscoped
// iteration — the "safe direction" drift this gate pins.
func TestPragmaLoopScopedIterationsAdvanceSharedCooldown(t *testing.T) {
	hugeSummary := strings.Repeat("word ", 3000)
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		// Run 1 (unscoped), call 1: compaction summary — huge, so the
		// post-compaction conversation stays over the threshold.
		{Content: []model.ContentPart{model.TextPart{Text: hugeSummary}}, StopReason: model.StopEndTurn},
		// Run 1, call 2: final text — the one post-compaction iteration
		// leaves the cooldown at 1/2 turns when run 1 returns.
		{Content: []model.ContentPart{model.TextPart{Text: "done"}}, StopReason: model.StopEndTurn},
		// Run 2 (scoped), call 3: one bash iteration.
		{Content: []model.ContentPart{model.TextPart{Text: "```bash\ntrue\n```"}}, StopReason: model.StopEndTurn},
		// Run 2, call 4: final text.
		{Content: []model.ContentPart{model.TextPart{Text: "done again"}}, StopReason: model.StopEndTurn},
		// Run 3 (unscoped), call 5: second compaction summary.
		{Content: []model.ContentPart{model.TextPart{Text: hugeSummary}}, StopReason: model.StopEndTurn},
		// Run 3, call 6: final text.
		{Content: []model.ContentPart{model.TextPart{Text: "done thrice"}}, StopReason: model.StopEndTurn},
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
		Model:    "test-model",
		LoopMode: LoopModePragma,
		MaxTurns: 5,
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

	countEvents := func(events []LoopEvent) (started int, err error) {
		for _, ev := range events {
			switch e := ev.(type) {
			case CompactionStartedEvent:
				started++
			case ErrorEvent:
				if err == nil {
					err = e.Err
				}
			}
		}
		return started, err
	}

	// Run 1 (unscoped, the plain `pragma` session entry): compacts, then
	// one final iteration — cooldown 1/2 when the turn ends.
	started1, err := countEvents(collectPragmaLoopEvents(engine.Run(t.Context(), "first turn")))
	if err != nil {
		t.Fatalf("run 1 unexpected ErrorEvent: %v", err)
	}
	if started1 != 1 {
		t.Fatalf("run 1 CompactionStartedEvent count = %d, want 1", started1)
	}

	// Run 2 (scoped): the exact orchestration non-persistent state shape.
	// Two model-request iterations; zero compaction events (the trigger
	// must stay scope-gated).
	scopedEvents := collectPragmaLoopEvents(engine.RunPragmaLoopWithSystemCompletionCheckOptions(
		t.Context(), model.SystemPrompt{}, "second turn", nil, PragmaLoopRunOptions{}))
	started2, err := countEvents(scopedEvents)
	if err != nil {
		t.Fatalf("run 2 unexpected ErrorEvent: %v", err)
	}
	if started2 != 0 {
		t.Fatalf("run 2 CompactionStartedEvent count = %d, want 0 — a scoped run must never compact", started2)
	}
	if prov.calls != 4 {
		t.Fatalf("provider calls after run 2 = %d, want 4 (summary + 3 model requests)", prov.calls)
	}

	// Run 3 (unscoped): the scoped run's two iterations must have expired
	// the cooldown, so this run compacts on its FIRST iteration — before
	// its own model request. If scoped iterations still do not increment
	// (the CMP-001.3.F2 drift), run 3's first iteration sits inside the
	// cooldown, the conversation stays whole, and the compacted request
	// never happens this turn.
	started3, err := countEvents(collectPragmaLoopEvents(engine.Run(t.Context(), "third turn")))
	if err != nil {
		t.Fatalf("run 3 unexpected ErrorEvent: %v", err)
	}
	if started3 != 1 {
		t.Fatalf("run 3 CompactionStartedEvent count = %d, want 1 — the scoped run's %d model-request iterations did not advance the shared cooldown (CMP-001.3.F2)", started3, 2)
	}
	if prov.calls != 6 {
		t.Fatalf("provider calls after run 3 = %d, want 6 (two summaries + four model requests)", prov.calls)
	}
}
