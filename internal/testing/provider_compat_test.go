package testing_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/session"
	gtesting "github.com/artpar/pragma/internal/testing"
)

// These tests verify provider-compatibility edge cases observed during multi-provider testing.
// All use SequenceProvider (deterministic, no live API calls) modeling realistic provider behaviors.

func TestProviderCompat_EmptyGrepNoHallucination(t *testing.T) {
	// Google Gemini regression: when Grep returns empty, the LLM sometimes fabricates
	// file paths in follow-up tool calls. This test verifies the "Do NOT fabricate"
	// guidance propagates through the conversation and the LLM stops (EndTurn) without
	// issuing further tool calls to non-existent files.

	emptyGrepGuidance := "IMPORTANT: No matches were found for this pattern. Do NOT fabricate results. Try: case-insensitive (-i), partial name, different naming convention, or broader pattern."

	h := gtesting.NewHarness().
		WithProviderResponses(
			// Turn 1: LLM calls Grep
			model.Response{
				ID:    "resp-grep",
				Model: "gemini-2.5-flash",
				Content: []model.ContentPart{
					model.ToolCallPart{
						ID:    "tc-grep-1",
						Name:  "Grep",
						Input: json.RawMessage(`{"pattern":"nonexistent_xyz_symbol","path":"/tmp/test"}`),
					},
				},
				StopReason: model.StopToolUse,
				Usage:      model.TokenUsage{InputTokens: 500, OutputTokens: 30},
			},
			// Turn 2: LLM correctly responds with text (no hallucinated tool calls)
			model.Response{
				ID:         "resp-no-hallucinate",
				Model:      "gemini-2.5-flash",
				Content:    []model.ContentPart{model.TextPart{Text: "I couldn't find any matches for that pattern. Let me try a different approach."}},
				StopReason: model.StopEndTurn,
				Usage:      model.TokenUsage{InputTokens: 600, OutputTokens: 20},
			},
		).
		WithTool("Grep", func(input json.RawMessage) (string, error) {
			return emptyGrepGuidance, nil
		})

	_, err := h.Run(context.Background(), "Search for nonexistent_xyz_symbol in the codebase")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the empty grep guidance is in the conversation (tool result message)
	msgs := h.ConversationMessages()
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages (user + tool_call + tool_result + text), got %d", len(msgs))
	}

	// msg[2] is the tool result (user role)
	toolResultMsg := msgs[2]
	var foundGuidance bool
	for _, part := range toolResultMsg.Content {
		if tr, ok := part.(model.ToolResultPart); ok {
			if strings.Contains(tr.Content, "Do NOT fabricate") {
				foundGuidance = true
			}
		}
	}
	if !foundGuidance {
		t.Error("tool result should contain 'Do NOT fabricate' guidance")
	}

	// Provider should have been called exactly 2 times (grep turn + final text)
	// A hallucinating LLM would cause a 3rd call
	calls := h.ProviderCalls()
	if len(calls) != 2 {
		t.Errorf("expected 2 provider calls (no hallucinated follow-up), got %d", len(calls))
	}

	// The second provider call should include the tool result with guidance
	if len(calls) >= 2 {
		lastCall := calls[1]
		var hasGuidanceInContext bool
		for _, msg := range lastCall.Messages {
			for _, part := range msg.Content {
				if tr, ok := part.(model.ToolResultPart); ok {
					if strings.Contains(tr.Content, "Do NOT fabricate") {
						hasGuidanceInContext = true
					}
				}
			}
		}
		if !hasGuidanceInContext {
			t.Error("second provider call should include the 'Do NOT fabricate' guidance in message context")
		}
	}
}

func TestProviderCompat_ToolInputJSONFidelity(t *testing.T) {
	// Lilac/any-llm-go regression: tool call inputs with complex JSON (nested strings,
	// special chars, unicode) must survive the translation round-trip without mangling.

	complexInput := `{"file_path":"/tmp/test/src/日本語.go","old_string":"func héllo() {\n\treturn \"world\\n\"\n}","new_string":"func héllo() {\n\treturn \"updated\\n\"\n}"}`

	h := gtesting.NewHarness().
		WithProviderResponses(
			// Turn 1: LLM calls Edit with complex input
			model.Response{
				ID:    "resp-edit",
				Model: "kimi-k2.5",
				Content: []model.ContentPart{
					model.ToolCallPart{
						ID:    "tc-edit-1",
						Name:  "Edit",
						Input: json.RawMessage(complexInput),
					},
				},
				StopReason: model.StopToolUse,
				Usage:      model.TokenUsage{InputTokens: 400, OutputTokens: 50},
			},
			// Turn 2: LLM responds
			model.Response{
				ID:         "resp-edit-done",
				Model:      "kimi-k2.5",
				Content:    []model.ContentPart{model.TextPart{Text: "I've updated the function."}},
				StopReason: model.StopEndTurn,
				Usage:      model.TokenUsage{InputTokens: 500, OutputTokens: 10},
			},
		).
		WithTool("Edit", func(input json.RawMessage) (string, error) {
			// Verify the input survived intact
			var parsed map[string]interface{}
			if err := json.Unmarshal(input, &parsed); err != nil {
				return "", err
			}
			fp, _ := parsed["file_path"].(string)
			if fp != "/tmp/test/src/日本語.go" {
				return "", fmt.Errorf("file path mangled: %q", fp)
			}
			return "Edit applied successfully.", nil
		})

	_, err := h.Run(context.Background(), "Fix the héllo function in 日本語.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the tool call input in the conversation preserved exact bytes
	msgs := h.ConversationMessages()
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}

	// msg[1] is the assistant message with the tool call
	assistMsg := msgs[1]
	var foundToolCall bool
	for _, part := range assistMsg.Content {
		if tc, ok := part.(model.ToolCallPart); ok && tc.Name == "Edit" {
			foundToolCall = true
			// Verify the JSON round-trips correctly
			var parsed map[string]interface{}
			if err := json.Unmarshal(tc.Input, &parsed); err != nil {
				t.Errorf("tool call input not valid JSON after round-trip: %v", err)
			}
			if fp, _ := parsed["file_path"].(string); fp != "/tmp/test/src/日本語.go" {
				t.Errorf("file_path mangled: got %q", fp)
			}
			if os, _ := parsed["old_string"].(string); !strings.Contains(os, "héllo") {
				t.Errorf("old_string lost unicode: got %q", os)
			}
		}
	}
	if !foundToolCall {
		t.Error("expected Edit tool call in assistant message")
	}
}

func TestProviderCompat_CrossProviderSessionResume(t *testing.T) {
	// Sessions persist Provider in JSONL header. Verify round-trip for different providers.
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	store, err := session.NewStore()
	if err != nil {
		t.Fatal(err)
	}

	providers := []struct {
		provider string
		model    string
	}{
		{"google", "gemini-2.5-flash"},
		{"lilac", "kimi-k2.5"},
		{"anthropic", "claude-sonnet-4-6"},
		{"openai", "gpt-4o"},
		{"groq", "llama-3.3-70b"},
	}

	for _, p := range providers {
		t.Run(p.provider, func(t *testing.T) {
			now := time.Now()
			sessionID := "session-" + p.provider

			conv := model.Conversation{
				ID: sessionID,
				Messages: []model.Message{
					{
						ID:        "msg-1",
						Role:      model.RoleUser,
						Content:   []model.ContentPart{model.TextPart{Text: "Hello from " + p.provider}},
						Timestamp: now,
					},
					{
						ID:        "msg-2",
						Role:      model.RoleAssistant,
						Content:   []model.ContentPart{model.TextPart{Text: "Response via " + p.provider}},
						Timestamp: now.Add(time.Second),
					},
				},
				Model:     p.model,
				Provider:  p.provider,
				WorkDir:   "/tmp/test",
				CreatedAt: now,
				UpdatedAt: now.Add(time.Second),
			}

			// Create session
			w, err := store.Create(session.HeaderData{
				SessionID: sessionID,
				Model:     p.model,
				Provider:  p.provider,
				WorkDir:   "/tmp/test",
				CreatedAt: now,
			})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			for _, msg := range conv.Messages {
				if err := w.WriteMessage(msg); err != nil {
					t.Fatalf("WriteMessage: %v", err)
				}
			}
			if err := w.WriteMetadata(session.MetadataData{
				CostUSD:   0.01,
				TurnCount: 1,
				UpdatedAt: now.Add(time.Second),
			}); err != nil {
				t.Fatalf("WriteMetadata: %v", err)
			}
			w.Close()

			// Load and verify provider round-trip
			loaded, err := store.Load(sessionID)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			if loaded.Conversation.Provider != p.provider {
				t.Errorf("Provider = %q, want %q", loaded.Conversation.Provider, p.provider)
			}
			if loaded.Conversation.Model != p.model {
				t.Errorf("Model = %q, want %q", loaded.Conversation.Model, p.model)
			}
			if len(loaded.Conversation.Messages) != 2 {
				t.Errorf("Messages = %d, want 2", len(loaded.Conversation.Messages))
			}
		})
	}
}

func TestProviderCompat_MixedEmptyNonEmptyBatchResults(t *testing.T) {
	// When LLM issues multiple tool calls in one turn and one returns empty while
	// the other returns results, both tool results must appear in the next provider call.

	emptyGrepResult := "IMPORTANT: No matches were found for this pattern. Do NOT fabricate results. Try: case-insensitive (-i), partial name, different naming convention, or broader pattern."
	globResult := "src/main.go\nsrc/util.go\nsrc/handler.go"

	h := gtesting.NewHarness().
		WithProviderResponses(
			// Turn 1: LLM calls both Grep and Glob simultaneously
			model.Response{
				ID:    "resp-batch",
				Model: "gemini-2.5-flash",
				Content: []model.ContentPart{
					model.ToolCallPart{
						ID:    "tc-grep-1",
						Name:  "Grep",
						Input: json.RawMessage(`{"pattern":"nonexistent_pattern","path":"/tmp/test"}`),
					},
					model.ToolCallPart{
						ID:    "tc-glob-1",
						Name:  "Glob",
						Input: json.RawMessage(`{"pattern":"src/**/*.go"}`),
					},
				},
				StopReason: model.StopToolUse,
				Usage:      model.TokenUsage{InputTokens: 500, OutputTokens: 40},
			},
			// Turn 2: LLM responds using only the Glob results
			model.Response{
				ID:         "resp-batch-summary",
				Model:      "gemini-2.5-flash",
				Content:    []model.ContentPart{model.TextPart{Text: "I found 3 Go files in src/. The grep pattern had no matches."}},
				StopReason: model.StopEndTurn,
				Usage:      model.TokenUsage{InputTokens: 700, OutputTokens: 25},
			},
		).
		WithTool("Grep", func(input json.RawMessage) (string, error) {
			return emptyGrepResult, nil
		}).
		WithTool("Glob", func(input json.RawMessage) (string, error) {
			return globResult, nil
		})

	_, err := h.Run(context.Background(), "Search for nonexistent_pattern and list all Go files in src/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify conversation structure: user + assistant(2 tool calls) + user(2 tool results) + assistant(text)
	msgs := h.ConversationMessages()
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}

	// msg[2] should contain both tool results
	toolResultMsg := msgs[2]
	var grepResultFound, globResultFound bool
	for _, part := range toolResultMsg.Content {
		if tr, ok := part.(model.ToolResultPart); ok {
			switch tr.ToolCallID {
			case "tc-grep-1":
				grepResultFound = true
				if !strings.Contains(tr.Content, "No matches") {
					t.Errorf("grep result should contain empty guidance, got: %q", tr.Content[:min(len(tr.Content), 80)])
				}
			case "tc-glob-1":
				globResultFound = true
				if !strings.Contains(tr.Content, "main.go") {
					t.Errorf("glob result should contain file list, got: %q", tr.Content[:min(len(tr.Content), 80)])
				}
			}
		}
	}
	if !grepResultFound {
		t.Error("missing Grep tool result in conversation")
	}
	if !globResultFound {
		t.Error("missing Glob tool result in conversation")
	}

	// Verify the second provider call includes both tool results in context
	calls := h.ProviderCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 provider calls, got %d", len(calls))
	}

	secondCall := calls[1]
	var grepInCtx, globInCtx bool
	for _, msg := range secondCall.Messages {
		for _, part := range msg.Content {
			if tr, ok := part.(model.ToolResultPart); ok {
				if tr.ToolCallID == "tc-grep-1" {
					grepInCtx = true
				}
				if tr.ToolCallID == "tc-glob-1" {
					globInCtx = true
				}
			}
		}
	}
	if !grepInCtx {
		t.Error("second provider call missing Grep result in context")
	}
	if !globInCtx {
		t.Error("second provider call missing Glob result in context")
	}
}
