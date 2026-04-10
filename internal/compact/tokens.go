package compact

import "github.com/artpar/gogent/internal/model"

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
	switch p := part.(type) {
	case model.TextPart:
		n := len(p.Text) / bytesPerToken
		if n == 0 && len(p.Text) > 0 {
			return 1
		}
		return n

	case model.ThinkingPart:
		tokens := len(p.Text) / bytesPerToken
		if p.Redacted && len(p.RedactedData) > 0 {
			tokens += len(p.RedactedData) / bytesPerToken
		}
		return tokens

	case model.ToolCallPart:
		return (len(p.Name)+len(p.Input))/bytesPerToken + structOverheadToolCall

	case model.ToolResultPart:
		return len(p.Content)/bytesPerToken + structOverheadToolResult

	case model.ImagePart:
		return fixedImageTokens

	case model.DocumentPart:
		return fixedDocumentTokens

	default:
		return 0
	}
}

// EstimateTokens returns the estimated token count for a single message.
func EstimateTokens(msg model.Message) int {
	tokens := 0
	for _, part := range msg.Content {
		tokens += EstimatePartTokens(part)
	}
	// Per-message overhead: role label, structure
	tokens += 4
	return tokens
}

// EstimateConversationTokens returns the total estimated tokens for all messages.
func EstimateConversationTokens(msgs []model.Message) int {
	tokens := 0
	for _, msg := range msgs {
		tokens += EstimateTokens(msg)
	}
	return tokens
}

// EstimateSystemPromptTokens returns the estimated token count for a system prompt.
// This is needed for death spiral prevention (#24179): the threshold calculation
// must account for system prompt tokens being re-injected after compaction.
func EstimateSystemPromptTokens(sp model.SystemPrompt) int {
	tokens := 0
	for _, block := range sp.Blocks {
		tokens += len(block.Text) / bytesPerToken
	}
	return tokens
}
