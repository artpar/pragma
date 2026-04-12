package google

import (
	"encoding/json"
	"testing"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
)

func TestResponseFromWire_TextOnly(t *testing.T) {
	mapper := NewIDMapper()
	bus := observe.NewEventBus(16)

	wire := &wireResponse{
		Candidates: []wireCandidate{
			{
				Content:      wireContent{Role: "model", Parts: []wirePart{{Text: "Hello!"}}},
				FinishReason: "STOP",
			},
		},
		UsageMetadata: wireUsageMetadata{
			PromptTokenCount:     10,
			CandidatesTokenCount: 5,
		},
	}

	resp := responseFromWire(wire, mapper, bus)

	if resp.StopReason != model.StopEndTurn {
		t.Errorf("stop=%v, want EndTurn", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content len=%d, want 1", len(resp.Content))
	}
	if tp, ok := resp.Content[0].(model.TextPart); !ok || tp.Text != "Hello!" {
		t.Errorf("content=%v", resp.Content[0])
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("usage=%+v", resp.Usage)
	}
}

func TestResponseFromWire_FunctionCallWithId(t *testing.T) {
	mapper := NewIDMapper()
	bus := observe.NewEventBus(16)

	wire := &wireResponse{
		Candidates: []wireCandidate{
			{
				Content: wireContent{
					Role: "model",
					Parts: []wirePart{
						{Text: "Let me search."},
						{FunctionCall: &wireFunctionCall{
							Name: "search",
							Args: json.RawMessage(`{"query":"go generics"}`),
							Id:   "abc123",
						}},
					},
				},
				FinishReason: "STOP",
			},
		},
		UsageMetadata: wireUsageMetadata{PromptTokenCount: 20, CandidatesTokenCount: 15},
	}

	resp := responseFromWire(wire, mapper, bus)

	if resp.StopReason != model.StopToolUse {
		t.Errorf("stop=%v, want ToolUse", resp.StopReason)
	}

	tc, ok := resp.Content[1].(model.ToolCallPart)
	if !ok {
		t.Fatal("second part should be ToolCallPart")
	}
	if tc.Name != "search" {
		t.Errorf("name=%q", tc.Name)
	}
	// The Gemini-provided id "abc123" should be the wire ID in the mapper
	wireID := mapper.ToWire(tc.ID)
	if wireID != "abc123" {
		t.Errorf("wire ID=%q, want abc123", wireID)
	}
}

func TestResponseFromWire_FunctionCallWithoutId(t *testing.T) {
	mapper := NewIDMapper()
	bus := observe.NewEventBus(16)

	wire := &wireResponse{
		Candidates: []wireCandidate{
			{
				Content: wireContent{
					Role: "model",
					Parts: []wirePart{
						{FunctionCall: &wireFunctionCall{
							Name: "read",
							Args: json.RawMessage(`{"path":"foo.txt"}`),
							// No Id — older model
						}},
					},
				},
				FinishReason: "STOP",
			},
		},
		UsageMetadata: wireUsageMetadata{},
	}

	resp := responseFromWire(wire, mapper, bus)

	tc, ok := resp.Content[0].(model.ToolCallPart)
	if !ok {
		t.Fatal("should be ToolCallPart")
	}
	// Should have a synthetic wire ID
	wireID := mapper.ToWire(tc.ID)
	if wireID == "" {
		t.Error("should have a wire ID even without Google-provided id")
	}
}

func TestResponseFromWire_ThinkingWithSignature(t *testing.T) {
	mapper := NewIDMapper()
	bus := observe.NewEventBus(16)
	thought := true

	wire := &wireResponse{
		Candidates: []wireCandidate{
			{
				Content: wireContent{
					Role: "model",
					Parts: []wirePart{
						{Text: "thinking...", Thought: &thought, ThoughtSignature: "sig_abc"},
						{Text: "Answer."},
					},
				},
				FinishReason: "STOP",
			},
		},
		UsageMetadata: wireUsageMetadata{},
	}

	resp := responseFromWire(wire, mapper, bus)

	if len(resp.Content) != 2 {
		t.Fatalf("content len=%d, want 2", len(resp.Content))
	}

	tp, ok := resp.Content[0].(model.ThinkingPart)
	if !ok {
		t.Fatalf("first part should be ThinkingPart, got %T", resp.Content[0])
	}
	if tp.Signature != "sig_abc" {
		t.Errorf("signature=%q, want sig_abc", tp.Signature)
	}
}

func TestResponseFromWire_NoCandidates(t *testing.T) {
	mapper := NewIDMapper()
	bus := observe.NewEventBus(16)

	wire := &wireResponse{}
	resp := responseFromWire(wire, mapper, bus)

	if resp.StopReason != model.StopError {
		t.Errorf("stop=%v, want Error", resp.StopReason)
	}
}

func TestStopReasonFromWire(t *testing.T) {
	tests := []struct {
		reason string
		want   model.StopReason
	}{
		{"STOP", model.StopEndTurn},
		{"MAX_TOKENS", model.StopMaxTokens},
		{"SAFETY", model.StopError},
		{"RECITATION", model.StopError},
		{"BLOCKLIST", model.StopError},
		{"MALFORMED_FUNCTION_CALL", model.StopError},
		{"OTHER", model.StopError},
		{"", model.StopError},
	}

	for _, tt := range tests {
		got := stopReasonFromWire(tt.reason)
		if got != tt.want {
			t.Errorf("stopReasonFromWire(%q)=%v, want %v", tt.reason, got, tt.want)
		}
	}
}

func TestUsageFromWire_ThinkingTokensFolded(t *testing.T) {
	u := wireUsageMetadata{
		PromptTokenCount:        100,
		CandidatesTokenCount:    50,
		ThoughtsTokenCount:      200,
		CachedContentTokenCount: 30,
	}

	usage := usageFromWire(u)

	if usage.InputTokens != 100 {
		t.Errorf("input=%d, want 100", usage.InputTokens)
	}
	if usage.OutputTokens != 250 {
		t.Errorf("output=%d, want 250 (50 candidates + 200 thinking)", usage.OutputTokens)
	}
	if usage.CacheReadInputTokens != 30 {
		t.Errorf("cache=%d, want 30", usage.CacheReadInputTokens)
	}
}

func TestNormalizeArguments(t *testing.T) {
	bus := observe.NewEventBus(16)

	tests := []struct {
		name string
		args json.RawMessage
		want string
	}{
		{"valid", json.RawMessage(`{"key":"value"}`), `{"key":"value"}`},
		{"null", json.RawMessage("null"), "{}"},
		{"empty", json.RawMessage(""), "{}"},
		{"invalid", json.RawMessage("{broken"), "{}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeArguments(tt.args, bus)
			if string(got) != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}
