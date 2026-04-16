package compact

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/model"
)

func TestEstimatePartTokens(t *testing.T) {
	tests := []struct {
		name     string
		part     model.ContentPart
		wantMin  int
		wantMax  int
	}{
		{
			name:    "empty text",
			part:    model.TextPart{Text: ""},
			wantMin: 0,
			wantMax: 0,
		},
		{
			name:    "short text returns minimum 1",
			part:    model.TextPart{Text: "hi"},
			wantMin: 1,
			wantMax: 1,
		},
		{
			name:    "100 byte text",
			part:    model.TextPart{Text: strings.Repeat("a", 100)},
			wantMin: 25,
			wantMax: 25,
		},
		{
			name:    "thinking with signature",
			part:    model.ThinkingPart{Text: strings.Repeat("x", 400)},
			wantMin: 100,
			wantMax: 100,
		},
		{
			name: "redacted thinking adds redacted data",
			part: model.ThinkingPart{
				Text:         "",
				Redacted:     true,
				RedactedData: strings.Repeat("y", 200),
			},
			wantMin: 50,
			wantMax: 50,
		},
		{
			name: "tool call with overhead",
			part: model.ToolCallPart{
				Name:  "FileRead",
				Input: json.RawMessage(`{"path":"/foo/bar.go"}`),
			},
			wantMin: 10, // (8+22)/4 + 10 = 17
			wantMax: 20,
		},
		{
			name: "tool result with overhead",
			part: model.ToolResultPart{
				ToolCallID: "abc",
				Content:    strings.Repeat("z", 100),
			},
			wantMin: 25 + structOverheadToolResult,
			wantMax: 25 + structOverheadToolResult,
		},
		{
			name:    "image fixed tokens",
			part:    model.ImagePart{MimeType: "image/png", Data: []byte{1, 2, 3}},
			wantMin: fixedImageTokens,
			wantMax: fixedImageTokens,
		},
		{
			name:    "document fixed tokens",
			part:    model.DocumentPart{MimeType: "application/pdf", Data: []byte{1}},
			wantMin: fixedDocumentTokens,
			wantMax: fixedDocumentTokens,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimatePartTokens(tt.part)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("EstimatePartTokens() = %d, want [%d, %d]", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestEstimateTokens(t *testing.T) {
	msg := model.Message{
		Role: model.RoleUser,
		Content: []model.ContentPart{
			model.TextPart{Text: strings.Repeat("a", 100)},
		},
	}
	got := EstimateTokens(msg)
	// 100/4 = 25 tokens + 4 overhead = 29
	if got != 29 {
		t.Errorf("EstimateTokens() = %d, want 29", got)
	}
}

func TestEstimateConversationTokens(t *testing.T) {
	msgs := []model.Message{
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: strings.Repeat("a", 100)}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: strings.Repeat("b", 200)}}},
	}
	got := EstimateConversationTokens(msgs)
	// msg1: 25 + 4 = 29, msg2: 50 + 4 = 54, total = 83
	if got != 83 {
		t.Errorf("EstimateConversationTokens() = %d, want 83", got)
	}
}

func TestEstimateSystemPromptTokens(t *testing.T) {
	sp := model.SystemPrompt{
		Blocks: []model.SystemBlock{
			{Text: strings.Repeat("x", 400)},
			{Text: strings.Repeat("y", 100)},
		},
	}
	got := EstimateSystemPromptTokens(sp)
	// 400/4 + 100/4 = 100 + 25 = 125
	if got != 125 {
		t.Errorf("EstimateSystemPromptTokens() = %d, want 125", got)
	}
}
