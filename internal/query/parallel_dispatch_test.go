package query

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/tools/applypatch"
)

// PAR-001 gates: sibling tool calls in one assistant turn execute
// concurrently — a quick call's result is observable while a slow sibling
// is still in flight — while the results message keeps call order, stays
// one message, and pairs every call. Same-batch apply_patch calls stay
// sequential in call order (same-file lost-update hazard).

func parallelDispatchResponses() []model.Response {
	blocker, err := json.Marshal(map[string]string{"cmd": "sleep 1; echo PAR_BLOCKER_DONE"})
	if err != nil {
		panic(err)
	}
	quick, err := json.Marshal(map[string]string{"cmd": "echo PAR_QUICK_DONE"})
	if err != nil {
		panic(err)
	}
	return []model.Response{
		{
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "call-par-blocker", Name: "Bash", Input: blocker},
				model.ToolCallPart{ID: "call-par-quick", Name: "Bash", Input: quick},
			},
			StopReason: model.StopToolUse,
		},
		{
			Content:    []model.ContentPart{model.TextPart{Text: "done"}},
			StopReason: model.StopEndTurn,
		},
	}
}

func TestProviderToolsLoopDispatchesSiblingCallsConcurrently(t *testing.T) {
	prov := &pragmaLoopTestProvider{responses: parallelDispatchResponses()}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
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
		MaxTurns:  5,
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "work"))

	// Sanity: the loop must complete both turns — the failure below must
	// be about dispatch concurrency, not a broken loop.
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", prov.calls)
	}

	var callOrder []string
	quickResultIdx, blockerResultIdx := -1, -1
	for i, ev := range events {
		switch e := ev.(type) {
		case ToolCallEvent:
			callOrder = append(callOrder, e.Call.ID)
		case ToolResultEvent:
			switch e.Result.ToolCallID {
			case "call-par-quick":
				quickResultIdx = i
			case "call-par-blocker":
				blockerResultIdx = i
			}
		}
	}
	if len(callOrder) != 2 || callOrder[0] != "call-par-blocker" || callOrder[1] != "call-par-quick" {
		t.Fatalf("ToolCallEvent order = %v, want [call-par-blocker, call-par-quick]", callOrder)
	}
	if quickResultIdx < 0 || blockerResultIdx < 0 {
		t.Fatalf("missing tool results: quick=%d blocker=%d", quickResultIdx, blockerResultIdx)
	}
	if quickResultIdx > blockerResultIdx {
		t.Fatalf("quick result (event %d) not observed before blocker result (event %d): sibling calls executed serially",
			quickResultIdx, blockerResultIdx)
	}

	// Results message: call order preserved, one user message, both calls
	// paired, and no stamp-only companion follows (CLK-002 ban).
	req1 := prov.requests[1].Messages
	if len(req1) != 3 {
		t.Fatalf("request 1 carries %d messages, want 3 (prompt, assistant, results)", len(req1))
	}
	resultsMsg := req1[2]
	if len(resultsMsg.Content) != 2 {
		t.Fatalf("results message carries %d parts, want 2", len(resultsMsg.Content))
	}
	first, ok := resultsMsg.Content[0].(model.ToolResultPart)
	if !ok {
		t.Fatalf("results content[0] is %T, want model.ToolResultPart", resultsMsg.Content[0])
	}
	second, ok := resultsMsg.Content[1].(model.ToolResultPart)
	if !ok {
		t.Fatalf("results content[1] is %T, want model.ToolResultPart", resultsMsg.Content[1])
	}
	if first.ToolCallID != "call-par-blocker" || second.ToolCallID != "call-par-quick" {
		t.Fatalf("results order = [%s, %s], want call order [call-par-blocker, call-par-quick]",
			first.ToolCallID, second.ToolCallID)
	}
	if !strings.Contains(first.Content, "PAR_BLOCKER_DONE") || !strings.Contains(second.Content, "PAR_QUICK_DONE") {
		t.Fatalf("results content mismatch: %q / %q", first.Content, second.Content)
	}
	assertNoStampOnlyUserMessages(t, req1, "request 1")
}

// Hazard invariant: two same-batch apply_patch calls on one file must both
// succeed in call order. Passes on the sequential baseline; guards the
// candidate's sequential apply_patch lane (a concurrent second patch would
// race the first: file-not-found or lost update).
func TestProviderToolsLoopSerializesSameBatchApplyPatch(t *testing.T) {
	patchOne := "*** Begin Patch\n*** Add File: par_serial.txt\n+alpha\n*** End Patch\n"
	patchTwo := "*** Begin Patch\n*** Update File: par_serial.txt\n@@\n-alpha\n+bravo\n*** End Patch\n"
	cwd := t.TempDir()

	patchCalls := make([]model.ContentPart, 0, 2)
	for _, p := range []struct {
		id    string
		patch string
	}{
		{"call-par-patch-one", patchOne},
		{"call-par-patch-two", patchTwo},
	} {
		input, err := json.Marshal(map[string]string{"patch": p.patch})
		if err != nil {
			t.Fatal(err)
		}
		patchCalls = append(patchCalls, model.ToolCallPart{ID: p.id, Name: applypatch.ToolName, Input: input})
	}
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{Content: patchCalls, StopReason: model.StopToolUse},
		{Content: []model.ContentPart{model.TextPart{Text: "done"}}, StopReason: model.StopEndTurn},
	}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", cwd)
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          cwd,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(64), EngineConfig{
		Model:     "test-model",
		LoopMode:  LoopModeProviderTools,
		MaxTokens: 4096,
		MaxTurns:  5,
	})

	collectPragmaLoopEvents(engine.Run(t.Context(), "work"))

	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", prov.calls)
	}
	resultsMsg := prov.requests[1].Messages[2]
	for i, wantID := range []string{"call-par-patch-one", "call-par-patch-two"} {
		rp, ok := resultsMsg.Content[i].(model.ToolResultPart)
		if !ok {
			t.Fatalf("results content[%d] is %T, want model.ToolResultPart", i, resultsMsg.Content[i])
		}
		if rp.ToolCallID != wantID {
			t.Fatalf("results[%d] ToolCallID = %q, want %q", i, rp.ToolCallID, wantID)
		}
		if rp.IsError {
			t.Fatalf("results[%d] is an error: %q", i, rp.Content)
		}
	}
	final, err := os.ReadFile(cwd + "/par_serial.txt")
	if err != nil {
		t.Fatalf("reading patched file: %v", err)
	}
	if string(final) != "bravo\n" {
		t.Fatalf("file content = %q, want %q", string(final), "bravo\n")
	}
}
