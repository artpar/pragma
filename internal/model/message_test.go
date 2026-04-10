package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMessageRoundTrip(t *testing.T) {
	msg := Message{
		ID:   "msg-1",
		Role: RoleAssistant,
		Content: []ContentPart{
			TextPart{Text: "Here's what I found:"},
			ToolCallPart{
				ID:    "tc-1",
				Name:  "FileRead",
				Input: json.RawMessage(`{"path":"/tmp/foo.go"}`),
			},
			ThinkingPart{Text: "Let me check the file..."},
		},
		Timestamp: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
		Flags:     MessageFlags{},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Message
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != msg.ID {
		t.Errorf("ID: got %q, want %q", got.ID, msg.ID)
	}
	if got.Role != msg.Role {
		t.Errorf("Role: got %q, want %q", got.Role, msg.Role)
	}
	if len(got.Content) != len(msg.Content) {
		t.Fatalf("Content length: got %d, want %d", len(got.Content), len(msg.Content))
	}
	for i := range msg.Content {
		if got.Content[i].PartType() != msg.Content[i].PartType() {
			t.Errorf("Content[%d] type: got %q, want %q", i, got.Content[i].PartType(), msg.Content[i].PartType())
		}
	}
	if !got.Timestamp.Equal(msg.Timestamp) {
		t.Errorf("Timestamp: got %v, want %v", got.Timestamp, msg.Timestamp)
	}
}

func TestMessageFlagsOmitEmpty(t *testing.T) {
	msg := Message{
		ID:        "msg-2",
		Role:      RoleUser,
		Content:   []ContentPart{TextPart{Text: "hi"}},
		Timestamp: time.Now(),
		Flags:     MessageFlags{},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}

	// With omitzero, zero-value MessageFlags must be omitted entirely
	if _, ok := raw["flags"]; ok {
		t.Errorf("expected 'flags' to be omitted when zero, but got: %s", raw["flags"])
	}
}

func TestUnmarshalMessageInvalidRole(t *testing.T) {
	data := []byte(`{"id":"m1","role":"system","content":[],"timestamp":"2026-04-10T00:00:00Z"}`)
	var msg Message
	if err := json.Unmarshal(data, &msg); err == nil {
		t.Error("expected error for invalid role 'system', got nil")
	}
}

func TestSystemPromptRoundTrip(t *testing.T) {
	sp := SystemPrompt{
		Blocks: []SystemBlock{
			{Text: "You are a helpful assistant.", Cacheable: true},
			{Text: "Additional context.", Cacheable: false},
		},
	}

	data, err := json.Marshal(sp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got SystemPrompt
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(got.Blocks) != len(sp.Blocks) {
		t.Fatalf("blocks length: got %d, want %d", len(got.Blocks), len(sp.Blocks))
	}
	for i := range sp.Blocks {
		if got.Blocks[i].Text != sp.Blocks[i].Text {
			t.Errorf("block[%d] text: got %q, want %q", i, got.Blocks[i].Text, sp.Blocks[i].Text)
		}
		if got.Blocks[i].Cacheable != sp.Blocks[i].Cacheable {
			t.Errorf("block[%d] cacheable: got %v, want %v", i, got.Blocks[i].Cacheable, sp.Blocks[i].Cacheable)
		}
	}
}
