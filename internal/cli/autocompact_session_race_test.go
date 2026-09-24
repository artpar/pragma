package cli

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/session"
)

// TestOperatorInputDuringCompactionRewriteIsSerialized reproduces the
// CMP-001.2.F5 (critic C-5) unsynchronized-writer window: the loop
// goroutine's post-compaction session rewrite (the SetSessionRewrite hook
// body, rewriteCurrentSession — run.go wires it next to every
// SetSessionCheckpoint) and the UI goroutine's mid-turn operator input
// (Engine.AppendUserInput → checkpointSession → the SetSessionCheckpoint
// saveFn from makeSessionSaveClose) both read and write d.SessionLastIdx
// with no lock in common. The session Writer itself is mutex-protected
// (internal/session/writer.go), so the exposure is the shared index int:
// on the unfixed code the two goroutines race on a plain int (Go memory
// model: undefined behavior), and the interleaving can also splice a
// pre-compaction incremental checkpoint onto a just-rewritten file.
//
// The gate for this regression is `go test -race`: both goroutines execute
// the exact production functions of the operator-input-during-compaction
// window — the loop side runs what the compaction success path runs
// (provider_tools_loop.go rewriteSession → the SetSessionRewrite hook) and
// the UI side runs the INT-001 mid-turn path (RunInput → AppendUserInput →
// checkpointSession). The compaction TRIGGER is driven directly (no
// provider compaction request) because the subject under test is the
// persistence serialization, not the trigger; the end-to-end trigger→rewrite
// path is covered by TestAutoCompactRewritesResumableSessionFile.
func TestOperatorInputDuringCompactionRewriteIsSerialized(t *testing.T) {
	// Redirect ~/.pragma (session.NewStore resolves it via os.UserHomeDir)
	// so the test owns the session store.
	t.Setenv("HOME", t.TempDir())

	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	// Real session messages carry IDs (every engine append mints one);
	// the session writer dedups by ID, so the seeded history must too.
	seedIDs := []string{model.NewUUID(), model.NewUUID(), model.NewUUID(), model.NewUUID()}
	conv.Messages = []model.Message{
		{ID: seedIDs[0], Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "seed user one"}}},
		{ID: seedIDs[1], Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: "seed assistant one"}}},
		{ID: seedIDs[2], Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "seed user two"}}},
		{ID: seedIDs[3], Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: "seed assistant two"}}},
	}
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	bus := observe.NewEventBus(64)

	// Persist the seeded history to a session file as a long-running
	// session would have, then reopen in append mode exactly like the
	// --resume path (deps.go: sessionWriter = sessionStore.Open(id),
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

	// The provider is never invoked: the race window is persistence-side,
	// so no compaction request is scripted.
	prov := &compactSessionTestProvider{}
	engine := query.NewEngine(prov, store, d.CostTracker, bus, query.EngineConfig{
		Model:     "test-model",
		LoopMode:  query.LoopModeProviderTools,
		MaxTokens: 4096,
	})
	sessionSaveFn, sessionCloseFn := makeSessionSaveClose(d)
	// Wire the engine exactly as the production runtimes do: the
	// checkpoint is the saveFn the UI goroutine reaches through
	// AppendUserInput, and the rewrite is what the loop goroutine runs
	// after auto-compaction applies its result.
	engine.SetSessionCheckpoint(sessionSaveFn)
	engine.SetSessionRewrite(func() error { return rewriteCurrentSession(d) })

	const rounds = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		// Loop goroutine: the post-compaction rewrite, exactly the
		// SetSessionRewrite hook body (rewriteCurrentSession(d)).
		defer wg.Done()
		<-start
		for i := 0; i < rounds; i++ {
			if err := rewriteCurrentSession(d); err != nil {
				t.Errorf("loop-side rewriteCurrentSession: %v", err)
				return
			}
		}
	}()
	go func() {
		// UI goroutine: mid-turn operator input (INT-001) — AppendUserInput
		// appends the message and checkpoints through the same saveFn.
		defer wg.Done()
		<-start
		for i := 0; i < rounds; i++ {
			if err := engine.AppendUserInput(fmt.Sprintf("operator input during compaction %d", i)); err != nil {
				t.Errorf("UI-side AppendUserInput: %v", err)
				return
			}
		}
	}()
	close(start)
	wg.Wait()

	// Consistency invariants: after the window the index matches the live
	// conversation and the durable file carries every message (nothing
	// lost to the interleaving).
	snap := d.Store.Snapshot()
	if d.SessionLastIdx != len(snap.Conversation.Messages) {
		t.Fatalf("SessionLastIdx = %d after the window, want %d (the live conversation length)", d.SessionLastIdx, len(snap.Conversation.Messages))
	}
	if err := sessionCloseFn(); err != nil {
		t.Fatalf("close session writer: %v", err)
	}
	reloaded, err := sessStore.Load(conv.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	sawInputs := 0
	for _, msg := range reloaded.Conversation.Messages {
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok {
				if strings.Contains(tp.Text, "operator input during compaction") {
					sawInputs++
				}
			}
		}
	}
	if sawInputs != rounds {
		t.Fatalf("durable session carries %d of %d mid-window operator inputs — the interleaving lost input", sawInputs, rounds)
	}
	if len(reloaded.Conversation.Messages) != len(conv.Messages)+rounds {
		t.Fatalf("durable session has %d messages, want %d (seeds + inputs)", len(reloaded.Conversation.Messages), len(conv.Messages)+rounds)
	}
}
