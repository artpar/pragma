package anthropic

import (
	"encoding/json"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/artpar/gogent/internal/model"
)

// responseFromWire converts an Anthropic API response to an internal Response.
func responseFromWire(msg *sdk.Message, mapper *IDMapper) model.Response {
	var parts []model.ContentPart
	for _, block := range msg.Content {
		part := contentBlockFromWire(block, mapper)
		if part != nil {
			parts = append(parts, part)
		}
	}
	return model.Response{
		ID:         msg.ID,
		Model:      string(msg.Model),
		Content:    parts,
		StopReason: stopReasonFromWire(msg.StopReason),
		Usage:      usageFromWire(msg.Usage),
	}
}

// contentBlockFromWire converts a single Anthropic content block to an internal ContentPart.
// Returns nil for unsupported block types.
func contentBlockFromWire(block sdk.ContentBlockUnion, mapper *IDMapper) model.ContentPart {
	switch block.Type {
	case "text":
		return model.TextPart{Text: block.Text}

	case "tool_use":
		internalID := model.NewUUID()
		mapper.RegisterPair(internalID, block.ID)
		var input json.RawMessage
		if len(block.Input) > 0 {
			input = block.Input
		} else {
			input = json.RawMessage(`{}`)
		}
		return model.ToolCallPart{
			ID:    internalID,
			Name:  block.Name,
			Input: input,
		}

	case "thinking":
		return model.ThinkingPart{
			Text:      block.Thinking,
			Signature: block.Signature,
		}

	case "redacted_thinking":
		return model.ThinkingPart{
			Text:      "[redacted]",
			Signature: "",
		}

	default:
		// Unsupported block types (server_tool_use, web_search_tool_result, etc.)
		// are silently skipped. They can be added as needed.
		return nil
	}
}

// stopReasonFromWire maps Anthropic stop reasons to internal StopReason.
func stopReasonFromWire(sr sdk.StopReason) model.StopReason {
	switch sr {
	case sdk.StopReasonEndTurn:
		return model.StopEndTurn
	case sdk.StopReasonToolUse:
		return model.StopToolUse
	case sdk.StopReasonMaxTokens:
		return model.StopMaxTokens
	case sdk.StopReasonStopSequence:
		return model.StopEndTurn
	case sdk.StopReasonPauseTurn:
		return model.StopEndTurn
	case sdk.StopReasonRefusal:
		return model.StopError
	default:
		return model.StopError
	}
}

// usageFromWire converts Anthropic usage to internal TokenUsage.
func usageFromWire(u sdk.Usage) model.TokenUsage {
	return model.TokenUsage{
		InputTokens:              int(u.InputTokens),
		OutputTokens:             int(u.OutputTokens),
		CacheCreationInputTokens: int(u.CacheCreationInputTokens),
		CacheReadInputTokens:     int(u.CacheReadInputTokens),
	}
}

