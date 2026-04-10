package groq

import (
	"encoding/json"
	"fmt"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
)

// responseFromWire converts a Groq wire response to an internal model.Response.
func responseFromWire(resp *wireResponse, mapper *IDMapper, bus *observe.EventBus) model.Response {
	if len(resp.Choices) == 0 {
		return model.Response{
			Model:      resp.Model,
			StopReason: model.StopError,
			Usage:      usageFromWire(resp.Usage),
		}
	}

	choice := resp.Choices[0]
	var parts []model.ContentPart

	// Reasoning first (ThinkingPart)
	if choice.Message.Reasoning != "" {
		parts = append(parts, model.ThinkingPart{
			Text: choice.Message.Reasoning,
		})
	}

	// Text content
	if content, ok := choice.Message.Content.(string); ok && content != "" {
		parts = append(parts, model.TextPart{Text: content})
	}

	// Tool calls
	for _, tc := range choice.Message.ToolCalls {
		internalID := model.NewUUID()
		mapper.RegisterPair(internalID, tc.ID)

		args := normalizeArguments(tc.Function.Arguments, bus)
		parts = append(parts, model.ToolCallPart{
			ID:    internalID,
			Name:  tc.Function.Name,
			Input: json.RawMessage(args),
		})
	}

	return model.Response{
		ID:         resp.ID,
		Model:      resp.Model,
		Content:    parts,
		StopReason: stopReasonFromWire(choice.FinishReason),
		Usage:      usageFromWire(resp.Usage),
	}
}

// stopReasonFromWire maps Groq finish_reason to internal StopReason.
// Note: OpenAI-compatible APIs have no "pause_turn" concept (that's Anthropic-specific).
// "length" maps to StopMaxTokens — the agentic loop treats this as terminal.
func stopReasonFromWire(fr string) model.StopReason {
	switch fr {
	case "stop":
		return model.StopEndTurn
	case "tool_calls":
		return model.StopToolUse
	case "length":
		return model.StopMaxTokens
	default:
		return model.StopError
	}
}

// usageFromWire converts Groq wire usage to internal TokenUsage.
func usageFromWire(u wireUsage) model.TokenUsage {
	usage := model.TokenUsage{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
	}
	if u.PromptTokensDetails != nil {
		usage.CacheReadInputTokens = u.PromptTokensDetails.CachedTokens
	}
	return usage
}

// normalizeArguments ensures tool call arguments are valid JSON.
// Empty string or whitespace-only becomes "{}".
// Invalid JSON is replaced with "{}" and a warning is emitted.
func normalizeArguments(args string, bus *observe.EventBus) string {
	if args == "" || args == "null" {
		return "{}"
	}
	if !json.Valid([]byte(args)) {
		if bus != nil {
			bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
				Severity:     "warning",
				Component:    "groq/translate_in",
				ErrorType:    "malformed_tool_arguments",
				ErrorMessage: fmt.Sprintf("tool call arguments are not valid JSON (%d bytes), replaced with {}", len(args)),
			})
		}
		return "{}"
	}
	return args
}
