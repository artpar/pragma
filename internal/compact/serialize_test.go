package compact

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/gogent/internal/model"
)

func TestSerializeForCompaction(t *testing.T) {
	msgs := []model.Message{
		{
			ID:   "msg1",
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.TextPart{Text: "Hello, help me with Go"},
			},
		},
		{
			ID:   "msg2",
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				model.TextPart{Text: "I'll help you with that."},
				model.ToolCallPart{
					ID:    "tc1",
					Name:  "FileRead",
					Input: json.RawMessage(`{"path":"/main.go"}`),
				},
			},
		},
		{
			ID:   "msg3",
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.ToolResultPart{
					ToolCallID: "tc1",
					Content:    "package main\nfunc main() {}",
				},
			},
		},
	}

	result := SerializeForCompaction(msgs)

	if !strings.Contains(result, "[User]") {
		t.Error("should contain [User] label")
	}
	if !strings.Contains(result, "Hello, help me with Go") {
		t.Error("should contain user text")
	}
	if !strings.Contains(result, "[Assistant]") {
		t.Error("should contain [Assistant] label")
	}
	if !strings.Contains(result, "[Tool Call: FileRead(") {
		t.Error("should contain tool call")
	}
	if !strings.Contains(result, "package main") {
		t.Error("should contain tool result content")
	}
}

func TestSerializeInternalMessagesSkipped(t *testing.T) {
	msgs := []model.Message{
		{
			ID:   "msg1",
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.TextPart{Text: "visible"},
			},
		},
		{
			ID:   "msg2",
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				model.TextPart{Text: "internal stuff"},
			},
			Flags: model.MessageFlags{IsInternal: true},
		},
	}

	result := SerializeForCompaction(msgs)
	if strings.Contains(result, "internal stuff") {
		t.Error("internal messages should be skipped")
	}
}

func TestSerializeCompactSummaryLabel(t *testing.T) {
	msgs := []model.Message{
		{
			ID:   "msg1",
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.TextPart{Text: "Previous summary content"},
			},
			Flags: model.MessageFlags{IsCompactSummary: true},
		},
	}

	result := SerializeForCompaction(msgs)
	if !strings.Contains(result, "[Previous Compaction Summary]") {
		t.Error("compact summary should have special label")
	}
}

func TestSerializeToolError(t *testing.T) {
	msgs := []model.Message{
		{
			ID:   "msg1",
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.ToolResultPart{
					ToolCallID: "tc1",
					Content:    "permission denied",
					IsError:    true,
				},
			},
		},
	}

	result := SerializeForCompaction(msgs)
	if !strings.Contains(result, "[Tool Error]: permission denied") {
		t.Errorf("should format tool errors, got: %s", result)
	}
}

func TestSerializeLongToolInput(t *testing.T) {
	msgs := []model.Message{
		{
			ID:   "msg1",
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				model.ToolCallPart{
					ID:    "tc1",
					Name:  "FileWrite",
					Input: json.RawMessage(`{"path":"/very/long/path.go","content":"` + strings.Repeat("x", 500) + `"}`),
				},
			},
		},
	}

	result := SerializeForCompaction(msgs)
	if !strings.Contains(result, "...") {
		t.Error("long tool input should be truncated with ...")
	}
}

func TestSerializeEmptyConversation(t *testing.T) {
	result := SerializeForCompaction(nil)
	if result != "" {
		t.Errorf("empty conversation should produce empty string, got: %q", result)
	}
}
