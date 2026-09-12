package query

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// TUI-004 gate (real local integration): a running foreground Bash call
// must emit its output-so-far to the event channel before it completes —
// the authentic shape is this session's 361-second silent go test/go
// build window (18:57:27 → 19:03:28, log 2026-09-12T18-53-02.jsonl),
// during which the operator saw no command output at all.
//
// The assertion uses only the existing ToolCallEvent/ToolResultEvent
// vocabulary, so it runs (and fails, for the stated reason) on the
// unchanged harness: on the baseline nothing arrives between the call
// and its result.
func TestBashToolEmitsLiveOutputWhileRunning(t *testing.T) {
	input, err := json.Marshal(map[string]string{
		"cmd": "echo TUI004_STEP_ONE; sleep 1.5; echo TUI004_STEP_TWO",
	})
	if err != nil {
		t.Fatalf("marshal tool input: %v", err)
	}
	responses := []model.Response{
		{
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "call-live", Name: "Bash", Input: input},
			},
			StopReason: model.StopToolUse,
		},
		{
			Content:   []model.ContentPart{model.TextPart{Text: "done"}},
			StopReason: model.StopEndTurn,
		},
	}
	prov := &pragmaLoopTestProvider{responses: responses}
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

	callIdx, resultIdx := -1, -1
	for i, ev := range events {
		switch e := ev.(type) {
		case ToolCallEvent:
			if e.Call.ID == "call-live" && callIdx == -1 {
				callIdx = i
			}
		case ToolResultEvent:
			if e.Result.ToolCallID == "call-live" && resultIdx == -1 {
				resultIdx = i
			}
		}
	}
	if callIdx == -1 || resultIdx == -1 {
		t.Fatalf("missing call/result events (call=%d result=%d) in %d events", callIdx, resultIdx, len(events))
	}

	// The first step's output is on disk ~0s into a ~1.5s command: some
	// event between the call and its result must carry it.
	between := events[callIdx+1 : resultIdx]
	for _, ev := range between {
		if strings.Contains(fmt.Sprint(ev), "TUI004_STEP_ONE") {
			return // live output observed before completion
		}
	}
	t.Fatalf("no live output event during the running command: %d events between call and result (%v)",
		len(between), between)
}
