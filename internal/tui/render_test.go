package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/tui/render"
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
			contains: []string{"Bash", "ls -la"}, // arg shown inline in parens
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
			result := render.RenderToolCall(tt.call, 80)
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
			contains: "file.go",
		},
		{
			name: "error result",
			result: model.ToolResultPart{
				ToolCallID: "t2",
				Content:    "permission denied",
				IsError:    true,
			},
			contains: "permission denied",
		},
		{
			name: "empty result shows no output",
			result: model.ToolResultPart{
				ToolCallID: "t3",
				Content:    "",
			},
			contains: "no output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := render.WrapWithBracket(tt.result.Content, tt.result.IsError, 80)
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
			name:     "long thinking preserved",
			part:     model.ThinkingPart{Text: strings.Repeat("x", 600)},
			contains: strings.Repeat("x", 100), // no truncation anymore
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := render.RenderThinking(tt.part)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("expected %q in result %q", tt.contains, result)
			}
		})
	}
}

func TestRenderMessage(t *testing.T) {
	md := render.NewMarkdownRenderer(80)

	// User message
	userMsg := model.Message{
		ID:   "m1",
		Role: model.RoleUser,
		Content: []model.ContentPart{
			model.TextPart{Text: "Hello, world!"},
		},
	}
	result := render.RenderMessage(userMsg, md)
	if !strings.Contains(result, "❯") {
		t.Error("expected ❯ glyph in rendered user message")
	}
	if !strings.Contains(result, "Hello") {
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
	result = render.RenderMessage(internalMsg, md)
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
	result = render.RenderMessage(assistantMsg, md)
	if !strings.Contains(result, "I can help") {
		t.Error("expected assistant text in rendered message")
	}
}

func TestRenderConversation(t *testing.T) {
	md := render.NewMarkdownRenderer(80)
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

	result := render.RenderConversation(msgs, md)
	if !strings.Contains(result, "Hi") {
		t.Error("expected user text in conversation")
	}
	if !strings.Contains(result, "Hello") {
		t.Error("expected assistant text in conversation")
	}
}
