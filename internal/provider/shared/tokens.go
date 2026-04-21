// Package shared provides utilities shared across all provider implementations:
// token estimation, retry logic, and error classification.
package shared

import (
	"encoding/json"
	"strings"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

// EstimateTokens provides a rough token estimate for observability events.
// Uses ~4 characters per token heuristic, 1000 flat for images.
func EstimateTokens(params provider.RequestParams) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	total := 0
	for _, block := range params.System.Blocks {
		observe.GlobalTrace("range params.System.Blocks")
		total += len(block.Text) / 4
	}
	for _, m := range params.Messages {
		observe.GlobalTrace("range params.Messages")
		for _, part := range m.Content {
			observe.GlobalTrace("range m.Content")
			switch p := part.(type) {
			case model.TextPart:
				observe.GlobalTrace("typecase: model.TextPart")
				total += len(p.Text) / 4
			case model.ToolCallPart:
				observe.GlobalTrace("typecase: model.ToolCallPart")
				total += len(p.Input) / 4
			case model.ToolResultPart:
				observe.GlobalTrace("typecase: model.ToolResultPart")
				total += len(p.Content) / 4
			case model.ThinkingPart:
				observe.GlobalTrace("typecase: model.ThinkingPart")
				total += len(p.Text) / 4
			case model.ImagePart:
				observe.GlobalTrace("typecase: model.ImagePart")
				total += 1000
			case model.DocumentPart:
				observe.GlobalTrace("typecase: model.DocumentPart")

				total += 5000
			}
		}
	}
	observe.GlobalTrace("return: total")
	return total
}

// EstimateOutputTokens provides a rough token estimate for response content.
// Used when providers don't report usage (e.g., Lilac/vLLM streaming).
func EstimateOutputTokens(parts []model.ContentPart) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	total := 0
	for _, part := range parts {
		observe.GlobalTrace("range parts")
		switch p := part.(type) {
		case model.TextPart:
			observe.GlobalTrace("typecase: model.TextPart")
			total += len(p.Text) / 4
		case model.ToolCallPart:
			observe.GlobalTrace("typecase: model.ToolCallPart")
			total += len(p.Input) / 4
		case model.ThinkingPart:
			observe.GlobalTrace("typecase: model.ThinkingPart")
			total += len(p.Text) / 4
		}
	}
	observe.GlobalTrace("return: total")
	return total
}

// MarshalContent serializes ContentParts to json.RawMessage for event recording.
func MarshalContent(parts []model.ContentPart) json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(parts) == 0 {
		observe.GlobalTrace("if: len(parts) == 0")
		observe.GlobalTrace("return: nil")
		return nil
	}
	data, err := model.MarshalContentParts(parts)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: data")
	return data
}

// SystemText concatenates all system prompt blocks into a single string.
func SystemText(sp model.SystemPrompt) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(sp.Blocks) == 0 {
		observe.GlobalTrace("if: len(sp.Blocks) == 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	if len(sp.Blocks) == 1 {
		observe.GlobalTrace("if: len(sp.Blocks) == 1")
		observe.GlobalTrace("return: sp.Blocks[0].Text")
		return sp.Blocks[0].Text
	}
	var parts []string
	for _, b := range sp.Blocks {
		observe.GlobalTrace("range sp.Blocks")
		parts = append(parts, b.Text)
	}
	observe.GlobalTrace("return: strings.Join(parts, \"\\n\\n\")")
	return strings.Join(parts, "\n\n")
}
