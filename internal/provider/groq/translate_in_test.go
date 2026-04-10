package groq

import (
	"testing"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
)

func TestResponseFromWire_TextOnly(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()
	mapper := NewIDMapper()

	resp := &wireResponse{
		ID:    "chatcmpl-123",
		Model: "llama-3.3-70b-versatile",
		Choices: []wireChoice{{
			Index:        0,
			Message:      wireMessage{Role: "assistant", Content: "Hello!"},
			FinishReason: "stop",
		}},
		Usage: wireUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}

	result := responseFromWire(resp, mapper, bus)
	if result.Model != "llama-3.3-70b-versatile" {
		t.Errorf("model=%q", result.Model)
	}
	if result.StopReason != model.StopEndTurn {
		t.Errorf("stop_reason=%q, want end_turn", result.StopReason)
	}
	if len(result.Content) != 1 {
		t.Fatalf("got %d parts, want 1", len(result.Content))
	}
	tp, ok := result.Content[0].(model.TextPart)
	if !ok {
		t.Fatalf("part type=%T, want TextPart", result.Content[0])
	}
	if tp.Text != "Hello!" {
		t.Errorf("text=%q", tp.Text)
	}
}

func TestResponseFromWire_ToolCalls(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()
	mapper := NewIDMapper()

	resp := &wireResponse{
		Model: "llama-3.3-70b-versatile",
		Choices: []wireChoice{{
			Message: wireMessage{
				Role: "assistant",
				ToolCalls: []wireToolCall{{
					ID:   "call_abc",
					Type: "function",
					Function: wireFunction{
						Name:      "bash",
						Arguments: `{"cmd":"ls"}`,
					},
				}},
			},
			FinishReason: "tool_calls",
		}},
		Usage: wireUsage{PromptTokens: 10, CompletionTokens: 20},
	}

	result := responseFromWire(resp, mapper, bus)
	if result.StopReason != model.StopToolUse {
		t.Errorf("stop_reason=%q, want tool_use", result.StopReason)
	}
	if len(result.Content) != 1 {
		t.Fatalf("got %d parts, want 1", len(result.Content))
	}
	tc, ok := result.Content[0].(model.ToolCallPart)
	if !ok {
		t.Fatalf("part type=%T, want ToolCallPart", result.Content[0])
	}
	if tc.Name != "bash" {
		t.Errorf("name=%q", tc.Name)
	}
	if string(tc.Input) != `{"cmd":"ls"}` {
		t.Errorf("input=%q", string(tc.Input))
	}
	// Check ID mapper registration
	if mapper.ToInternal("call_abc") != tc.ID {
		t.Error("wire ID not registered in mapper")
	}
}

func TestResponseFromWire_WithReasoning(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()
	mapper := NewIDMapper()

	resp := &wireResponse{
		Model: "openai/gpt-oss-20b",
		Choices: []wireChoice{{
			Message: wireMessage{
				Role:      "assistant",
				Content:   "The answer is 42.",
				Reasoning: "Let me think step by step...",
			},
			FinishReason: "stop",
		}},
		Usage: wireUsage{PromptTokens: 10, CompletionTokens: 50},
	}

	result := responseFromWire(resp, mapper, bus)
	if len(result.Content) != 2 {
		t.Fatalf("got %d parts, want 2 (thinking + text)", len(result.Content))
	}
	// First: thinking
	thk, ok := result.Content[0].(model.ThinkingPart)
	if !ok {
		t.Fatalf("part[0] type=%T, want ThinkingPart", result.Content[0])
	}
	if thk.Text != "Let me think step by step..." {
		t.Errorf("thinking text=%q", thk.Text)
	}
	// Second: text
	tp, ok := result.Content[1].(model.TextPart)
	if !ok {
		t.Fatalf("part[1] type=%T, want TextPart", result.Content[1])
	}
	if tp.Text != "The answer is 42." {
		t.Errorf("text=%q", tp.Text)
	}
}

func TestResponseFromWire_NullContent(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()
	mapper := NewIDMapper()

	// When content is null (tool_calls only), Content field will be nil
	resp := &wireResponse{
		Model: "llama-3.3-70b-versatile",
		Choices: []wireChoice{{
			Message: wireMessage{
				Role:    "assistant",
				Content: nil, // null content
				ToolCalls: []wireToolCall{{
					ID:       "call_xyz",
					Type:     "function",
					Function: wireFunction{Name: "grep", Arguments: `{"pattern":"foo"}`},
				}},
			},
			FinishReason: "tool_calls",
		}},
	}

	result := responseFromWire(resp, mapper, bus)
	// Should only have tool call, no empty text part
	if len(result.Content) != 1 {
		t.Fatalf("got %d parts, want 1", len(result.Content))
	}
	if _, ok := result.Content[0].(model.ToolCallPart); !ok {
		t.Errorf("expected ToolCallPart, got %T", result.Content[0])
	}
}

func TestResponseFromWire_EmptyChoices(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()
	mapper := NewIDMapper()

	resp := &wireResponse{Model: "test", Choices: nil}
	result := responseFromWire(resp, mapper, bus)
	if result.StopReason != model.StopError {
		t.Errorf("stop_reason=%q, want error", result.StopReason)
	}
}

func TestUsageFromWire_WithCache(t *testing.T) {
	u := wireUsage{
		PromptTokens:        100,
		CompletionTokens:    50,
		TotalTokens:         150,
		PromptTokensDetails: &wireTokenDetail{CachedTokens: 80},
	}
	usage := usageFromWire(u)
	if usage.InputTokens != 100 {
		t.Errorf("input=%d, want 100", usage.InputTokens)
	}
	if usage.OutputTokens != 50 {
		t.Errorf("output=%d, want 50", usage.OutputTokens)
	}
	if usage.CacheReadInputTokens != 80 {
		t.Errorf("cache_read=%d, want 80", usage.CacheReadInputTokens)
	}
}

func TestStopReasonFromWire(t *testing.T) {
	tests := []struct {
		wire string
		want model.StopReason
	}{
		{"stop", model.StopEndTurn},
		{"tool_calls", model.StopToolUse},
		{"length", model.StopMaxTokens},
		{"unknown", model.StopError},
		{"", model.StopError},
	}
	for _, tt := range tests {
		got := stopReasonFromWire(tt.wire)
		if got != tt.want {
			t.Errorf("stopReasonFromWire(%q)=%q, want %q", tt.wire, got, tt.want)
		}
	}
}

func TestNormalizeArguments(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`{"key":"val"}`, `{"key":"val"}`},
		{"", "{}"},
		{"null", "{}"},
		{"invalid json", "{}"},
	}
	for _, tt := range tests {
		got := normalizeArguments(tt.input)
		if got != tt.want {
			t.Errorf("normalizeArguments(%q)=%q, want %q", tt.input, got, tt.want)
		}
	}
}
