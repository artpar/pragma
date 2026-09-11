package query

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// TOK-001 gates: a tool-free StopMaxTokens response is a truncated
// termination, not a completed turn. Baseline RED: the current loop emits
// TurnCompleteEvent and exits as success; candidate GREEN: ErrorEvent naming
// the truncation, partial content preserved for continuation.

func TestProviderToolsLoopErrorsOnToolFreeMaxTokensResponse(t *testing.T) {
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{
			Content:    []model.ContentPart{model.TextPart{Text: "partial final answ"}},
			StopReason: model.StopMaxTokens,
		},
	}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(16), EngineConfig{
		Model:     "test-model",
		LoopMode:  LoopModeProviderTools,
		MaxTokens: 4096,
		MaxTurns:  5,
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))

	var truncErr string
	var completed bool
	for _, ev := range events {
		switch e := ev.(type) {
		case ErrorEvent:
			truncErr = e.Err.Error()
		case TurnCompleteEvent:
			completed = true
		}
	}
	if truncErr == "" {
		t.Fatal("tool-free max_tokens truncation must terminate with an ErrorEvent")
	}
	if !strings.Contains(truncErr, "truncated") || !strings.Contains(truncErr, "max_tokens") {
		t.Fatalf("truncation error = %q, want it to name the truncation and max_tokens", truncErr)
	}
	if completed {
		t.Fatal("truncated termination must not be classified as a completed turn")
	}
	if prov.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", prov.calls)
	}
	msgs := store.Snapshot().Conversation.Messages
	if len(msgs) == 0 || msgs[len(msgs)-1].Role != model.RoleAssistant {
		t.Fatalf("conversation tail = %#v, want the partial assistant message preserved for continuation", msgs)
	}
	text, _ := msgs[len(msgs)-1].Content[0].(model.TextPart)
	if text.Text != "partial final answ" {
		t.Fatalf("preserved partial content = %#v, want the truncated text", msgs[len(msgs)-1].Content)
	}
}

func TestProviderToolsLoopErrorsOnReasoningOnlyMaxTokensResponse(t *testing.T) {
	// The recorded wire shape of the 2026-09-06 direct probes: finish_reason
	// length with content null and only reasoning, cut mid-thought.
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{
			Content:    []model.ContentPart{model.ThinkingPart{Text: "partial reasoning about the ta"}},
			StopReason: model.StopMaxTokens,
		},
	}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(16), EngineConfig{
		Model:     "test-model",
		LoopMode:  LoopModeProviderTools,
		MaxTokens: 4096,
		MaxTurns:  5,
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))

	var truncErr string
	var completed bool
	for _, ev := range events {
		switch e := ev.(type) {
		case ErrorEvent:
			truncErr = e.Err.Error()
		case TurnCompleteEvent:
			completed = true
		}
	}
	if truncErr == "" {
		t.Fatal("reasoning-only max_tokens truncation must terminate with an ErrorEvent")
	}
	if !strings.Contains(truncErr, "truncated") {
		t.Fatalf("truncation error = %q, want it to name the truncation", truncErr)
	}
	if completed {
		t.Fatal("truncated termination must not be classified as a completed turn")
	}
	msgs := store.Snapshot().Conversation.Messages
	if len(msgs) == 0 || msgs[len(msgs)-1].Role != model.RoleAssistant {
		t.Fatalf("conversation tail = %#v, want the partial assistant message preserved", msgs)
	}
	if _, ok := msgs[len(msgs)-1].Content[0].(model.ThinkingPart); !ok {
		t.Fatalf("preserved partial content = %#v, want the truncated reasoning", msgs[len(msgs)-1].Content)
	}
}

func TestProviderToolsLoopMaxTokensWithToolCallsContinues(t *testing.T) {
	// A truncated response that still carries a tool call is the model
	// working, not a terminal: the loop must execute the tool and continue.
	callInput, err := json.Marshal(map[string]string{"cmd": "printf continued-after-cut"})
	if err != nil {
		t.Fatal(err)
	}
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "call-cut-1", Name: "Bash", Input: callInput},
			},
			StopReason: model.StopMaxTokens,
		},
		{
			Content:    []model.ContentPart{model.TextPart{Text: "done"}},
			StopReason: model.StopEndTurn,
		},
	}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(16), EngineConfig{
		Model:     "test-model",
		LoopMode:  LoopModeProviderTools,
		MaxTokens: 4096,
		MaxTurns:  5,
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))

	var sawToolResult, sawComplete bool
	for _, ev := range events {
		switch e := ev.(type) {
		case ErrorEvent:
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		case ToolResultEvent:
			if strings.Contains(e.Result.Content, "continued-after-cut") {
				sawToolResult = true
			}
		case TurnCompleteEvent:
			sawComplete = true
		}
	}
	if !sawToolResult {
		t.Fatal("missing tool result after max_tokens response with a tool call")
	}
	if !sawComplete {
		t.Fatal("missing final TurnCompleteEvent")
	}
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", prov.calls)
	}
}

func TestPragmaLoopMaxTokensTextStillCompletes(t *testing.T) {
	// Adjacent mode check: the pragma loop has its own completion semantics
	// (no-action text is the final answer; FinalTextCheck governs rejection).
	// The provider-tools truncation classification must not leak into it.
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{
			Content:    []model.ContentPart{model.TextPart{Text: "partial final answ"}},
			StopReason: model.StopMaxTokens,
		},
	}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(16), EngineConfig{
		Model:     "test-model",
		MaxTokens: 4096,
		MaxTurns:  5,
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))

	var sawComplete bool
	for _, ev := range events {
		switch e := ev.(type) {
		case ErrorEvent:
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		case TurnCompleteEvent:
			sawComplete = true
			if e.StopReason != model.StopEndTurn {
				t.Fatalf("turn complete stop reason = %q, want %q", e.StopReason, model.StopEndTurn)
			}
		}
	}
	if !sawComplete {
		t.Fatal("pragma loop must still complete a no-action final turn")
	}
	if prov.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", prov.calls)
	}
}
