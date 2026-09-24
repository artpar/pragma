package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/session"
)

// compactSessionTestProvider scripts the two provider calls an auto-compacted
// turn makes: the compaction summary request and the post-compaction model
// turn. It mirrors query's unexported pragmaLoopTestProvider.
type compactSessionTestProvider struct {
	responses []model.Response
	calls     int
}

func (p *compactSessionTestProvider) Name() string { return "test" }

func (p *compactSessionTestProvider) Stream(context.Context, provider.RequestParams) (<-chan provider.StreamChunk, error) {
	return nil, errors.New("Stream is not used by this test provider")
}

func (p *compactSessionTestProvider) Complete(_ context.Context, params provider.RequestParams) (model.Response, error) {
	if p.calls >= len(p.responses) {
		return model.Response{}, fmt.Errorf("no response configured for call %d", p.calls+1)
	}
	p.calls++
	return p.responses[p.calls-1], nil
}

func (p *compactSessionTestProvider) SupportsFeature(provider.Feature) bool { return true }

func (p *compactSessionTestProvider) Pricing(string) (model.Pricing, bool) {
	return model.Pricing{}, false
}

func (p *compactSessionTestProvider) ContextWindow(string) (int, bool) { return 200_000, true }

// TestAutoCompactRewritesResumableSessionFile reproduces the CMP-001.2 F2
// defect: the provider-tools loop's auto-compaction replaces
// store.Conversation.Messages but never rewrites the session file. The
// incremental session writer (makeSessionSaveClose) writes by INDEX into the
// pre-compaction array, so the compacted summary and the first
// post-compaction exchange (which land below SessionLastIdx in the new,
// shorter array) are never written — and on --resume the full pre-compaction
// history replays from the untouched file, resurrecting the exact context
// blowup compaction was meant to remove. Manual /compact already rewrites
// (slash Result.RewriteSession → rewriteCurrentSession); the auto path
// needs the equivalent.
//
// The test wires the engine exactly as the production runtimes do
// (SetSessionCheckpoint with makeSessionSaveClose over a real session JSONL
// writer reopened in append mode, --resume style) and asserts the on-disk
// session a --resume would load.
func TestAutoCompactRewritesResumableSessionFile(t *testing.T) {
	// Redirect ~/.pragma (session.NewStore resolves it via os.UserHomeDir)
	// so the test owns the session store.
	t.Setenv("HOME", t.TempDir())

	prov := &compactSessionTestProvider{responses: []model.Response{
		{
			Content:    []model.ContentPart{model.TextPart{Text: "Summary: long history of routine exchanges."}},
			StopReason: model.StopEndTurn,
		},
		{
			Content:    []model.ContentPart{model.TextPart{Text: "OK, noted."}},
			StopReason: model.StopEndTurn,
		},
	}}

	bigText := strings.Repeat("word ", 3000)
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	// Real session messages carry IDs (every engine append mints one);
	// the session writer dedups by ID, so the seeded history must too.
	seedIDs := []string{
		model.NewUUID(), model.NewUUID(), model.NewUUID(),
		model.NewUUID(), model.NewUUID(), model.NewUUID(),
	}
	conv.Messages = []model.Message{
		{ID: seedIDs[0], Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{ID: seedIDs[1], Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{ID: seedIDs[2], Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{ID: seedIDs[3], Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{ID: seedIDs[4], Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{ID: seedIDs[5], Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
	}
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	bus := observe.NewEventBus(64)

	// Persist the pre-compaction history to a session file as a
	// long-running session would have, then reopen in append mode exactly
	// like the --resume path (deps.go: sessionWriter = sessionStore.Open(id),
	// SessionLastIdx = len(conv.Messages)).
	header := session.HeaderData{
		SessionID: conv.ID,
		Model:     "test-model",
		Provider:  "test",
		WorkDir:   conv.WorkDir,
		CreatedAt: conv.CreatedAt,
		System:    conv.System,
	}
	sessStore, err := session.NewStore()
	if err != nil {
		t.Fatalf("session store: %v", err)
	}
	seedWriter, err := sessStore.Create(header)
	if err != nil {
		t.Fatalf("seed session file: %v", err)
	}
	for i := range conv.Messages {
		if err := seedWriter.WriteMessage(conv.Messages[i]); err != nil {
			t.Fatalf("seed session message %d: %v", i, err)
		}
	}
	if err := seedWriter.Close(); err != nil {
		t.Fatalf("close seed writer: %v", err)
	}
	resumedWriter, err := sessStore.Open(conv.ID)
	if err != nil {
		t.Fatalf("reopen session for append: %v", err)
	}

	d := &Deps{
		Cfg:            config.Config{Provider: "test", Model: "test-model", MaxTokens: 4096},
		Bus:            bus,
		Store:          store,
		CostTracker:    model.NewCostTracker(0),
		Metrics:        observe.NewMetrics(observe.MetricsSeed{}),
		Cwd:            conv.WorkDir,
		SessionHeader:  header,
		SessionWriter:  resumedWriter,
		SessionLastIdx: len(conv.Messages),
	}

	engine := query.NewEngine(prov, store, d.CostTracker, bus, query.EngineConfig{
		Model:     "test-model",
		LoopMode:  query.LoopModeProviderTools,
		MaxTokens: 4096,
	})
	// EffectiveWindow = 20000 - 4096 - 2000 = 13904; threshold = 904 —
	// the 6-message big history is far above it, so auto-compaction fires
	// at the top of iteration 0.
	engine.SetCompaction(query.CompactionDeps{
		Compactor:   compact.NewService(prov, bus, model.NewCostTracker(0), "test-model"),
		AutoTracker: compact.NewAutoTracker(false),
		WindowConfig: compact.WindowConfig{
			ContextWindow:   20_000,
			MaxOutput:       4096,
			SystemPromptEst: 2000,
		},
	})
	sessionSaveFn, sessionCloseFn := makeSessionSaveClose(d)
	engine.SetSessionCheckpoint(sessionSaveFn)
	// Post-CMP-001.2 wiring, mirroring the production runtimes
	// (run.go wires SetSessionRewrite next to every SetSessionCheckpoint).
	engine.SetSessionRewrite(func() error { return rewriteCurrentSession(d) })

	for ev := range engine.Run(t.Context(), "continue the work") {
		if errEv, ok := ev.(query.ErrorEvent); ok {
			t.Fatalf("loop errored: %v", errEv.Err)
		}
	}
	if err := sessionCloseFn(); err != nil {
		t.Fatalf("close session writer: %v", err)
	}

	// Reload the session from disk exactly as --resume does.
	reloaded, err := sessStore.Load(conv.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	msgs := reloaded.Conversation.Messages
	if len(msgs) >= len(conv.Messages) {
		t.Fatalf("resumed session replays %d messages (>= the %d-message pre-compaction history) — the compacted conversation was not persisted",
			len(msgs), len(conv.Messages))
	}
	var sawSummary, sawAssistant, sawBig bool
	for _, msg := range msgs {
		if msg.Flags.IsCompactSummary {
			sawSummary = true
		}
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok {
				if strings.Contains(tp.Text, "long history of routine exchanges") {
					sawSummary = true
				}
				if strings.Contains(tp.Text, "OK, noted.") {
					sawAssistant = true
				}
				if strings.Contains(tp.Text, bigText[:100]) {
					sawBig = true
				}
			}
		}
	}
	if !sawSummary {
		t.Fatalf("resumed session does not contain the compaction summary — the incremental writer skipped it (index below SessionLastIdx)")
	}
	if !sawAssistant {
		t.Fatalf("resumed session does not contain the first post-compaction exchange (assistant reply) — it was written below the stale SessionLastIdx")
	}
	if sawBig {
		t.Fatalf("resumed session replays pre-compaction bulk history — resume resurrects the pre-compaction context")
	}
}
