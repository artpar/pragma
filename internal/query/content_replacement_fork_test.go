package query

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/session"
)

// CMP-001.4a.F2 gate (critic finding C-2). The finding's structural
// legs are CONFIRMED: config.RecordContentReplacements is wired in
// cli/deps.go to contentReplacementRecorder — a closure over the ROOT
// Deps whose body writes through the ROOT's SessionWriter — and
// ForkFreshConversation copies the whole EngineConfig
// (subCfg := engine.config), so every fork carries the root-bound
// recorder while reconstructing contentReplacementState from its own
// fresh conversation seeded with the ROOT's resume-loaded
// ContentReplacementRecords (pinned below).
//
// The consequence leg is REFUTED at HEAD: "fork-side content-replacement
// records land in the root's state" cannot occur, because the callback
// has NO production caller. Its only invocation ever — runLoop's
// request-build helper, next to toolresult.ApplyToolResultBudget (the
// only producer of records) — was removed at fed8bd7 (2026-06-08 10:41),
// six hours BEFORE ForkFreshConversation was born (27925a8, 17:02), so
// no commit ever had a fork with a reachable recorder call. The toolresult
// package as a whole is imported only by engine.go, whose
// contentReplacementState field is write-only.
//
// This gate executes the refutation through the real production shape
// — a real Bash tool call returning a far-oversized result (150k chars;
// the reaper's thresholds were 50k/100k) on BOTH the root engine and a
// fork, over a real session.Writer wired exactly like the production
// recorder — and asserts zero content_replacement entries reach the
// root session file. If the request-build reaper is ever revived, this
// gate fails and forces the fork question to be answered (rebind on
// forks, or nil the fork's recorder the way CMP-001.4a.F1 nil'd
// SessionCheckpoint).
func TestForkContentReplacementRecordsDoNotLandInRootSession(t *testing.T) {
	sessionPath := filepath.Join(t.TempDir(), "session.jsonl")
	w, err := session.NewWriter(sessionPath)
	if err != nil {
		t.Fatalf("create session writer: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	// The production recorder body (deps.go contentReplacementRecorder):
	// records land in the ROOT engine's session writer.
	recorder := func(records []model.ContentReplacementRecord) error {
		return w.WriteContentReplacement(records)
	}

	oversizedBashCall := func(id string) model.Response {
		t.Helper()
		callInput, err := json.Marshal(map[string]string{
			"cmd": "head -c 150000 /dev/zero | tr '\\0' 'x'",
		})
		if err != nil {
			t.Fatal(err)
		}
		return model.Response{
			Content:     []model.ContentPart{model.ToolCallPart{ID: id, Name: "Bash", Input: callInput}},
			StopReason:  model.StopToolUse,
		}
	}
	endTurn := func(text string) model.Response {
		return model.Response{
			Content:    []model.ContentPart{model.TextPart{Text: text}},
			StopReason: model.StopEndTurn,
		}
	}

	// One shared provider, as ForkFreshConversation shares it: the root
	// run consumes responses 0-1, the fork run 2-3.
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		oversizedBashCall("root-call-1"),
		endTurn("root done"),
		oversizedBashCall("fork-call-1"),
		endTurn("fork done"),
	}}

	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	root := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(64), EngineConfig{
		Model:                   "test-model",
		LoopMode:                LoopModeProviderTools,
		MaxTokens:               4096,
		MaxTurns:               10,
		RecordContentReplacements: recorder, // the deps.go:413 wiring shape
	})

	for _, ev := range collectPragmaLoopEvents(root.Run(t.Context(), "root work")) {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected ErrorEvent from the root run: %v", e.Err)
		}
	}
	if got := countContentReplacementEntries(t, sessionPath); got != 0 {
		t.Fatalf("root leg: %d content_replacement entries in the root session file, want 0 — the request-build reaper is dead in production (no caller since fed8bd7); reviving it must answer the fork-inheritance question first (CMP-001.4a.F2)", got)
	}

	fork, _ := root.ForkFreshConversation()
	// Structural pin: the fork DOES inherit the root-bound recorder
	// closure (the confirmed leg — defused only by the callback's
	// deadness, the SessionRewrite posture documented in CMP-001.4a).
	// If this pin starts failing because the field was deliberately
	// nil'd/rebound on forks, update the CMP-001.4a.F2 record with it.
	if fork.config.RecordContentReplacements == nil {
		t.Fatalf("premise: the fork no longer inherits the root's RecordContentReplacements closure — the CMP-001.4a.F2 record documents the inheritance as confirmed (defused by deadness, not removed)")
	}
	for _, ev := range collectPragmaLoopEvents(fork.Run(t.Context(), "persona work")) {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected ErrorEvent from the fork run: %v", e.Err)
		}
	}

	if prov.calls != 4 {
		t.Fatalf("provider calls = %d, want 4 (root tool call + end turn, fork tool call + end turn)", prov.calls)
	}
	// Premise + verbatim leg: both conversations' request builds passed
	// over the oversized result (150k chars > the reaper's 50k/100k
	// thresholds) without any persisted-output replacement — this is
	// the exact request-build point where the pre-fed8bd7 reaper fired.
	requestsWithOversizedResult := 0
	for _, req := range prov.requests {
		for _, msg := range req.Messages {
			for _, part := range msg.Content {
				tr, ok := part.(model.ToolResultPart)
				if !ok || len(tr.Content) <= 140_000 {
					continue
				}
				requestsWithOversizedResult++
				if strings.Contains(tr.Content, "<persisted-output") {
					t.Fatalf("oversized tool result was replaced by a persisted-output pointer before reaching the model")
				}
			}
		}
	}
	if requestsWithOversizedResult < 2 {
		t.Fatalf("requests carrying the oversized tool result verbatim = %d, want >= 2 (the root's and the fork's own request builds) — without this premise the zero-entry assertions below prove nothing (CMP-001.4a.F2)", requestsWithOversizedResult)
	}

	if got := countContentReplacementEntries(t, sessionPath); got != 0 {
		t.Fatalf("fork leg: %d content_replacement entries landed in the ROOT session file, want 0 — fork-side content-replacement records must not land in the root's state (CMP-001.4a.F2 refutation)", got)
	}
}

func countContentReplacementEntries(t *testing.T, path string) int {
	t.Helper()
	f, err := os.Open(path)
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
		if entry.Kind == session.EntryContentReplacement {
			count++
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan session file: %v", err)
	}
	return count
}

