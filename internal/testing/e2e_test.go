package testing_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/query"
	gtesting "github.com/artpar/pragma/internal/testing"
)

// These tests reproduce E2E scenarios observed against Google Gemini (gemini-2.5-flash)
// on 2026-04-13. Recordings in testdata/replays/e2e/ document the original runs.
// Tests use SequenceProvider with manually constructed responses matching Gemini output.

func TestE2E_SimpleText(t *testing.T) {
	// Scenario: "What is 2+2?" → LLM responds with text, no tool calls
	h := gtesting.NewHarness().
		WithProviderResponses(
			model.Response{
				ID:         "resp-simple",
				Model:      "gemini-2.5-flash",
				Content:    []model.ContentPart{model.TextPart{Text: "4"}},
				StopReason: model.StopEndTurn,
				Usage:      model.TokenUsage{InputTokens: 100, OutputTokens: 1},
			},
		)

	events, err := h.Run(context.Background(), "What is 2+2?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have text output
	var gotText bool
	for _, ev := range h.LoopEvents() {
		if te, ok := ev.(query.TextEvent); ok && te.Text == "4" {
			gotText = true
		}
	}
	if !gotText {
		t.Error("expected TextEvent with '4'")
	}

	// Should have TurnComplete with EndTurn
	var gotComplete bool
	for _, ev := range h.LoopEvents() {
		if tc, ok := ev.(query.TurnCompleteEvent); ok {
			if tc.StopReason == model.StopEndTurn {
				gotComplete = true
			}
		}
	}
	if !gotComplete {
		t.Error("expected TurnCompleteEvent with StopEndTurn")
	}

	// Conversation should have 2 messages: user + assistant
	msgs := h.ConversationMessages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != model.RoleUser {
		t.Error("first message should be user")
	}
	if msgs[1].Role != model.RoleAssistant {
		t.Error("second message should be assistant")
	}

	_ = events
}

func TestE2E_SingleToolCall(t *testing.T) {
	// Scenario: "List files in the current directory" → LLM calls Bash(ls), gets result, responds
	h := gtesting.NewHarness().
		WithProviderResponses(
			// Turn 1: LLM wants to call Bash
			model.Response{
				ID:    "resp-tool",
				Model: "gemini-2.5-flash",
				Content: []model.ContentPart{
					model.ToolCallPart{
						ID:    "tc-bash-1",
						Name:  "Bash",
						Input: json.RawMessage(`{"command":"ls -la","description":"List files"}`),
					},
				},
				StopReason: model.StopToolUse,
				Usage:      model.TokenUsage{InputTokens: 200, OutputTokens: 20},
			},
			// Turn 2: LLM summarizes tool output
			model.Response{
				ID:         "resp-summary",
				Model:      "gemini-2.5-flash",
				Content:    []model.ContentPart{model.TextPart{Text: "Here are the files in the current directory."}},
				StopReason: model.StopEndTurn,
				Usage:      model.TokenUsage{InputTokens: 300, OutputTokens: 15},
			},
		).
		WithTool("Bash", func(input json.RawMessage) (string, error) {
			return "file1.go\nfile2.go\nREADME.md", nil
		})

	_, err := h.Run(context.Background(), "List files in the current directory")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have tool call and result events
	if err := h.AssertEvent("ToolCallReceived", nil); err != nil {
		t.Error(err)
	}
	if err := h.AssertEvent("ToolExecutionCompleted", nil); err != nil {
		t.Error(err)
	}

	// Should have the event sequence: tool received → tool executed
	if err := h.AssertEventSequence(
		"ToolCallReceived",
		"ToolExecutionCompleted",
	); err != nil {
		t.Error(err)
	}

	// Conversation: user + assistant(tool_call) + user(tool_result) + assistant(text) = 4 messages
	msgs := h.ConversationMessages()
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	if msgs[0].Role != model.RoleUser {
		t.Error("msg[0] should be user")
	}
	if msgs[1].Role != model.RoleAssistant {
		t.Error("msg[1] should be assistant (tool call)")
	}
	if msgs[2].Role != model.RoleUser {
		t.Error("msg[2] should be user (tool result)")
	}
	if msgs[3].Role != model.RoleAssistant {
		t.Error("msg[3] should be assistant (summary)")
	}

	// Provider should have been called twice
	calls := h.ProviderCalls()
	if len(calls) != 2 {
		t.Errorf("expected 2 provider calls, got %d", len(calls))
	}
}

func TestE2E_ReadTool(t *testing.T) {
	// Scenario: "Read the first 10 lines of go.mod" → LLM calls Read tool, summarizes
	h := gtesting.NewHarness().
		WithProviderResponses(
			model.Response{
				ID:    "resp-read",
				Model: "gemini-2.5-flash",
				Content: []model.ContentPart{
					model.ToolCallPart{
						ID:    "tc-read-1",
						Name:  "Read",
						Input: json.RawMessage(`{"file_path":"/tmp/test/go.mod","limit":10}`),
					},
				},
				StopReason: model.StopToolUse,
				Usage:      model.TokenUsage{InputTokens: 200, OutputTokens: 15},
			},
			model.Response{
				ID:         "resp-read-summary",
				Model:      "gemini-2.5-flash",
				Content:    []model.ContentPart{model.TextPart{Text: "The module is github.com/artpar/pragma."}},
				StopReason: model.StopEndTurn,
				Usage:      model.TokenUsage{InputTokens: 300, OutputTokens: 10},
			},
		).
		WithTool("Read", func(input json.RawMessage) (string, error) {
			return "module github.com/artpar/pragma\n\ngo 1.25.0\n", nil
		})

	_, err := h.Run(context.Background(), "Read the first 10 lines of go.mod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have ToolCallReceived for Read
	if err := h.AssertEventField("ToolCallReceived", "tool_name", "Read"); err != nil {
		t.Error(err)
	}

	// 4 messages: user → assistant(tool_call) → user(tool_result) → assistant(text)
	msgs := h.ConversationMessages()
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
}

func TestE2E_MultiTurnToolLoop(t *testing.T) {
	// Scenario: "Find .go files in cmd/ and count them"
	// Turn 1: LLM calls Glob to find files
	// Turn 2: LLM responds with count
	h := gtesting.NewHarness().
		WithProviderResponses(
			model.Response{
				ID:    "resp-glob",
				Model: "gemini-2.5-flash",
				Content: []model.ContentPart{
					model.ToolCallPart{
						ID:    "tc-glob-1",
						Name:  "Glob",
						Input: json.RawMessage(`{"pattern":"cmd/**/*.go"}`),
					},
				},
				StopReason: model.StopToolUse,
			},
			model.Response{
				ID:    "resp-bash-count",
				Model: "gemini-2.5-flash",
				Content: []model.ContentPart{
					model.ToolCallPart{
						ID:    "tc-bash-count",
						Name:  "Bash",
						Input: json.RawMessage(`{"command":"echo 4","description":"Count files"}`),
					},
				},
				StopReason: model.StopToolUse,
			},
			model.Response{
				ID:         "resp-final",
				Model:      "gemini-2.5-flash",
				Content:    []model.ContentPart{model.TextPart{Text: "I found 4 .go files in cmd/."}},
				StopReason: model.StopEndTurn,
			},
		).
		WithTool("Glob", func(input json.RawMessage) (string, error) {
			return "cmd/pragma/main.go\ncmd/pragma/lifecycle.go\ncmd/pragma/sessions.go\ncmd/pragma/replay.go", nil
		}).
		WithTool("Bash", func(input json.RawMessage) (string, error) {
			return "4", nil
		})

	_, err := h.Run(context.Background(), "Find all .go files in cmd/ and count them")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Provider called 3 times (glob turn + bash turn + final text)
	calls := h.ProviderCalls()
	if len(calls) != 3 {
		t.Errorf("expected 3 provider calls, got %d", len(calls))
	}

	// 6 messages: user + assistant(glob) + user(glob_result) + assistant(bash) + user(bash_result) + assistant(text)
	msgs := h.ConversationMessages()
	if len(msgs) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(msgs))
	}

	// Final message should be assistant text
	lastMsg := msgs[len(msgs)-1]
	if lastMsg.Role != model.RoleAssistant {
		t.Error("last message should be assistant")
	}
	var hasText bool
	for _, part := range lastMsg.Content {
		if _, ok := part.(model.TextPart); ok {
			hasText = true
		}
	}
	if !hasText {
		t.Error("last message should contain text")
	}
}

func TestE2E_ToolError(t *testing.T) {
	// Scenario: Tool returns an error → LLM gets error in tool_result → responds gracefully
	h := gtesting.NewHarness().
		WithProviderResponses(
			model.Response{
				ID:    "resp-tool-err",
				Model: "gemini-2.5-flash",
				Content: []model.ContentPart{
					model.ToolCallPart{
						ID:    "tc-fail",
						Name:  "FailTool",
						Input: json.RawMessage(`{}`),
					},
				},
				StopReason: model.StopToolUse,
			},
			model.Response{
				ID:         "resp-recover",
				Model:      "gemini-2.5-flash",
				Content:    []model.ContentPart{model.TextPart{Text: "The tool failed. Let me try a different approach."}},
				StopReason: model.StopEndTurn,
			},
		).
		WithToolError("FailTool", "permission denied: /etc/shadow")

	_, err := h.Run(context.Background(), "Read /etc/shadow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Tool should have executed (even though it errored)
	if err := h.AssertEvent("ToolExecutionCompleted", nil); err != nil {
		// ToolExecutionFailed might be the event name
		if err2 := h.AssertEvent("ToolExecutionFailed", nil); err2 != nil {
			t.Errorf("expected tool execution event: %v / %v", err, err2)
		}
	}

	// Conversation should still complete with 4 messages
	msgs := h.ConversationMessages()
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
}
