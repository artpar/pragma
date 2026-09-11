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

// TURN-001 gates: the provider-tools loop must warn the model in-conversation
// before the turn budget is exhausted; the cap itself is unchanged; pragma
// loop mode is untouched.

func turnBudgetToolUseResponses(n int) []model.Response {
	input, err := json.Marshal(map[string]string{"cmd": "true"})
	if err != nil {
		panic(err)
	}
	responses := make([]model.Response, 0, n)
	for i := 0; i < n; i++ {
		responses = append(responses, model.Response{
			Content: []model.ContentPart{
				model.ToolCallPart{ID: fmt.Sprintf("call-turn-%d", i), Name: "Bash", Input: input},
			},
			StopReason: model.StopToolUse,
		})
	}
	return responses
}

func TestTurnBudgetWarnTurnWindow(t *testing.T) {
	for _, tc := range []struct {
		maxTurns, want int
	}{
		{100, 90}, {50, 45}, {20, 10}, {12, 6}, {8, 4}, {6, 3}, {2, 1}, {1, -1}, {0, -1},
	} {
		if got := turnBudgetWarnTurn(tc.maxTurns); got != tc.want {
			t.Fatalf("turnBudgetWarnTurn(%d) = %d, want %d", tc.maxTurns, got, tc.want)
		}
	}
}

func countTurnBudgetNotices(msgs []model.Message) int {
	count := 0
	for _, msg := range msgs {
		if msg.Role != model.RoleUser {
			continue
		}
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok && strings.Contains(tp.Text, turnBudgetNoticeMarker) {
				count++
			}
		}
	}
	return count
}

func TestProviderToolsLoopTurnBudgetNoticeBeforeCap(t *testing.T) {
	const maxTurns = 20
	prov := &pragmaLoopTestProvider{responses: turnBudgetToolUseResponses(maxTurns)}
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
		MaxTurns:  maxTurns,
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "work"))

	// Cap enforcement (must hold on baseline AND candidate): the loop still
	// terminates with the turn-limit error after exactly maxTurns requests.
	var capErr string
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			capErr = e.Err.Error()
		}
	}
	if !strings.Contains(capErr, "exceeded maximum of 20 turns") {
		t.Fatalf("loop termination error = %q, want turn-limit error", capErr)
	}
	if prov.calls != maxTurns {
		t.Fatalf("provider calls = %d, want %d", prov.calls, maxTurns)
	}

	// Notice gate (candidate GREEN, baseline RED): requests before the warn
	// iteration carry no notice; the warn-iteration request (index 10 of 20,
	// half the budget remaining) carries exactly one naming the budget state;
	// later requests retain exactly one (the notice fires once).
	for i := 0; i < 10; i++ {
		if n := countTurnBudgetNotices(prov.requests[i].Messages); n != 0 {
			t.Fatalf("request %d carries %d notices, want 0", i, n)
		}
	}
	if n := countTurnBudgetNotices(prov.requests[10].Messages); n != 1 {
		t.Fatalf("warn-iteration request carries %d notices, want 1", n)
	}
	if n := countTurnBudgetNotices(prov.requests[19].Messages); n != 1 {
		t.Fatalf("final request carries %d notices, want exactly 1 (notice must fire once)", n)
	}
	var noticeText string
	for _, msg := range prov.requests[10].Messages {
		if msg.Role != model.RoleUser {
			continue
		}
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok && strings.Contains(tp.Text, turnBudgetNoticeMarker) {
				noticeText = tp.Text
			}
		}
	}
	if !strings.Contains(noticeText, "10 of 20") || !strings.Contains(noticeText, "10 remain") {
		t.Fatalf("notice text = %q, want budget state \"10 of 20\" and \"10 remain\"", noticeText)
	}
}

// Adjacent check: pragma loop mode gains no turn-budget notice (its budget
// mechanism is a separate case if one is ever evidenced).
func TestPragmaLoopNoTurnBudgetNotice(t *testing.T) {
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{Content: []model.ContentPart{model.TextPart{Text: "done"}}, StopReason: model.StopEndTurn},
	}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          t.TempDir(),
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(16), EngineConfig{
		Model:     "test-model",
		MaxTokens: 4096,
		MaxTurns:  5,
	})

	collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))

	if prov.calls == 0 {
		t.Fatal("pragma loop did not call the provider")
	}
	for i, req := range prov.requests {
		if n := countTurnBudgetNotices(req.Messages); n != 0 {
			t.Fatalf("pragma-mode request %d carries %d turn-budget notices, want 0", i, n)
		}
	}
}
