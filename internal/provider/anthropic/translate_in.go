package anthropic

import (
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
)

// responseFromWire converts an Anthropic API response to an internal Response.
// Unsupported content block types are skipped with a warning emitted via bus.
func responseFromWire(msg *sdk.Message, mapper *IDMapper, bus *observe.EventBus) model.Response {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var parts []model.ContentPart
	var skipped []string
	for _, block := range msg.Content {
		observe.GlobalTrace("range msg.Content")
		part, skippedType := contentBlockFromWire(block, mapper)
		if part != nil {
			observe.GlobalTrace("if: part != nil")
			parts = append(parts, part)
		} else if skippedType != "" {
			observe.GlobalTrace("else-if: skippedType != \"\"")
			skipped = append(skipped, skippedType)
		}
	}
	if len(skipped) > 0 && bus != nil {
		observe.GlobalTrace("if: len(skipped) > 0 && bus != nil")
		bus.Emit(observe.ErrorOccurred{
			EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
			Severity:     "warn",
			Component:    "anthropic.translate_in",
			ErrorType:    "unsupported_content_block",
			ErrorMessage: fmt.Sprintf("skipped %d unsupported content block(s): %s", len(skipped), strings.Join(skipped, ", ")),
		})
	}
	observe.GlobalTrace("return: model.Response{\n\tID:\t\tmsg.ID,\n\tModel:\t\tstring(msg.Model),\n\tContent:\tparts,\n\tS...")
	return model.Response{
		ID:         msg.ID,
		Model:      string(msg.Model),
		Content:    parts,
		StopReason: stopReasonFromWire(msg.StopReason),
		Usage:      usageFromWire(msg.Usage),
	}
}

// contentBlockFromWire converts a single Anthropic content block to an internal ContentPart.
// Returns (nil, blockType) for unsupported block types so the caller can log them.
func contentBlockFromWire(block sdk.ContentBlockUnion, mapper *IDMapper) (model.ContentPart, string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch block.Type {
	case "text":
		observe.GlobalTrace("case: \"text\"")
		return model.TextPart{Text: block.Text}, ""

	case "tool_use":
		observe.GlobalTrace("case: \"tool_use\"")
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
		}, ""

	case "thinking":
		observe.GlobalTrace("case: \"thinking\"")
		return model.ThinkingPart{
			Text:      block.Thinking,
			Signature: block.Signature,
		}, ""

	case "redacted_thinking":
		observe.GlobalTrace("case: \"redacted_thinking\"")
		return model.ThinkingPart{
			Text:         "[redacted]",
			Redacted:     true,
			RedactedData: block.Data,
		}, ""

	default:
		observe.GlobalTrace("default")

		return nil, block.Type
	}
}

// stopReasonFromWire maps Anthropic stop reasons to internal StopReason.
func stopReasonFromWire(sr sdk.StopReason) model.StopReason {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch sr {
	case sdk.StopReasonEndTurn:
		observe.GlobalTrace("case: sdk.StopReasonEndTurn")
		return model.StopEndTurn
	case sdk.StopReasonToolUse:
		observe.GlobalTrace("case: sdk.StopReasonToolUse")
		return model.StopToolUse
	case sdk.StopReasonMaxTokens:
		observe.GlobalTrace("case: sdk.StopReasonMaxTokens")
		return model.StopMaxTokens
	case sdk.StopReasonStopSequence:
		observe.GlobalTrace("case: sdk.StopReasonStopSequence")
		return model.StopEndTurn
	case sdk.StopReasonPauseTurn:
		observe.GlobalTrace("case: sdk.StopReasonPauseTurn")
		return model.StopPauseTurn
	case sdk.StopReasonRefusal:
		observe.GlobalTrace("case: sdk.StopReasonRefusal")
		return model.StopError
	default:
		observe.GlobalTrace("default")
		return model.StopError
	}
}

// usageFromWire converts Anthropic usage to internal TokenUsage.
func usageFromWire(u sdk.Usage) model.TokenUsage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: model.TokenUsage{\n\tInputTokens:\t\t\tint(u.InputTokens),\n\tOutputTokens:\t\t\tint(u....")
	return model.TokenUsage{
		InputTokens:              int(u.InputTokens),
		OutputTokens:             int(u.OutputTokens),
		CacheCreationInputTokens: int(u.CacheCreationInputTokens),
		CacheReadInputTokens:     int(u.CacheReadInputTokens),
	}
}
