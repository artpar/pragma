package groq

import (
	"testing"

	"github.com/artpar/gogent/internal/model"
)

func TestFilterEmpty(t *testing.T) {
	msgs := []model.Message{
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hello"}}},
		{Role: model.RoleAssistant, Content: nil},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "world"}}},
	}
	out := filterEmpty(msgs)
	if len(out) != 2 {
		t.Errorf("got %d messages, want 2", len(out))
	}
}

func TestStripEmptyTextParts(t *testing.T) {
	msgs := []model.Message{
		{
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				model.TextPart{Text: ""},
				model.TextPart{Text: "real content"},
			},
		},
	}
	out := stripEmptyTextParts(msgs)
	if len(out) != 1 {
		t.Fatalf("got %d messages, want 1", len(out))
	}
	if len(out[0].Content) != 1 {
		t.Errorf("got %d parts, want 1", len(out[0].Content))
	}
}

func TestStripEmptyTextParts_RemovesEmptyMessage(t *testing.T) {
	msgs := []model.Message{
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: ""}}},
	}
	out := stripEmptyTextParts(msgs)
	if len(out) != 0 {
		t.Errorf("got %d messages, want 0", len(out))
	}
}

func TestEnsureToolResultPairing_OrphanCall(t *testing.T) {
	msgs := []model.Message{
		{
			Role:    model.RoleAssistant,
			Content: []model.ContentPart{model.ToolCallPart{ID: "tc1", Name: "bash"}},
		},
		{
			Role:    model.RoleUser,
			Content: []model.ContentPart{model.TextPart{Text: "next message"}},
		},
	}
	out := ensureToolResultPairing(msgs)
	// User message should have synthetic tool result injected
	if len(out) != 2 {
		t.Fatalf("got %d messages, want 2", len(out))
	}
	userMsg := out[1]
	found := false
	for _, p := range userMsg.Content {
		if tr, ok := p.(model.ToolResultPart); ok && tr.ToolCallID == "tc1" {
			found = true
			if !tr.IsError {
				t.Error("synthetic result should be error")
			}
		}
	}
	if !found {
		t.Error("expected synthetic tool result for orphan call")
	}
}

func TestEnsureToolResultPairing_OrphanResult(t *testing.T) {
	msgs := []model.Message{
		{
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.ToolResultPart{ToolCallID: "orphan", Content: "result"},
				model.TextPart{Text: "text"},
			},
		},
	}
	out := ensureToolResultPairing(msgs)
	if len(out) != 1 {
		t.Fatalf("got %d messages, want 1", len(out))
	}
	// Orphan result should be dropped, text kept
	for _, p := range out[0].Content {
		if _, ok := p.(model.ToolResultPart); ok {
			t.Error("orphan tool result should be removed")
		}
	}
}

func TestEnsureToolResultPairing_TrailingAssistant(t *testing.T) {
	// Conversation ends with assistant tool calls and no following user message
	msgs := []model.Message{
		{
			Role:    model.RoleAssistant,
			Content: []model.ContentPart{model.ToolCallPart{ID: "tc1", Name: "bash"}},
		},
	}
	out := ensureToolResultPairing(msgs)
	// Should inject a synthetic user message with error result
	if len(out) != 2 {
		t.Fatalf("got %d messages, want 2 (assistant + synthetic user)", len(out))
	}
	if out[1].Role != model.RoleUser {
		t.Errorf("second message role=%q, want user", out[1].Role)
	}
	found := false
	for _, p := range out[1].Content {
		if tr, ok := p.(model.ToolResultPart); ok && tr.ToolCallID == "tc1" {
			found = true
			if !tr.IsError {
				t.Error("synthetic result should be error")
			}
		}
	}
	if !found {
		t.Error("expected synthetic tool result for trailing assistant call")
	}
}

func TestMergeConsecutiveUser(t *testing.T) {
	msgs := []model.Message{
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "a"}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "b"}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: "c"}}},
	}
	out := mergeConsecutiveUser(msgs)
	if len(out) != 2 {
		t.Fatalf("got %d messages, want 2", len(out))
	}
	if len(out[0].Content) != 2 {
		t.Errorf("merged user message has %d parts, want 2", len(out[0].Content))
	}
}
