package google

import (
	"testing"

	"github.com/artpar/gogent/internal/model"
)

func TestMergeConsecutiveRoles(t *testing.T) {
	msgs := []model.Message{
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "a"}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "b"}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: "c"}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: "d"}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "e"}}},
	}

	result := mergeConsecutiveRoles(msgs)

	if len(result) != 3 {
		t.Fatalf("len=%d, want 3", len(result))
	}
	if len(result[0].Content) != 2 {
		t.Errorf("first msg parts=%d, want 2 (merged users)", len(result[0].Content))
	}
	if len(result[1].Content) != 2 {
		t.Errorf("second msg parts=%d, want 2 (merged assistants)", len(result[1].Content))
	}
	if result[0].Role != model.RoleUser {
		t.Errorf("first role=%v", result[0].Role)
	}
	if result[1].Role != model.RoleAssistant {
		t.Errorf("second role=%v", result[1].Role)
	}
}

func TestStripEmptyTextParts(t *testing.T) {
	msgs := []model.Message{
		{Role: model.RoleUser, Content: []model.ContentPart{
			model.TextPart{Text: ""},
			model.TextPart{Text: "real"},
		}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{
			model.TextPart{Text: ""},
		}},
	}

	result := stripEmptyTextParts(msgs)

	if len(result) != 1 {
		t.Fatalf("len=%d, want 1 (empty assistant dropped)", len(result))
	}
	if len(result[0].Content) != 1 {
		t.Errorf("parts=%d, want 1", len(result[0].Content))
	}
}

func TestEnsureToolResultPairing_OrphanCall(t *testing.T) {
	msgs := []model.Message{
		{Role: model.RoleAssistant, Content: []model.ContentPart{
			model.ToolCallPart{ID: "tc-1", Name: "search"},
		}},
		// No following user message with tool result
	}

	result := ensureToolResultPairing(msgs)

	if len(result) != 2 {
		t.Fatalf("len=%d, want 2 (synthetic result injected)", len(result))
	}
	if result[1].Role != model.RoleUser {
		t.Error("second msg should be user")
	}
	tr, ok := result[1].Content[0].(model.ToolResultPart)
	if !ok {
		t.Fatal("should be ToolResultPart")
	}
	if !tr.IsError {
		t.Error("synthetic result should be error")
	}
}

func TestNormalizeMessages_EndToEnd(t *testing.T) {
	msgs := []model.Message{
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: ""}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hello"}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "world"}}},
	}

	result := normalizeMessages(msgs)

	// Empty text stripped → only 2 msgs, then merged → 1 msg
	if len(result) != 1 {
		t.Fatalf("len=%d, want 1", len(result))
	}
	if len(result[0].Content) != 2 {
		t.Errorf("parts=%d, want 2", len(result[0].Content))
	}
}
