package shared

import (
	"encoding/json"
	"testing"

	"github.com/artpar/pragma/internal/model"
)

func TestMarshalContent_TextOnly(t *testing.T) {
	parts := []model.ContentPart{
		model.TextPart{Text: "Hello, world!"},
	}
	raw := MarshalContent(parts)
	if raw == nil {
		t.Fatal("expected non-nil result")
	}

	// Should round-trip through UnmarshalContentParts
	roundTripped, err := model.UnmarshalContentParts(raw)
	if err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if len(roundTripped) != 1 {
		t.Fatalf("expected 1 part, got %d", len(roundTripped))
	}
	tp, ok := roundTripped[0].(model.TextPart)
	if !ok {
		t.Fatalf("expected TextPart, got %T", roundTripped[0])
	}
	if tp.Text != "Hello, world!" {
		t.Errorf("text: got %q, want %q", tp.Text, "Hello, world!")
	}
}

func TestMarshalContent_ToolCall(t *testing.T) {
	parts := []model.ContentPart{
		model.ToolCallPart{
			ID:    "tc-123",
			Name:  "Bash",
			Input: json.RawMessage(`{"cmd":"ls"}`),
		},
	}
	raw := MarshalContent(parts)
	if raw == nil {
		t.Fatal("expected non-nil result")
	}

	roundTripped, err := model.UnmarshalContentParts(raw)
	if err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if len(roundTripped) != 1 {
		t.Fatalf("expected 1 part, got %d", len(roundTripped))
	}
	tc, ok := roundTripped[0].(model.ToolCallPart)
	if !ok {
		t.Fatalf("expected ToolCallPart, got %T", roundTripped[0])
	}
	if tc.ID != "tc-123" {
		t.Errorf("id: got %q", tc.ID)
	}
	if tc.Name != "Bash" {
		t.Errorf("name: got %q", tc.Name)
	}
	var input map[string]string
	if err := json.Unmarshal(tc.Input, &input); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if input["cmd"] != "ls" {
		t.Errorf("input: got %v", input)
	}
}

func TestMarshalContent_Mixed(t *testing.T) {
	parts := []model.ContentPart{
		model.TextPart{Text: "I'll run that command"},
		model.ToolCallPart{
			ID:    "tc-456",
			Name:  "Read",
			Input: json.RawMessage(`{"path":"/tmp/file.txt"}`),
		},
	}
	raw := MarshalContent(parts)
	if raw == nil {
		t.Fatal("expected non-nil result")
	}

	roundTripped, err := model.UnmarshalContentParts(raw)
	if err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if len(roundTripped) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(roundTripped))
	}
	if _, ok := roundTripped[0].(model.TextPart); !ok {
		t.Errorf("part 0: expected TextPart, got %T", roundTripped[0])
	}
	if _, ok := roundTripped[1].(model.ToolCallPart); !ok {
		t.Errorf("part 1: expected ToolCallPart, got %T", roundTripped[1])
	}
}

func TestMarshalContent_Empty(t *testing.T) {
	raw := MarshalContent(nil)
	if raw != nil {
		t.Errorf("expected nil for nil parts, got %s", string(raw))
	}

	raw = MarshalContent([]model.ContentPart{})
	if raw != nil {
		t.Errorf("expected nil for zero-length parts, got %s", string(raw))
	}
}

func TestSystemText_Empty(t *testing.T) {
	s := SystemText(model.SystemPrompt{})
	if s != "" {
		t.Errorf("expected empty string, got %q", s)
	}
}

func TestSystemText_Single(t *testing.T) {
	s := SystemText(model.SystemPrompt{
		Blocks: []model.SystemBlock{{Text: "You are helpful."}},
	})
	if s != "You are helpful." {
		t.Errorf("got %q", s)
	}
}

func TestSystemText_Multiple(t *testing.T) {
	s := SystemText(model.SystemPrompt{
		Blocks: []model.SystemBlock{
			{Text: "Block 1"},
			{Text: "Block 2"},
			{Text: "Block 3"},
		},
	})
	expected := "Block 1\n\nBlock 2\n\nBlock 3"
	if s != expected {
		t.Errorf("got %q, want %q", s, expected)
	}
}
