package model

import (
	"encoding/json"
	"testing"
)

func TestContentPartRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		part ContentPart
	}{
		{
			name: "TextPart",
			part: TextPart{Text: "hello world"},
		},
		{
			name: "ImagePart",
			part: ImagePart{MimeType: "image/png", Data: []byte{0x89, 0x50, 0x4e, 0x47}},
		},
		{
			name: "ToolCallPart",
			part: ToolCallPart{
				ID:    "call-123",
				Name:  "Bash",
				Input: json.RawMessage(`{"command":"ls -la","timeout":5000}`),
			},
		},
		{
			name: "ToolResultPart",
			part: ToolResultPart{
				ToolCallID: "call-123",
				Content:    "file1.go\nfile2.go",
				IsError:    false,
			},
		},
		{
			name: "ToolResultPart/error",
			part: ToolResultPart{
				ToolCallID: "call-456",
				Content:    "command not found",
				IsError:    true,
			},
		},
		{
			name: "ThinkingPart",
			part: ThinkingPart{Text: "Let me think about this..."},
		},
		{
			name: "ThinkingPart/with_signature",
			part: ThinkingPart{Text: "reasoning trace", Signature: "sig_abc123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := MarshalContentPart(tt.part)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			got, err := UnmarshalContentPart(data)
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if got.PartType() != tt.part.PartType() {
				t.Errorf("PartType: got %q, want %q", got.PartType(), tt.part.PartType())
			}

			// Re-marshal and compare JSON to verify full round-trip
			data2, err := MarshalContentPart(got)
			if err != nil {
				t.Fatalf("re-marshal: %v", err)
			}
			if string(data) != string(data2) {
				t.Errorf("round-trip mismatch:\n  got:  %s\n  want: %s", data2, data)
			}
		})
	}
}

func TestContentPartDiscriminatorPresent(t *testing.T) {
	data, err := MarshalContentPart(TextPart{Text: "hi"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if _, ok := raw["type"]; !ok {
		t.Error("expected 'type' field in JSON output")
	}
	if _, ok := raw["data"]; !ok {
		t.Error("expected 'data' field in JSON output")
	}
}

func TestUnmarshalContentPartUnknownType(t *testing.T) {
	data := []byte(`{"type":"unknown","data":{}}`)
	_, err := UnmarshalContentPart(data)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestToolCallPartInputPreservesRawJSON(t *testing.T) {
	input := json.RawMessage(`{"nested":{"key":"value"},"array":[1,2,3]}`)
	part := ToolCallPart{ID: "x", Name: "test", Input: input}

	data, err := MarshalContentPart(part)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	got, err := UnmarshalContentPart(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	tcp := got.(ToolCallPart)
	// Compact both for comparison (JSON encoding may vary whitespace)
	var orig, restored json.RawMessage
	if err := json.Unmarshal(input, &orig); err != nil {
		t.Fatalf("compact original: %v", err)
	}
	if err := json.Unmarshal(tcp.Input, &restored); err != nil {
		t.Fatalf("compact restored: %v", err)
	}

	origBytes, _ := json.Marshal(orig)
	restoredBytes, _ := json.Marshal(restored)
	if string(origBytes) != string(restoredBytes) {
		t.Errorf("input not preserved:\n  got:  %s\n  want: %s", restoredBytes, origBytes)
	}
}

func TestMarshalUnmarshalContentParts(t *testing.T) {
	parts := []ContentPart{
		TextPart{Text: "hello"},
		ToolCallPart{ID: "c1", Name: "Bash", Input: json.RawMessage(`{"cmd":"ls"}`)},
		ThinkingPart{Text: "thinking..."},
	}

	data, err := MarshalContentParts(parts)
	if err != nil {
		t.Fatalf("marshal parts: %v", err)
	}

	got, err := UnmarshalContentParts(data)
	if err != nil {
		t.Fatalf("unmarshal parts: %v", err)
	}

	if len(got) != len(parts) {
		t.Fatalf("length: got %d, want %d", len(got), len(parts))
	}
	for i := range parts {
		if got[i].PartType() != parts[i].PartType() {
			t.Errorf("[%d] PartType: got %q, want %q", i, got[i].PartType(), parts[i].PartType())
		}
	}
}
