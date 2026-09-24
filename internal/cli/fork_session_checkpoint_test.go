package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/session"
)

// CMP-001.4a.F1 gates. Fork engines inherit the root's EngineConfig
// (engine.go ForkFreshConversation: subCfg := engine.config), which until
// this fix carried the root's SessionCheckpoint closure — the run.go
// makeSessionSaveClose saveFn wired next to every SetSessionCheckpoint
// site. Unlike SessionRewrite (defused by CMP-001.4a: its only callers
// are compaction success branches unreachable on deps-nil forks), the
// checkpoint closure is live on EVERY fork message append: all fork loops
// append through appendConversationMessage → checkpointSession →
// config.SessionCheckpoint(). The closure snapshots the ROOT engine's
// store (d.Store), so a fork append persists nothing of the fork's own
// conversation but unconditionally rewrites the ROOT session's metadata
// entry and emits a SessionSaved event keyed to the root conversation —
// per append, for every subagent (Agent tool) and orchestration persona
// fork. The pre-F5 memory-race/double-write aspect of this leg is
// REFUTED at HEAD (CMP-001.2.F5's d.sessionMu serializes both paths
// through the same closure — gated below under -race); the live defect
// is the spurious root-session persistence work and events per fork
// append, gated here.

type sessionSavedRecorder struct {
	mu     sync.Mutex
	events []observe.SessionSaved
}

func (r *sessionSavedRecorder) HandleEvent(e observe.Event) {
	if s, ok := e.(observe.SessionSaved); ok {
		r.mu.Lock()
		r.events = append(r.events, s)
		r.mu.Unlock()
	}
}

func (r *sessionSavedRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

func countSessionFileEntries(t *testing.T, sessionID string, kind session.EntryKind) int {
	t.Helper()
	dir, err := config.SessionsDir()
	if err != nil {
		t.Fatalf("resolve sessions dir: %v", err)
	}
	f, err := os.Open(filepath.Join(dir, sessionID+".jsonl"))
	if err != nil {
		t.Fatalf("open session file: %v", err)
	}
	defer f.Close()
	count := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var entry session.Entry
		if err := json.Unmarshal(sc.Bytes(), &entry); err != nil {
			t.Fatalf("parse session entry %q: %v", sc.Text(), err)
		}
		if entry.Kind == kind {
			count++
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan session file: %v", err)
	}
	return count
}

// forkCheckpointRootRuntime builds the production persistence wiring the
// interactive runtime uses: a real session JSONL seeded with a 6-message
// root conversation (ID'd messages — the writer dedups by ID), reopened
// in append mode like --resume, a Deps with the real saveFn/closeFn from
// makeSessionSaveClose, and a root engine whose SessionCheckpoint is
// that saveFn.
func forkCheckpointRootRuntime(t *testing.T, prov provider.Provider) (*query.Engine, *Deps, *sessionSavedRecorder, func() error) {
	t.Helper()
	// Redirect ~/.pragma (session.NewStore resolves it via os.UserHomeDir).
	t.Setenv("HOME", t.TempDir())

	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	seedIDs := []string{
		model.NewUUID(), model.NewUUID(), model.NewUUID(),
		model.NewUUID(), model.NewUUID(), model.NewUUID(),
	}
	seedText := "root history "
	conv.Messages = []model.Message{
		{ID: seedIDs[0], Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: seedText + "0"}}},
		{ID: seedIDs[1], Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: seedText + "1"}}},
		{ID: seedIDs[2], Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: seedText + "2"}}},
		{ID: seedIDs[3], Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: seedText + "3"}}},
		{ID: seedIDs[4], Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: seedText + "4"}}},
		{ID: seedIDs[5], Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: seedText + "5"}}},
	}
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	bus := observe.NewEventBus(64)
	recorder := &sessionSavedRecorder{}
	unsubscribe := bus.Subscribe(recorder)
	t.Cleanup(unsubscribe)

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
	sessionSaveFn, sessionCloseFn := makeSessionSaveClose(d)
	engine.SetSessionCheckpoint(sessionSaveFn)
	// Production parity: run.go wires SetSessionRewrite next to every
	// SetSessionCheckpoint site. Nothing in these gates compacts, so the
	// rewrite hook is never called; it is wired to prove the fork never
	// fires it.
	engine.SetSessionRewrite(func() error { return rewriteCurrentSession(d) })

	return engine, d, recorder, sessionCloseFn
}

// TestForkAppendsDoNotTouchRootSessionPersistence drives the exact
// subagent shape (Agent tool: ForkFreshConversation + provider-tools
// loop, inherited provider) through a real fork turn and asserts that
// fork appends perform NO root-session persistence: no metadata entries
// appended to the root session file, no SessionSaved events for the
// root conversation, and no fork message text in the durable root
// session. The root's own append path must keep persisting normally
// after the fix (adjacent assertion).
func TestForkAppendsDoNotTouchRootSessionPersistence(t *testing.T) {
	// One end_turn response: the fork's turn appends its prompt and the
	// assistant reply — two appends, each a pre-fix checkpoint of the
	// ROOT session.
	prov := &compactSessionTestProvider{responses: []model.Response{
		{
			Content:    []model.ContentPart{model.TextPart{Text: "subagent work done"}},
			StopReason: model.StopEndTurn,
		},
	}}
	root, d, recorder, sessionCloseFn := forkCheckpointRootRuntime(t, prov)
	defer func() {
		if err := sessionCloseFn(); err != nil {
			t.Fatalf("close session writer: %v", err)
		}
	}()
	rootConvID := d.Store.Snapshot().Conversation.ID

	if got := countSessionFileEntries(t, rootConvID, session.EntryMetadata); got != 0 {
		t.Fatalf("baseline session file already holds %d metadata entries", got)
	}
	if got := recorder.count(); got != 0 {
		t.Fatalf("baseline bus already holds %d SessionSaved events", got)
	}

	fork, _ := root.ForkFreshConversation()
	sawComplete := false
	for ev := range fork.Run(t.Context(), "do the subagent work") {
		switch e := ev.(type) {
		case query.ErrorEvent:
			t.Fatalf("fork run errored: %v", e.Err)
		case query.TurnCompleteEvent:
			sawComplete = true
		}
	}
	if !sawComplete {
		t.Fatalf("fork run did not complete")
	}

	if got := countSessionFileEntries(t, rootConvID, session.EntryMetadata); got != 0 {
		t.Fatalf("fork appends appended %d metadata entries to the ROOT session file (want 0) — the fork inherited the root's SessionCheckpoint closure, so every fork append re-persisted the root session (CMP-001.4a.F1)", got)
	}
	if got := recorder.count(); got != 0 {
		t.Fatalf("fork appends emitted %d SessionSaved events for the ROOT conversation (want 0) — the fork inherited the root's SessionCheckpoint closure (CMP-001.4a.F1)", got)
	}

	// The fork's own conversation must not have leaked into the root
	// session file either (it never did — the closure snapshots the root
	// store — but this pins that the fix does not start persisting fork
	// messages there).
	sessStore, err := session.NewStore()
	if err != nil {
		t.Fatalf("session store: %v", err)
	}
	reloaded, err := sessStore.Load(rootConvID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	for _, msg := range reloaded.Conversation.Messages {
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok {
				if strings.Contains(tp.Text, "do the subagent work") || strings.Contains(tp.Text, "subagent work done") {
					t.Fatalf("fork message text leaked into the ROOT session file")
				}
			}
		}
	}
	if len(reloaded.Conversation.Messages) != 6 {
		t.Fatalf("root session holds %d messages, want the 6 seeded ones (no fork side effects)", len(reloaded.Conversation.Messages))
	}

	// Adjacent: the root's own append path must still persist after the
	// fix — one operator input through the INT-001 busy-turn entry point.
	if err := root.AppendUserInput("operator mid-turn input"); err != nil {
		t.Fatalf("root AppendUserInput: %v", err)
	}
	if got := countSessionFileEntries(t, rootConvID, session.EntryMetadata); got != 1 {
		t.Fatalf("after the root's own append, metadata entries = %d, want 1 — the root's own checkpoint path must keep persisting", got)
	}
	if got := recorder.count(); got != 1 {
		t.Fatalf("after the root's own append, SessionSaved events = %d, want 1", got)
	}
	reloaded2, err := sessStore.Load(rootConvID)
	if err != nil {
		t.Fatalf("reload session after root append: %v", err)
	}
	if len(reloaded2.Conversation.Messages) != 7 {
		t.Fatalf("root session holds %d messages after the root's own append, want 7", len(reloaded2.Conversation.Messages))
	}
	foundInput := false
	for _, msg := range reloaded2.Conversation.Messages {
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok && strings.Contains(tp.Text, "operator mid-turn input") {
				foundInput = true
			}
		}
	}
	if !foundInput {
		t.Fatalf("root session does not contain the root's own appended input — the root checkpoint path was broken by the fork fix")
	}
}

// TestForkCheckpointsAreSerializedWithRootInputCheckpoints documents the
// REFUTED leg of CMP-001.4a.F1 with an executable -race gate: fork
// appends and the UI goroutine's mid-turn AppendUserInput (INT-001)
// both reach the SAME makeSessionSaveClose saveFn closure, which holds
// d.sessionMu across its whole index read/modify/write (CMP-001.2.F5),
// so the fork×root checkpoint interleaving cannot double-write a message
// range into the root session file. Run under -race this gate fails if
// the fork checkpoint path ever re-emerges without the shared lock. The
// consistency assertions (index == live length, every root input durable
// exactly once, no fork input in the file) are the invariants the race
// would violate.
func TestForkCheckpointsAreSerializedWithRootInputCheckpoints(t *testing.T) {
	prov := &compactSessionTestProvider{responses: []model.Response{}}
	root, d, recorder, sessionCloseFn := forkCheckpointRootRuntime(t, prov)
	defer func() {
		if err := sessionCloseFn(); err != nil {
			t.Fatalf("close session writer: %v", err)
		}
	}()
	rootConvID := d.Store.Snapshot().Conversation.ID

	fork, _ := root.ForkFreshConversation()
	const each = 16

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < each; i++ {
			if err := fork.AppendUserInput(fmt.Sprintf("fork input %02d", i)); err != nil {
				t.Errorf("fork AppendUserInput %d: %v", i, err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < each; i++ {
			if err := root.AppendUserInput(fmt.Sprintf("root input %02d", i)); err != nil {
				t.Errorf("root AppendUserInput %d: %v", i, err)
			}
		}
	}()
	wg.Wait()

	live := d.Store.Snapshot().Conversation.Messages
	if want := 6 + each; len(live) != want {
		t.Fatalf("root store holds %d messages, want %d (6 seeded + %d root inputs; fork inputs go to the fork's own store)", len(live), want, each)
	}
	if d.SessionLastIdx != len(live) {
		t.Fatalf("d.SessionLastIdx = %d, want %d — a checkpoint interleaving desynced the index (CMP-001.2.F5 serialization invariant)", d.SessionLastIdx, len(live))
	}

	sessStore, err := session.NewStore()
	if err != nil {
		t.Fatalf("session store: %v", err)
	}
	reloaded, err := sessStore.Load(rootConvID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if len(reloaded.Conversation.Messages) != len(live) {
		t.Fatalf("durable root session holds %d messages, want %d — an interleaved checkpoint lost or duplicated a message range", len(reloaded.Conversation.Messages), len(live))
	}
	for i := 0; i < each; i++ {
		// Zero-padded so no needle is a prefix of another (the durable
		// text also carries the wall-clock stamp line).
		needle := fmt.Sprintf("root input %02d", i)
		count := 0
		for _, msg := range reloaded.Conversation.Messages {
			for _, part := range msg.Content {
				if tp, ok := part.(model.TextPart); ok && strings.Contains(tp.Text, needle) {
					count++
				}
			}
		}
		if count != 1 {
			t.Fatalf("root input %q appears %d times in the durable session, want exactly 1 — an interleaved checkpoint double-wrote or lost the message range", needle, count)
		}
	}
	for _, msg := range reloaded.Conversation.Messages {
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok && strings.Contains(tp.Text, "fork input") {
				t.Fatalf("fork input text leaked into the ROOT session file")
			}
		}
	}
	// Every checkpoint that ran emitted exactly one SessionSaved event
	// for the root conversation; the count must equal the root-side
	// appends (pre-fix the fork side adds its own spurious events, which
	// the first gate forbids — here the invariant is that whatever
	// checkpoints ran left the index consistent).
	if recorder.count() == 0 {
		t.Fatalf("no SessionSaved events — the root append checkpoints did not run")
	}
}
