package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/session"
)

// TestSessionCheckpointRecoversFromFailedCompactionRewrite reproduces the
// CMP-001.2 F2 recovery defect: the post-compaction session rewrite FAILS
// after the loop already replaced the store and recorded success — only a
// turn ErrorEvent is emitted, no clamp, no repair. d.SessionLastIdx keeps
// the stale pre-compaction value, so the next incremental checkpoint
// (makeSessionSaveClose) writes nothing and then unconditionally resets
// the index: the durable file keeps the pre-compaction bulk history,
// later checkpoints splice post-compaction messages onto the wrong base,
// and --resume resurrects the bulk history while losing the summary and
// everything appended below the stale index.
//
// Failure-injection boundary (documented): the rewrite failure is
// injected at the EngineConfig.SessionRewrite hook seam — the exact
// interface where a rewriteCurrentSession failure (session load, truncate,
// encode, sync) surfaces to the loop. Everything after the failure — the
// stale-index post-failure state, the next checkpoint, the recovery, and
// the --resume reload — runs real production code (makeSessionSaveClose,
// rewriteCurrentSession, session.Store.Load) over a real session JSONL
// file. The second engine run restores the production hook wiring.
func TestSessionCheckpointRecoversFromFailedCompactionRewrite(t *testing.T) {
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

	// Run 1: auto-compaction replaces the store (summary + preserved
	// prompt), then the post-compaction rewrite fails — injected at the
	// hook seam, exactly where rewriteCurrentSession's IO error would
	// surface to the loop. The turn must end with the loop's real
	// error event.
	rewriteErr := errors.New("simulated disk failure during post-compaction rewrite")
	engine.SetSessionRewrite(func() error { return rewriteErr })
	sawRewriteError := false
	for ev := range engine.Run(t.Context(), "continue the work") {
		if errEv, ok := ev.(query.ErrorEvent); ok {
			if !strings.Contains(errEv.Err.Error(), "rewrite session after compaction") {
				t.Fatalf("run 1 errored for an unexpected reason: %v", errEv.Err)
			}
			if !strings.Contains(errEv.Err.Error(), rewriteErr.Error()) {
				t.Fatalf("run 1 rewrite error does not carry the injected failure: %v", errEv.Err)
			}
			sawRewriteError = true
		}
	}
	if !sawRewriteError {
		t.Fatalf("run 1 ended without the post-compaction rewrite error — the failure path was not exercised")
	}

	// Premise legs (hold on the unfixed tree too): the store was compacted
	// and the error path clamped/repaired nothing — SessionLastIdx keeps
	// the stale pre-compaction value (6 seeded messages + the run-1
	// prompt already written by the turn-start checkpoint).
	postCompact := store.Snapshot().Conversation.Messages
	if len(postCompact) >= len(conv.Messages) {
		t.Fatalf("store was not compacted in run 1 (still %d messages)", len(postCompact))
	}
	if d.SessionLastIdx != len(conv.Messages)+1 {
		t.Fatalf("premise: SessionLastIdx = %d after the failed rewrite, want the stale %d (pre-compaction writes) — the error path changed the index",
			d.SessionLastIdx, len(conv.Messages)+1)
	}

	// Run 2: the operator continues — the next message checkpoint runs
	// through the real production saveFn, with the production rewrite
	// hook restored (the runtime keeps rewriteCurrentSession wired across
	// turns).
	engine.SetSessionRewrite(func() error { return rewriteCurrentSession(d) })
	for ev := range engine.Run(t.Context(), "second operator prompt") {
		if errEv, ok := ev.(query.ErrorEvent); ok {
			t.Fatalf("run 2 errored: %v", errEv.Err)
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
		t.Fatalf("resumed session replays %d messages (>= the %d-message pre-compaction history) — the failed post-compaction rewrite was never repaired: the next checkpoint wrote nothing below the stale index and spliced onto the pre-compaction file",
			len(msgs), len(conv.Messages))
	}
	var sawSummary, sawReply, sawSecondPrompt, sawBig bool
	for _, msg := range msgs {
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok {
				if strings.Contains(tp.Text, "long history of routine exchanges") {
					sawSummary = true
				}
				if strings.Contains(tp.Text, "OK, noted.") {
					sawReply = true
				}
				if strings.Contains(tp.Text, "second operator prompt") {
					sawSecondPrompt = true
				}
				if strings.Contains(tp.Text, bigText[:100]) {
					sawBig = true
				}
			}
		}
	}
	if sawBig {
		t.Fatalf("resumed session replays pre-compaction bulk history — the failed rewrite left the bulk in the file and nothing repaired it")
	}
	if !sawSummary {
		t.Fatalf("resumed session lost the compaction summary — it was never persisted below the stale SessionLastIdx")
	}
	if !sawSecondPrompt {
		t.Fatalf("resumed session lost the run-2 operator prompt — the next checkpoint skipped it below the stale index")
	}
	if !sawReply {
		t.Fatalf("resumed session lost the post-compaction exchange — later checkpoints spliced onto the wrong base")
	}
}
