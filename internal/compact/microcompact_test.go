package compact

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/model"
)

func TestMicrocompact(t *testing.T) {
	msgs := []model.Message{
		{
			ID:   "msg1",
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				model.ThinkingPart{Text: "Let me think about this..."},
				model.TextPart{Text: "I'll read the file."},
				model.ToolCallPart{
					ID:    "tc1",
					Name:  "FileRead",
					Input: json.RawMessage(`{"path":"/foo.go"}`),
				},
			},
		},
		{
			ID:   "msg2",
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.ToolResultPart{
					ToolCallID: "tc1",
					Content:    strings.Repeat("x", 1000), // large result
				},
			},
		},
		{
			ID:   "msg3",
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.ImagePart{MimeType: "image/png", Data: []byte{1, 2, 3}},
			},
		},
		{
			ID:   "msg4",
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.DocumentPart{MimeType: "application/pdf", Data: make([]byte, 5000)},
			},
		},
		{
			ID:   "msg5",
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.ToolResultPart{
					ToolCallID: "tc2",
					Content:    "short result", // small result, preserved
				},
			},
		},
	}

	result := Microcompact(msgs)

	// msg1: ThinkingPart stripped, TextPart + ToolCallPart preserved
	if len(result[0].Content) != 2 {
		t.Fatalf("msg1: expected 2 parts, got %d", len(result[0].Content))
	}
	if _, ok := result[0].Content[0].(model.TextPart); !ok {
		t.Error("msg1 part 0 should be TextPart")
	}
	if tc, ok := result[0].Content[1].(model.ToolCallPart); !ok {
		t.Error("msg1 part 1 should be ToolCallPart")
	} else if tc.Name != "FileRead" {
		t.Errorf("msg1 tool call name = %q, want FileRead", tc.Name)
	}

	// msg2: large tool result stubbed
	if len(result[1].Content) != 1 {
		t.Fatalf("msg2: expected 1 part, got %d", len(result[1].Content))
	}
	tr, ok := result[1].Content[0].(model.ToolResultPart)
	if !ok {
		t.Fatal("msg2 part 0 should be ToolResultPart")
	}
	if !strings.Contains(tr.Content, "1000 chars") {
		t.Errorf("stubbed result should mention char count, got: %s", tr.Content)
	}
	if !strings.Contains(tr.Content, "FileRead") {
		t.Errorf("stubbed result should mention tool name, got: %s", tr.Content)
	}

	// msg3: ImagePart replaced with text marker
	tp, ok := result[2].Content[0].(model.TextPart)
	if !ok {
		t.Fatal("msg3 part 0 should be TextPart (image marker)")
	}
	if !strings.Contains(tp.Text, "image/png") {
		t.Errorf("image marker should contain mime type, got: %s", tp.Text)
	}

	// msg4: DocumentPart replaced with text marker
	tp, ok = result[3].Content[0].(model.TextPart)
	if !ok {
		t.Fatal("msg4 part 0 should be TextPart (document marker)")
	}
	if !strings.Contains(tp.Text, "application/pdf") {
		t.Errorf("document marker should contain mime type, got: %s", tp.Text)
	}

	// msg5: small tool result preserved
	tr, ok = result[4].Content[0].(model.ToolResultPart)
	if !ok {
		t.Fatal("msg5 part 0 should be ToolResultPart")
	}
	if tr.Content != "short result" {
		t.Errorf("small result should be preserved, got: %s", tr.Content)
	}
}

func TestMicrocompactDeepCopy(t *testing.T) {
	original := []model.Message{
		{
			ID:   "msg1",
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				model.ToolCallPart{
					ID:    "tc1",
					Name:  "Bash",
					Input: json.RawMessage(`{"command":"ls"}`),
				},
			},
		},
	}

	result := Microcompact(original)

	// Mutate the result's input
	if tc, ok := result[0].Content[0].(model.ToolCallPart); ok {
		tc.Input[0] = 'X' // mutate copy
	}

	// Original should be unchanged
	origTC := original[0].Content[0].(model.ToolCallPart)
	if origTC.Input[0] != '{' {
		t.Error("Microcompact should deep copy — original was mutated")
	}
}

func TestMicrocompactEmptyConversation(t *testing.T) {
	result := Microcompact(nil)
	if len(result) != 0 {
		t.Errorf("expected empty result for nil input, got %d messages", len(result))
	}
}

func TestMicrocompactThinkingOnlyMessage(t *testing.T) {
	msgs := []model.Message{
		{
			ID:   "msg1",
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				model.ThinkingPart{Text: "only thinking, no text output"},
			},
		},
	}

	result := Microcompact(msgs)
	// Message with only thinking should be dropped (0 content parts after trim)
	if len(result) != 0 {
		t.Errorf("expected thinking-only message to be dropped, got %d messages", len(result))
	}
}
