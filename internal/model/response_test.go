package model

import (
	"encoding/json"
	"testing"
)

func TestResponseRoundTrip(t *testing.T) {
	resp := Response{
		ID:    "resp-1",
		Model: "claude-sonnet-4-20250514",
		Content: []ContentPart{
			TextPart{Text: "Here's the result:"},
			ToolCallPart{
				ID:    "tc-1",
				Name:  "Bash",
				Input: json.RawMessage(`{"command":"ls"}`),
			},
		},
		StopReason: StopToolUse,
		Usage: TokenUsage{
			InputTokens:  1500,
			OutputTokens: 200,
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Response
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != resp.ID {
		t.Errorf("ID: got %q, want %q", got.ID, resp.ID)
	}
	if got.Model != resp.Model {
		t.Errorf("Model: got %q, want %q", got.Model, resp.Model)
	}
	if got.StopReason != resp.StopReason {
		t.Errorf("StopReason: got %q, want %q", got.StopReason, resp.StopReason)
	}
	if len(got.Content) != len(resp.Content) {
		t.Fatalf("Content length: got %d, want %d", len(got.Content), len(resp.Content))
	}
	if got.Content[0].PartType() != ContentText {
		t.Errorf("Content[0] type: got %q, want %q", got.Content[0].PartType(), ContentText)
	}
	if got.Content[1].PartType() != ContentToolCall {
		t.Errorf("Content[1] type: got %q, want %q", got.Content[1].PartType(), ContentToolCall)
	}
	if got.Usage.InputTokens != 1500 {
		t.Errorf("InputTokens: got %d, want 1500", got.Usage.InputTokens)
	}
}

func TestResponseEndTurn(t *testing.T) {
	resp := Response{
		ID:    "resp-2",
		Model: "gpt-4",
		Content: []ContentPart{
			TextPart{Text: "Done."},
		},
		StopReason: StopEndTurn,
		Usage:      TokenUsage{InputTokens: 100, OutputTokens: 10},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Response
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.StopReason != StopEndTurn {
		t.Errorf("StopReason: got %q, want %q", got.StopReason, StopEndTurn)
	}
}
