package query

import (
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// TestProviderToolsLoopCompactionEventPostTokensCoverPendingTail pins
// the CMP-001.2 F4 operator-visible leg: the CompactionEvent emitted
// after a successful auto-compaction must report the post-compaction
// conversation — the summary PLUS the re-appended pending prompt — not
// the summary alone. Before F4 the event (and the cooldown that engages
// on the same success) treated a compaction as shrinking to the
// summary's size even when a large unanswered tail was preserved, so a
// post-compaction request still over the trigger threshold was reported
// as a tiny one.
func TestProviderToolsLoopCompactionEventPostTokensCoverPendingTail(t *testing.T) {
	// Call 1: compaction summary — deliberately tiny and silent about
	// the pending prompt. Call 2: end-turn over the compacted
	// conversation.
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{
			Content:    []model.ContentPart{model.TextPart{Text: "Summary: user asked questions."}},
			StopReason: model.StopEndTurn,
		},
		{
			Content:    []model.ContentPart{model.TextPart{Text: "OK, noted."}},
			StopReason: model.StopEndTurn,
		},
	}}

	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	// ~3750 heuristic tokens per message; four messages put the
	// conversation far above the 904-token threshold configured below.
	bigText := strings.Repeat("word ", 3000)
	conv.Messages = []model.Message{
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

	// The run prompt becomes the pending tail: unanswered until the
	// first assistant reply, so compaction (which fires at the top of
	// iteration 0, before any response) must preserve it. It is much
	// larger than the summary, so a summary-only PostTokens cannot even
	// cover it.
	pendingPrompt := strings.Repeat("pending prompt ", 400)
	events := collectPragmaLoopEvents(engine.Run(t.Context(), pendingPrompt))

	var compacted int
	var preTokens, postTokens int
	for _, ev := range events {
		switch e := ev.(type) {
		case CompactionEvent:
			compacted++
			preTokens, postTokens = e.PreTokens, e.PostTokens
		case CompactionFailedEvent:
			t.Fatalf("compaction failed: %+v", e)
		}
	}
	if compacted != 1 {
		t.Fatalf("CompactionEvent count = %d, want 1", compacted)
	}

	pendingTokens := compact.EstimateTokens(model.Message{
		Role:    model.RoleUser,
		Content: []model.ContentPart{model.TextPart{Text: pendingPrompt}},
	})
	if postTokens < pendingTokens {
		t.Fatalf("CompactionEvent.PostTokens = %d does not even cover the re-appended pending prompt (%d tokens) — the event reports the summary alone while the real post-compaction request carries summary + pending tail (CMP-001.2 F4)",
			postTokens, pendingTokens)
	}
	if postTokens >= preTokens {
		t.Fatalf("postTokens %d not below preTokens %d — the compaction still shrinks the conversation (the summary is far below the 4-message prefix)",
			postTokens, preTokens)
	}
}
