package anthropic

import (
	"encoding/json"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/artpar/gogent/internal/model"
)

func TestContentBlockFromWireText(t *testing.T) {
	mapper := NewIDMapper()
	block := sdk.ContentBlockUnion{Type: "text", Text: "hello world"}
	part := contentBlockFromWire(block, mapper)
	tp, ok := part.(model.TextPart)
	if !ok {
		t.Fatalf("expected TextPart, got %T", part)
	}
	if tp.Text != "hello world" {
		t.Errorf("text: got %q", tp.Text)
	}
}

func TestContentBlockFromWireToolUse(t *testing.T) {
	mapper := NewIDMapper()
	block := sdk.ContentBlockUnion{
		Type:  "tool_use",
		ID:    "toolu_abc123",
		Name:  "Bash",
		Input: json.RawMessage(`{"cmd":"ls"}`),
	}
	part := contentBlockFromWire(block, mapper)
	tc, ok := part.(model.ToolCallPart)
	if !ok {
		t.Fatalf("expected ToolCallPart, got %T", part)
	}
	if tc.Name != "Bash" {
		t.Errorf("name: got %q", tc.Name)
	}
	if tc.ID == "" {
		t.Error("expected non-empty internal ID")
	}
	if tc.ID == "toolu_abc123" {
		t.Error("internal ID should NOT be the wire ID")
	}
	// Mapper should have the pair registered
	if mapper.ToWire(tc.ID) != "toolu_abc123" {
		t.Errorf("mapper ToWire: got %q, want %q", mapper.ToWire(tc.ID), "toolu_abc123")
	}
	if mapper.ToInternal("toolu_abc123") != tc.ID {
		t.Errorf("mapper ToInternal: got %q, want %q", mapper.ToInternal("toolu_abc123"), tc.ID)
	}
	// Input preserved
	var input map[string]string
	if err := json.Unmarshal(tc.Input, &input); err != nil {
		t.Fatalf("input unmarshal: %v", err)
	}
	if input["cmd"] != "ls" {
		t.Errorf("input: got %v", input)
	}
}

func TestContentBlockFromWireThinking(t *testing.T) {
	mapper := NewIDMapper()
	block := sdk.ContentBlockUnion{
		Type:      "thinking",
		Thinking:  "Let me think about this...",
		Signature: "sig_xyz",
	}
	part := contentBlockFromWire(block, mapper)
	tp, ok := part.(model.ThinkingPart)
	if !ok {
		t.Fatalf("expected ThinkingPart, got %T", part)
	}
	if tp.Text != "Let me think about this..." {
		t.Errorf("text: got %q", tp.Text)
	}
	if tp.Signature != "sig_xyz" {
		t.Errorf("signature: got %q, want %q", tp.Signature, "sig_xyz")
	}
}

func TestContentBlockFromWireRedactedThinking(t *testing.T) {
	mapper := NewIDMapper()
	block := sdk.ContentBlockUnion{Type: "redacted_thinking", Data: "encrypted_data"}
	part := contentBlockFromWire(block, mapper)
	tp, ok := part.(model.ThinkingPart)
	if !ok {
		t.Fatalf("expected ThinkingPart, got %T", part)
	}
	if tp.Text != "[redacted]" {
		t.Errorf("text: got %q, want %q", tp.Text, "[redacted]")
	}
	if tp.Signature != "" {
		t.Errorf("signature: got %q, want empty", tp.Signature)
	}
}

func TestContentBlockFromWireUnknown(t *testing.T) {
	mapper := NewIDMapper()
	block := sdk.ContentBlockUnion{Type: "web_search_tool_result"}
	part := contentBlockFromWire(block, mapper)
	if part != nil {
		t.Errorf("expected nil for unknown type, got %T", part)
	}
}

func TestStopReasonFromWire(t *testing.T) {
	tests := []struct {
		wire sdk.StopReason
		want model.StopReason
	}{
		{sdk.StopReasonEndTurn, model.StopEndTurn},
		{sdk.StopReasonToolUse, model.StopToolUse},
		{sdk.StopReasonMaxTokens, model.StopMaxTokens},
		{sdk.StopReasonStopSequence, model.StopEndTurn},
		{sdk.StopReasonPauseTurn, model.StopEndTurn},
		{sdk.StopReasonRefusal, model.StopError},
		{"unknown", model.StopError},
	}
	for _, tt := range tests {
		t.Run(string(tt.wire), func(t *testing.T) {
			got := stopReasonFromWire(tt.wire)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUsageFromWire(t *testing.T) {
	u := sdk.Usage{
		InputTokens:              1500,
		OutputTokens:             200,
		CacheCreationInputTokens: 100,
		CacheReadInputTokens:     50,
	}
	got := usageFromWire(u)
	if got.InputTokens != 1500 {
		t.Errorf("InputTokens: got %d", got.InputTokens)
	}
	if got.OutputTokens != 200 {
		t.Errorf("OutputTokens: got %d", got.OutputTokens)
	}
	if got.CacheCreationInputTokens != 100 {
		t.Errorf("CacheCreationInputTokens: got %d", got.CacheCreationInputTokens)
	}
	if got.CacheReadInputTokens != 50 {
		t.Errorf("CacheReadInputTokens: got %d", got.CacheReadInputTokens)
	}
}

func TestResponseFromWire(t *testing.T) {
	mapper := NewIDMapper()
	msg := &sdk.Message{
		ID:    "msg_123",
		Model: "claude-sonnet-4-20250514",
		Content: []sdk.ContentBlockUnion{
			{Type: "text", Text: "Hello!"},
		},
		StopReason: sdk.StopReasonEndTurn,
		Usage:      sdk.Usage{InputTokens: 10, OutputTokens: 5},
	}
	resp := responseFromWire(msg, mapper)
	if resp.ID != "msg_123" {
		t.Errorf("ID: got %q", resp.ID)
	}
	if resp.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model: got %q", resp.Model)
	}
	if resp.StopReason != model.StopEndTurn {
		t.Errorf("StopReason: got %q", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("Content length: got %d", len(resp.Content))
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("Usage.InputTokens: got %d", resp.Usage.InputTokens)
	}
}

