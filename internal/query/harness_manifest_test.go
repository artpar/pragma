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

func systemTextOf(sp model.SystemPrompt) string {
	var b strings.Builder
	for _, blk := range sp.Blocks {
		b.WriteString(blk.Text)
	}
	return b.String()
}

func manifestToolUseResponses(n int) []model.Response {
	input, err := json.Marshal(map[string]string{"cmd": "true"})
	if err != nil {
		panic(err)
	}
	responses := make([]model.Response, 0, n)
	for i := 0; i < n; i++ {
		responses = append(responses, model.Response{
			ID:      model.NewUUID(),
			Model:   "test-model",
			Content: []model.ContentPart{
				model.ToolCallPart{ID: fmt.Sprintf("call-mb-%d", i), Name: "Bash", Input: input},
			},
			StopReason: model.StopToolUse,
			Usage:   model.TokenUsage{InputTokens: 10, OutputTokens: 10},
		})
	}
	return responses
}

func countOutputBudgetNotices(msgs []model.Message) int {
	count := 0
	for _, msg := range msgs {
		if msg.Role != model.RoleUser {
			continue
		}
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok && strings.Contains(tp.Text, outputBudgetNoticeMarker) {
				count++
			}
		}
	}
	return count
}

// HMB-001: the first request carries the harness manifest — the model must
// learn identity and budget facts from the system prompt, not tool shapes.
func TestProviderToolsLoopHarnessManifestInFirstRequest(t *testing.T) {
	prov := &pragmaLoopTestProvider{responses: manifestToolUseResponses(3)}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          t.TempDir(),
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    512,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(64), EngineConfig{
		Model:     "test-model",
		LoopMode:  LoopModeProviderTools,
		MaxTokens: 512,
		MaxTurns:  3,
	})
	collectPragmaLoopEvents(engine.Run(t.Context(), "work"))
	if prov.calls == 0 {
		t.Fatal("loop made no provider calls")
	}
	text := systemTextOf(prov.requests[0].System)
	for _, want := range []string{
		"harness: pragma",
		"provider: test",
		"model: test-model",
		"loop: " + LoopModeProviderTools,
		"output_token_budget_per_turn: 512",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("first-request system missing %q:\n%s", want, text)
		}
	}
}

// HMB-002: the turn after a response that consumed ≥ half the output budget
// carries an output-budget notice with the consumed count.
func TestProviderToolsLoopOutputBudgetNoticeAfterHeavyTurn(t *testing.T) {
	responses := manifestToolUseResponses(3)
	responses[0].Usage.OutputTokens = 300 // ≥ 512/2
	prov := &pragmaLoopTestProvider{responses: responses}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          t.TempDir(),
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    512,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(64), EngineConfig{
		Model:     "test-model",
		LoopMode:  LoopModeProviderTools,
		MaxTokens: 512,
		MaxTurns:  3,
	})
	collectPragmaLoopEvents(engine.Run(t.Context(), "work"))
	if prov.calls != 3 {
		t.Fatalf("provider calls = %d, want 3", prov.calls)
	}
	if n := countOutputBudgetNotices(prov.requests[0].Messages); n != 0 {
		t.Fatalf("first request carries %d budget notices, want 0", n)
	}
	if n := countOutputBudgetNotices(prov.requests[1].Messages); n != 1 {
		t.Fatalf("second request carries %d budget notices, want 1", n)
	}
	var noticeText string
	for _, msg := range prov.requests[1].Messages {
		if msg.Role != model.RoleUser {
			continue
		}
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok && strings.Contains(tp.Text, outputBudgetNoticeMarker) {
				noticeText = tp.Text
			}
		}
	}
	if !strings.Contains(noticeText, "300 of 512") {
		t.Fatalf("notice lacks consumed count: %q", noticeText)
	}
}

