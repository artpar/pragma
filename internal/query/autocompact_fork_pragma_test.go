package query

import (
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// CMP-001.3.F1 gates. The CMP-001.3 port put the auto-compact trigger in
// the DEFAULT pragma loop, and orchestration persistent-conversation
// states run that loop UNSCOPED on forked engines
// (orchestration/runner.go engineForOrchestrationState →
// root.ForkFreshConversation; RunStateEvents →
// RunPragmaLoopWithSystemCompletionCheckOptions with
// IncludePriorConversation = stateUsesPersistentConversation). At the
// audited commit (1b4fb1b) the fork constructor still copied the root's
// live compactor/autoTracker/windowConfig, so a persistent state's fork
// could compact: its rewriteSession fired the ROOT's SessionRewrite
// closure (rewriteCurrentSession rewrites the root store's session file
// and resets the root SessionLastIdx), its RecordSuccess/RecordFailure
// mutated the root's shared AutoTracker (cooldown/breaker), and its
// summary went through the root's compactor — i.e. the root's provider —
// even after applyStateLLMRuntime rebinds the fork's persona provider.
// CMP-001.4a removed the fork inheritance (all fork paths route through
// ForkFreshConversation), defusing every leg; these gates pin the
// payload's exact path so re-enabling fork compaction without giving the
// fork its own deps/store rebinding cannot silently resurrect the leak.

// TestPersistentForkPragmaLoopNeverCompactsOrRewritesRootSession drives
// the exact orchestration persistent-state shape: a root engine with
// LIVE compaction deps and a session-rewrite hook, a fork created by
// ForkFreshConversation, a persona provider rebound via BindProvider,
// and an UNSCOPED pragma-loop run (IncludePriorConversation) whose
// conversation grows far over the threshold. The fork run must complete
// normally with zero compaction events, the root's rewrite hook must
// never fire, and the root's tracker must stay clean.
func TestPersistentForkPragmaLoopNeverCompactsOrRewritesRootSession(t *testing.T) {
	// The root's provider: it serves ONLY the fork's compaction summary
	// on a leaky tree (the root's compact.Service is constructed over
	// it); on a fixed tree it must never be called at all.
	rootProv := &pragmaLoopTestProvider{responses: []model.Response{
		{
			Content:    []model.ContentPart{model.TextPart{Text: "Summary: the persona state did its work."}},
			StopReason: model.StopEndTurn,
		},
	}}
	// The persona provider (applyStateLLMRuntime → BindProvider): serves
	// the fork's model turns. Turns 1 and 2 grow the fork's fresh
	// conversation far over the 904-token threshold via huge bash-block
	// actions (two turns so the trigger check runs over a conversation
	// with enough messages for a SUCCEEDING compaction — the leg that
	// fires the root's session rewrite); turn 3 is the final text.
	hugeText := strings.Repeat("word ", 3000)
	personaProv := &pragmaLoopTestProvider{responses: []model.Response{
		{
			Content:    []model.ContentPart{model.TextPart{Text: hugeText + "\n```bash\ntrue\n```"}},
			StopReason: model.StopEndTurn,
		},
		{
			Content:    []model.ContentPart{model.TextPart{Text: hugeText + "\n```bash\ntrue\n```"}},
			StopReason: model.StopEndTurn,
		},
		{
			Content:    []model.ContentPart{model.TextPart{Text: "done"}},
			StopReason: model.StopEndTurn,
		},
	}}

	// The root conversation is itself far over the threshold, so the
	// root's deps are live and its tracker eligible — the precondition
	// that made the inherited-deps leak reach a LIVE root.
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	conv.Messages = []model.Message{
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: hugeText}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: hugeText}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: hugeText}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: hugeText}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: hugeText}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: hugeText}}},
	}
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          t.TempDir(),
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	root := NewEngine(rootProv, store, model.NewCostTracker(0), observe.NewEventBus(64), EngineConfig{
		Model:    "test-model",
		LoopMode: LoopModePragma,
		MaxTurns: 5,
	})
	// EffectiveWindow = 20000 - 4096 - 2000 = 13904; threshold = 904.
	window := compact.WindowConfig{
		ContextWindow:   20_000,
		MaxOutput:       4096,
		SystemPromptEst: 2000,
	}
	root.SetCompaction(CompactionDeps{
		Compactor:    compact.NewService(rootProv, observe.NewEventBus(64), model.NewCostTracker(0), "test-model"),
		AutoTracker:  compact.NewAutoTracker(false),
		WindowConfig: window,
	})
	// The exact production SessionRewrite seam (run.go wires
	// rewriteCurrentSession(d) here). Read after the fork run drains,
	// so the channel close orders the write before the read.
	rootRewritten := false
	root.SetSessionRewrite(func() error {
		rootRewritten = true
		return nil
	})

	fork, _ := root.ForkFreshConversation()
	// The persona rebind: a fork compaction on a leaky tree still ran
	// through the ROOT's compactor (root provider/model); pin that the
	// rebind cannot make the fork compact at all.
	fork.BindProvider(personaProv, "persona-prov", "persona-model")

	events := collectPragmaLoopEvents(fork.RunPragmaLoopWithSystemCompletionCheckOptions(
		t.Context(), model.SystemPrompt{}, "persona state work", nil,
		PragmaLoopRunOptions{IncludePriorConversation: true}))

	var started, compacted, failed, complete int
	for _, ev := range events {
		switch e := ev.(type) {
		case ErrorEvent:
			t.Fatalf("unexpected ErrorEvent from the fork run: %v", e.Err)
		case CompactionStartedEvent:
			started++
		case CompactionEvent:
			compacted++
		case CompactionFailedEvent:
			failed++
		case TurnCompleteEvent:
			complete++
		}
	}
	if started != 0 {
		t.Fatalf("CompactionStartedEvent count = %d, want 0 — the persistent-state fork's compaction deps activated auto-compaction on the unscoped pragma-loop run (CMP-001.3.F1)", started)
	}
	if compacted != 0 {
		t.Fatalf("CompactionEvent count = %d, want 0 — the persistent-state fork compacted (CMP-001.3.F1)", compacted)
	}
	if failed != 0 {
		t.Fatalf("CompactionFailedEvent count = %d, want 0 — the persistent-state fork attempted compaction and failed (CMP-001.3.F1)", failed)
	}
	if complete != 1 {
		t.Fatalf("TurnCompleteEvent count = %d, want 1 — the fork run must complete normally without compaction", complete)
	}
	if rootRewritten {
		t.Fatalf("the fork's compaction rewrote the ROOT's session file through the inherited SessionRewrite closure (CMP-001.3.F1)")
	}
	if rootProv.calls != 0 {
		t.Fatalf("the root's provider served %d fork compaction call(s) — the fork compacted via the root's provider/model even after the persona rebind (CMP-001.3.F1)", rootProv.calls)
	}
	if got := root.autoTracker.FailureCount(); got != 0 {
		t.Fatalf("root breaker contaminated by the fork run: %d compaction failures counted toward the root (CMP-001.3.F1)", got)
	}
	if !root.autoTracker.ShouldAutoCompact(20_000, window) {
		t.Fatalf("root auto-compaction denied after the fork run — the fork's compaction engaged the root's cooldown through the shared tracker (CMP-001.3.F1)")
	}
}
