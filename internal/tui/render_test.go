package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/gogent/internal/model"
)

func TestRenderToolCall(t *testing.T) {
	tests := []struct {
		name     string
		call     model.ToolCallPart
		contains []string
	}{
		{
			name: "simple tool call",
			call: model.ToolCallPart{
				ID:    "t1",
				Name:  "Bash",
				Input: json.RawMessage(`{"command":"ls -la"}`),
			},
			contains: []string{"Bash", "command"},
		},
		{
			name: "empty input",
			call: model.ToolCallPart{
				ID:   "t2",
				Name: "Glob",
			},
			contains: []string{"Glob"},
		},
		{
			name: "tool with multiple params",
			call: model.ToolCallPart{
				ID:    "t3",
				Name:  "FileWrite",
				Input: json.RawMessage(`{"path":"/tmp/test.go","content":"package main"}`),
			},
			contains: []string{"FileWrite"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderToolCall(tt.call)
			for _, s := range tt.contains {
				if !strings.Contains(result, s) {
					t.Errorf("expected %q in result %q", s, result)
				}
			}
		})
	}
}

func TestRenderToolResult(t *testing.T) {
	tests := []struct {
		name     string
		result   model.ToolResultPart
		contains string
	}{
		{
			name: "normal result",
			result: model.ToolResultPart{
				ToolCallID: "t1",
				Content:    "file.go\ndir/",
			},
			contains: "result",
		},
		{
			name: "error result",
			result: model.ToolResultPart{
				ToolCallID: "t2",
				Content:    "permission denied",
				IsError:    true,
			},
			contains: "error",
		},
		{
			name: "long result truncated",
			result: model.ToolResultPart{
				ToolCallID: "t3",
				Content:    strings.Repeat("a", 300),
			},
			contains: "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderToolResult(tt.result)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("expected %q in result %q", tt.contains, result)
			}
		})
	}
}

func TestRenderThinking(t *testing.T) {
	tests := []struct {
		name     string
		part     model.ThinkingPart
		contains string
	}{
		{
			name:     "normal thinking",
			part:     model.ThinkingPart{Text: "Let me think about this..."},
			contains: "Let me think",
		},
		{
			name:     "redacted thinking",
			part:     model.ThinkingPart{Redacted: true, RedactedData: "encrypted"},
			contains: "redacted",
		},
		{
			name:     "long thinking truncated",
			part:     model.ThinkingPart{Text: strings.Repeat("x", 600)},
			contains: "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderThinking(tt.part)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("expected %q in result %q", tt.contains, result)
			}
		})
	}
}

func TestRenderMessage(t *testing.T) {
	// User message
	userMsg := model.Message{
		ID:   "m1",
		Role: model.RoleUser,
		Content: []model.ContentPart{
			model.TextPart{Text: "Hello, world!"},
		},
	}
	result := renderMessage(userMsg)
	if !strings.Contains(result, "You") {
		t.Error("expected user label in rendered message")
	}
	if !strings.Contains(result, "Hello, world!") {
		t.Error("expected message text in rendered message")
	}

	// Internal message should be empty
	internalMsg := model.Message{
		ID:   "m2",
		Role: model.RoleUser,
		Content: []model.ContentPart{
			model.TextPart{Text: "internal"},
		},
		Flags: model.MessageFlags{IsInternal: true},
	}
	result = renderMessage(internalMsg)
	if result != "" {
		t.Errorf("expected empty string for internal message, got %q", result)
	}

	// Assistant message
	assistantMsg := model.Message{
		ID:   "m3",
		Role: model.RoleAssistant,
		Content: []model.ContentPart{
			model.TextPart{Text: "I can help with that."},
		},
	}
	result = renderMessage(assistantMsg)
	if !strings.Contains(result, "Assistant") {
		t.Error("expected assistant label in rendered message")
	}
}

func TestRenderConversation(t *testing.T) {
	msgs := []model.Message{
		{
			ID:      "m1",
			Role:    model.RoleUser,
			Content: []model.ContentPart{model.TextPart{Text: "Hi"}},
		},
		{
			ID:      "m2",
			Role:    model.RoleAssistant,
			Content: []model.ContentPart{model.TextPart{Text: "Hello!"}},
		},
	}

	result := renderConversation(msgs)
	if !strings.Contains(result, "Hi") {
		t.Error("expected user text in conversation")
	}
	if !strings.Contains(result, "Hello!") {
		t.Error("expected assistant text in conversation")
	}
}
