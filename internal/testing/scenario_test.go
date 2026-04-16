package testing

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/artpar/pragma/internal/model"
)

func scenariosDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "scenarios")
}

func TestRunAllScenarios(t *testing.T) {
	RunAllScenarios(t, scenariosDir())
}

func TestHarnessAssertEvent(t *testing.T) {
	// Use a tool scenario so the orchestrator emits events to the local bus.
	h := NewHarness().
		WithTool("ping", func(_ json.RawMessage) (string, error) {
			return "pong", nil
		}).
		WithProviderResponses(
			model.Response{
				StopReason: model.StopToolUse,
				Content: []model.ContentPart{model.ToolCallPart{
					ID: "tc-assert-1", Name: "ping", Input: json.RawMessage(`{}`),
				}},
				Usage: model.TokenUsage{InputTokens: 50, OutputTokens: 10},
			},
			model.Response{
				StopReason: model.StopEndTurn,
				Content:    []model.ContentPart{model.TextPart{Text: "done"}},
				Usage:      model.TokenUsage{InputTokens: 80, OutputTokens: 10},
			},
		)

	_, err := h.Run(context.Background(), "ping")
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}

	// ToolCallReceived is emitted by orchestrator to the local bus.
	if err := h.AssertEvent("ToolCallReceived", nil); err != nil {
		t.Errorf("expected ToolCallReceived event: %v", err)
	}

	// No compaction events should exist.
	if err := h.AssertNoEvent("CompactionStarted"); err != nil {
		t.Errorf("unexpected CompactionStarted: %v", err)
	}

	// Non-existent event kind.
	if err := h.AssertEvent("NonExistentKind", nil); err == nil {
		t.Error("expected error for non-existent event kind")
	}
}

func TestHarnessAssertNoEvent(t *testing.T) {
	// Use a tool scenario so we have events to check against.
	h := NewHarness().
		WithTool("ping", func(_ json.RawMessage) (string, error) {
			return "pong", nil
		}).
		WithProviderResponses(
			model.Response{
				StopReason: model.StopToolUse,
				Content: []model.ContentPart{model.ToolCallPart{
					ID: "tc-no-1", Name: "ping", Input: json.RawMessage(`{}`),
				}},
				Usage: model.TokenUsage{InputTokens: 50, OutputTokens: 10},
			},
			model.Response{
				StopReason: model.StopEndTurn,
				Content:    []model.ContentPart{model.TextPart{Text: "done"}},
				Usage:      model.TokenUsage{InputTokens: 80, OutputTokens: 10},
			},
		)

	_, _ = h.Run(context.Background(), "ping")

	// No compaction events should exist.
	if err := h.AssertNoEvent("CompactionStarted"); err != nil {
		t.Errorf("unexpected CompactionStarted: %v", err)
	}

	// ToolCallReceived should exist, so AssertNoEvent should fail.
	if err := h.AssertNoEvent("ToolCallReceived"); err == nil {
		t.Error("expected error because ToolCallReceived events exist")
	}
}

func TestHarnessAssertEventSequence(t *testing.T) {
	h := NewHarness().
		WithTool("echo", func(input json.RawMessage) (string, error) {
			return "echoed", nil
		}).
		WithProviderResponses(
			model.Response{
				StopReason: model.StopToolUse,
				Content: []model.ContentPart{model.ToolCallPart{
					ID:    "tc-seq-1",
					Name:  "echo",
					Input: json.RawMessage(`{}`),
				}},
				Usage: model.TokenUsage{InputTokens: 50, OutputTokens: 10},
			},
			model.Response{
				StopReason: model.StopEndTurn,
				Content:    []model.ContentPart{model.TextPart{Text: "done"}},
				Usage:      model.TokenUsage{InputTokens: 80, OutputTokens: 15},
			},
		)

	_, err := h.Run(context.Background(), "call echo")
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}

	// Verify tool lifecycle sequence.
	if err := h.AssertEventSequence(
		"ToolCallReceived",
		"ToolBatchStarted",
		"ToolPermissionChecked",
		"ToolExecutionStarted",
		"ToolExecutionCompleted",
		"ToolBatchCompleted",
	); err != nil {
		t.Errorf("event sequence: %v", err)
	}

	// Bad sequence should fail.
	if err := h.AssertEventSequence("ToolBatchCompleted", "ToolBatchStarted"); err == nil {
		t.Error("expected error for reversed sequence")
	}
}

func TestHarnessProviderCalls(t *testing.T) {
	h := NewHarness().WithProviderResponses(model.Response{
		StopReason: model.StopEndTurn,
		Content:    []model.ContentPart{model.TextPart{Text: "hello"}},
		Usage:      model.TokenUsage{InputTokens: 10, OutputTokens: 5},
	})

	_, _ = h.Run(context.Background(), "hi")

	calls := h.ProviderCalls()
	if len(calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(calls))
	}
	if calls[0].Model != "test-model" {
		t.Errorf("model = %q, want %q", calls[0].Model, "test-model")
	}
}

func TestHarnessPermissionDeny(t *testing.T) {
	h := NewHarness().
		WithPermissionDeny("BadTool").
		WithTool("BadTool", func(_ json.RawMessage) (string, error) {
			return "should not run", nil
		}).
		WithProviderResponses(
			model.Response{
				StopReason: model.StopToolUse,
				Content: []model.ContentPart{model.ToolCallPart{
					ID:    "tc-deny-1",
					Name:  "BadTool",
					Input: json.RawMessage(`{}`),
				}},
				Usage: model.TokenUsage{InputTokens: 50, OutputTokens: 10},
			},
			model.Response{
				StopReason: model.StopEndTurn,
				Content:    []model.ContentPart{model.TextPart{Text: "denied"}},
				Usage:      model.TokenUsage{InputTokens: 80, OutputTokens: 10},
			},
		)

	_, _ = h.Run(context.Background(), "run bad tool")

	if err := h.AssertEventField("ToolPermissionChecked", "decision", "deny"); err != nil {
		t.Errorf("permission check: %v", err)
	}
	if err := h.AssertEventField("PermissionDenialEnforced", "was_executed", "false"); err != nil {
		t.Errorf("denial enforced: %v", err)
	}
	if err := h.AssertNoEvent("ToolExecutionStarted"); err != nil {
		t.Errorf("tool should not have executed: %v", err)
	}
}

func TestHarnessMaxTurns(t *testing.T) {
	// Provider always returns tool_use — engine should stop after 2 turns.
	h := NewHarness().
		WithMaxTurns(2).
		WithTool("Loop", func(_ json.RawMessage) (string, error) {
			return "again", nil
		}).
		WithProviderResponses(
			model.Response{
				StopReason: model.StopToolUse,
				Content: []model.ContentPart{model.ToolCallPart{
					ID: "tc-loop-1", Name: "Loop", Input: json.RawMessage(`{}`),
				}},
				Usage: model.TokenUsage{InputTokens: 50, OutputTokens: 10},
			},
			model.Response{
				StopReason: model.StopToolUse,
				Content: []model.ContentPart{model.ToolCallPart{
					ID: "tc-loop-2", Name: "Loop", Input: json.RawMessage(`{}`),
				}},
				Usage: model.TokenUsage{InputTokens: 80, OutputTokens: 10},
			},
			// Third response would be needed if maxTurns weren't enforced.
			model.Response{
				StopReason: model.StopEndTurn,
				Content:    []model.ContentPart{model.TextPart{Text: "done"}},
				Usage:      model.TokenUsage{InputTokens: 100, OutputTokens: 10},
			},
		)

	_, runErr := h.Run(context.Background(), "loop forever")
	if runErr == nil {
		t.Fatal("expected error from max turns exceeded")
	}
}
