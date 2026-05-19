package toolresult

import (
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/model"
)

func TestProcessToolResultPersistsLargeOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	part := model.ToolResultPart{ToolCallID: "toolu-1", Content: strings.Repeat("a", 30_001)}
	got, err := ProcessToolResult(part, "Bash", 30_000, "session-1")
	if err != nil {
		t.Fatalf("ProcessToolResult: %v", err)
	}

	if !strings.HasPrefix(got.Content, "<persisted-output") {
		t.Fatalf("expected persisted output marker, got %q", got.Content[:min(len(got.Content), 80)])
	}
	if !strings.Contains(got.Content, `tool_call_id="toolu-1"`) {
		t.Fatalf("expected tool_call_id attribute: %q", got.Content[:min(len(got.Content), 160)])
	}
	if !strings.Contains(got.Content, "Full output saved to:") {
		t.Fatalf("expected saved path in replacement: %q", got.Content)
	}
	if !strings.Contains(got.Content, "Preview (first 2KB):") {
		t.Fatalf("expected 2KB preview header: %q", got.Content)
	}
}

func TestProcessToolResultReadNeverPersists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	content := strings.Repeat("r", 80_000)
	part := model.ToolResultPart{ToolCallID: "read-1", Content: content}
	got, err := ProcessToolResult(part, "Read", -1, "session-1")
	if err != nil {
		t.Fatalf("ProcessToolResult: %v", err)
	}
	if got.Content != content {
		t.Fatal("Read output should not be persisted or replaced")
	}
}

func TestApplyToolResultBudgetReplacesLargestFreshResult(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	msgs := []model.Message{
		{
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "small", Name: "Bash"},
				model.ToolCallPart{ID: "large", Name: "Bash"},
			},
		},
		{
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.ToolResultPart{ToolCallID: "small", Content: strings.Repeat("s", 90_000)},
				model.ToolResultPart{ToolCallID: "large", Content: strings.Repeat("l", 130_000)},
			},
		},
	}

	state := NewContentReplacementState()
	got, records, err := ApplyToolResultBudget(msgs, state, "session-1", nil)
	if err != nil {
		t.Fatalf("ApplyToolResultBudget: %v", err)
	}
	if len(records) != 1 || records[0].ToolUseID != "large" {
		t.Fatalf("records = %#v, want one replacement for large", records)
	}

	user := got[1]
	small := user.Content[0].(model.ToolResultPart)
	large := user.Content[1].(model.ToolResultPart)
	if strings.HasPrefix(small.Content, "<persisted-output") {
		t.Fatal("smaller result should remain inline")
	}
	if !strings.HasPrefix(large.Content, "<persisted-output") {
		t.Fatal("larger result should be replaced")
	}

	reapplied, newRecords, err := ApplyToolResultBudget(msgs, state, "session-1", nil)
	if err != nil {
		t.Fatalf("reapply ApplyToolResultBudget: %v", err)
	}
	if len(newRecords) != 0 {
		t.Fatalf("reapply should not create new records: %#v", newRecords)
	}
	if reapplied[1].Content[1].(model.ToolResultPart).Content != large.Content {
		t.Fatal("replacement should be byte-identical on reapply")
	}
}

func TestApplyToolResultBudgetRetriesAfterPersistFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	msgs := []model.Message{
		{
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "small", Name: "Bash"},
				model.ToolCallPart{ID: "large", Name: "Bash"},
			},
		},
		{
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.ToolResultPart{ToolCallID: "small", Content: strings.Repeat("s", 90_000)},
				model.ToolResultPart{ToolCallID: "large", Content: strings.Repeat("l", 130_000)},
			},
		},
	}

	state := NewContentReplacementState()
	_, records, err := ApplyToolResultBudget(msgs, state, "", nil)
	if err == nil {
		t.Fatal("expected persistence error without session ID")
	}
	if len(records) != 0 {
		t.Fatalf("records = %#v, want none on persist failure", records)
	}
	if state.SeenIDs["large"] {
		t.Fatal("failed large result should remain retryable")
	}

	got, records, err := ApplyToolResultBudget(msgs, state, "session-1", nil)
	if err != nil {
		t.Fatalf("retry ApplyToolResultBudget: %v", err)
	}
	if len(records) != 1 || records[0].ToolUseID != "large" {
		t.Fatalf("retry records = %#v, want one replacement for large", records)
	}
	if !strings.HasPrefix(got[1].Content[1].(model.ToolResultPart).Content, "<persisted-output") {
		t.Fatal("retry should replace the large result")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
