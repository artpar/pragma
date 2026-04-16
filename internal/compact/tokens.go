package compact

import (
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// bytesPerToken is the rough heuristic for token estimation.
// Matches the TS reference's default of ~4 bytes per token.
const bytesPerToken = 4

// structOverheadToolCall accounts for JSON structure tokens around a tool call
// (braces, field names, quotes, etc.).
const structOverheadToolCall = 10

// structOverheadToolResult accounts for JSON structure tokens around a tool result.
const structOverheadToolResult = 5

// fixedImageTokens is the estimated token count for an image content block.
const fixedImageTokens = 2000

// fixedDocumentTokens is the estimated token count for a document content block.
const fixedDocumentTokens = 2000

// EstimatePartTokens returns the estimated token count for a single ContentPart.
func EstimatePartTokens(part model.ContentPart) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch p := part.(type) {
	case model.TextPart:
		observe.GlobalTrace("typecase: model.TextPart")
		n := len(p.Text) / bytesPerToken
		if n == 0 && len(p.Text) > 0 {
			observe.GlobalTrace("return: 1")
			return 1
		}
		return n

	case model.ThinkingPart:
		observe.GlobalTrace("typecase: model.ThinkingPart")
		tokens := len(p.Text) / bytesPerToken
		if p.Redacted && len(p.RedactedData) > 0 {
			tokens += len(p.RedactedData) / bytesPerToken
		}
		return tokens

	case model.ToolCallPart:
		observe.GlobalTrace("typecase: model.ToolCallPart")
		return (len(p.Name)+len(p.Input))/bytesPerToken + structOverheadToolCall

	case model.ToolResultPart:
		observe.GlobalTrace("typecase: model.ToolResultPart")
		return len(p.Content)/bytesPerToken + structOverheadToolResult

	case model.ImagePart:
		observe.GlobalTrace("typecase: model.ImagePart")
		return fixedImageTokens

	case model.DocumentPart:
		observe.GlobalTrace("typecase: model.DocumentPart")
		return fixedDocumentTokens

	default:
		observe.GlobalTrace("typedefault")
		return 0
	}
}

// EstimateTokens returns the estimated token count for a single message.
func EstimateTokens(msg model.Message) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	tokens := 0
	for _, part := range msg.Content {
		observe.GlobalTrace("range msg.Content")
		tokens += EstimatePartTokens(part)
	}

	tokens += 4
	observe.GlobalTrace("return: tokens")
	return tokens
}

// EstimateConversationTokens returns the total estimated tokens for all messages.
func EstimateConversationTokens(msgs []model.Message) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	tokens := 0
	for _, msg := range msgs {
		observe.GlobalTrace("range msgs")
		tokens += EstimateTokens(msg)
	}
	observe.GlobalTrace("return: tokens")
	return tokens
}

// EstimateSystemPromptTokens returns the estimated token count for a system prompt.
// This is needed for death spiral prevention (#24179): the threshold calculation
// must account for system prompt tokens being re-injected after compaction.
func EstimateSystemPromptTokens(sp model.SystemPrompt) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	tokens := 0
	for _, block := range sp.Blocks {
		observe.GlobalTrace("range sp.Blocks")
		tokens += len(block.Text) / bytesPerToken
	}
	observe.GlobalTrace("return: tokens")
	return tokens
}
