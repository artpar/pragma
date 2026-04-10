package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/artpar/gogent/internal/model"
)

func msg(role model.Role, parts ...model.ContentPart) model.Message {
	return model.Message{ID: "test", Role: role, Content: parts}
}

func TestFilterEmpty(t *testing.T) {
	msgs := []model.Message{
		msg(model.RoleUser, model.TextPart{Text: "hello"}),
		msg(model.RoleAssistant), // empty
		msg(model.RoleUser, model.TextPart{Text: "world"}),
	}
	got := filterEmpty(msgs)
	if len(got) != 2 {
		t.Fatalf("length: got %d, want 2", len(got))
	}
}

func TestMergeConsecutiveSameRole(t *testing.T) {
	msgs := []model.Message{
		msg(model.RoleUser, model.TextPart{Text: "a"}),
		msg(model.RoleUser, model.TextPart{Text: "b"}),
		msg(model.RoleAssistant, model.TextPart{Text: "c"}),
		msg(model.RoleAssistant, model.TextPart{Text: "d"}),
		msg(model.RoleUser, model.TextPart{Text: "e"}),
	}
	got := mergeConsecutiveSameRole(msgs)
	if len(got) != 3 {
		t.Fatalf("length: got %d, want 3", len(got))
	}
	// First merged user
	if len(got[0].Content) != 2 {
		t.Errorf("first user content: got %d parts, want 2", len(got[0].Content))
	}
	// Merged assistant
	if len(got[1].Content) != 2 {
		t.Errorf("assistant content: got %d parts, want 2", len(got[1].Content))
	}
	// Final user
	if len(got[2].Content) != 1 {
		t.Errorf("last user content: got %d parts, want 1", len(got[2].Content))
	}
}

func TestMergeAlreadyAlternating(t *testing.T) {
	msgs := []model.Message{
		msg(model.RoleUser, model.TextPart{Text: "a"}),
		msg(model.RoleAssistant, model.TextPart{Text: "b"}),
		msg(model.RoleUser, model.TextPart{Text: "c"}),
	}
	got := mergeConsecutiveSameRole(msgs)
	if len(got) != 3 {
		t.Fatalf("length: got %d, want 3", len(got))
	}
}

func TestEnsureToolResultPairingOrphanedCall(t *testing.T) {
	msgs := []model.Message{
		msg(model.RoleUser, model.TextPart{Text: "do something"}),
		msg(model.RoleAssistant,
			model.ToolCallPart{ID: "tc1", Name: "Bash", Input: json.RawMessage(`{}`)},
		),
		// No user message with tool result follows
	}
	got := ensureToolResultPairing(msgs)
	if len(got) != 3 {
		t.Fatalf("length: got %d, want 3 (synthetic user injected)", len(got))
	}
	// Last message should be synthetic user with tool result
	last := got[2]
	if last.Role != model.RoleUser {
		t.Errorf("synthetic message role: got %q", last.Role)
	}
	if len(last.Content) != 1 {
		t.Fatalf("synthetic content: got %d parts, want 1", len(last.Content))
	}
	tr, ok := last.Content[0].(model.ToolResultPart)
	if !ok {
		t.Fatalf("synthetic content type: got %T", last.Content[0])
	}
	if tr.ToolCallID != "tc1" {
		t.Errorf("ToolCallID: got %q", tr.ToolCallID)
	}
	if !tr.IsError {
		t.Error("expected IsError=true for synthetic result")
	}
}

func TestEnsureToolResultPairingMissingResult(t *testing.T) {
	msgs := []model.Message{
		msg(model.RoleUser, model.TextPart{Text: "go"}),
		msg(model.RoleAssistant,
			model.ToolCallPart{ID: "tc1", Name: "Bash", Input: json.RawMessage(`{}`)},
			model.ToolCallPart{ID: "tc2", Name: "Read", Input: json.RawMessage(`{}`)},
		),
		msg(model.RoleUser,
			model.ToolResultPart{ToolCallID: "tc1", Content: "ok"},
			// tc2 result missing
		),
	}
	got := ensureToolResultPairing(msgs)
	if len(got) != 3 {
		t.Fatalf("length: got %d, want 3", len(got))
	}
	// User message should now have both results
	userParts := got[2].Content
	resultCount := 0
	for _, p := range userParts {
		if _, ok := p.(model.ToolResultPart); ok {
			resultCount++
		}
	}
	if resultCount != 2 {
		t.Errorf("result count: got %d, want 2", resultCount)
	}
}

func TestEnsureToolResultPairingOrphanedResult(t *testing.T) {
	msgs := []model.Message{
		msg(model.RoleUser, model.TextPart{Text: "go"}),
		msg(model.RoleAssistant,
			model.ToolCallPart{ID: "tc1", Name: "Bash", Input: json.RawMessage(`{}`)},
		),
		msg(model.RoleUser,
			model.ToolResultPart{ToolCallID: "tc1", Content: "ok"},
			model.ToolResultPart{ToolCallID: "orphan", Content: "stale"}, // no matching call
		),
	}
	got := ensureToolResultPairing(msgs)
	userParts := got[2].Content
	for _, p := range userParts {
		if tr, ok := p.(model.ToolResultPart); ok && tr.ToolCallID == "orphan" {
			t.Error("orphaned tool result should have been removed")
		}
	}
}

func TestNormalizeMessagesFullPipeline(t *testing.T) {
	msgs := []model.Message{
		msg(model.RoleUser, model.TextPart{Text: "a"}),
		msg(model.RoleUser, model.TextPart{Text: "b"}), // consecutive, should merge
		msg(model.RoleAssistant), // empty, should be filtered
		msg(model.RoleAssistant,
			model.ToolCallPart{ID: "tc1", Name: "X", Input: json.RawMessage(`{}`)},
		),
		// No tool result — should be synthesized
	}
	got := normalizeMessages(msgs)
	// After filter: removes empty assistant
	// After merge: user a+b merged
	// After pairing: synthetic result for tc1
	if len(got) != 3 {
		t.Fatalf("length: got %d, want 3", len(got))
	}
	if got[0].Role != model.RoleUser {
		t.Errorf("msg[0] role: got %q", got[0].Role)
	}
	if got[1].Role != model.RoleAssistant {
		t.Errorf("msg[1] role: got %q", got[1].Role)
	}
	if got[2].Role != model.RoleUser {
		t.Errorf("msg[2] role: got %q", got[2].Role)
	}
}
