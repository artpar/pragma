// Package shared provides utilities shared across all provider implementations:
// token estimation, retry logic, and error classification.
package shared

import (
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
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
				// Flat estimate for binary documents (PDFs). Binary byte count / 4 is meaningless.
				total += 5000
			}
		}
	}
	observe.GlobalTrace("return: total")
	return total
}
