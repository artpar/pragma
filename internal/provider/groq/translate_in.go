package groq

import (
	"encoding/json"
	"fmt"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
)

// responseFromWire converts a Groq wire response to an internal model.Response.
func responseFromWire(resp *wireResponse, mapper *IDMapper, bus *observe.EventBus) model.Response {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(resp.Choices) == 0 {
		observe.GlobalTrace("if: len(resp.Choices) == 0")
		observe.GlobalTrace("return: model.Response{\n\tModel:\t\tresp.Model,\n\tStopReason:\tmodel.StopError,\n\tUsage:\t\tu...")
		return model.Response{
			Model:      resp.Model,
			StopReason: model.StopError,
			Usage:      usageFromWire(resp.Usage),
		}
	}

	choice := resp.Choices[0]
	var parts []model.ContentPart

	if choice.Message.Reasoning != "" {
		observe.GlobalTrace("if: choice.Message.Reasoning != \"\"")
		parts = append(parts, model.ThinkingPart{
			Text: choice.Message.Reasoning,
		})
	}

	if content, ok := choice.Message.Content.(string); ok && content != "" {
		observe.GlobalTrace("if: ok && content != \"\"")
		parts = append(parts, model.TextPart{Text: content})
	}

	for _, tc := range choice.Message.ToolCalls {
		observe.GlobalTrace("range choice.Message.ToolCalls")
		internalID := model.NewUUID()
		mapper.RegisterPair(internalID, tc.ID)

		args := normalizeArguments(tc.Function.Arguments, bus)
		parts = append(parts, model.ToolCallPart{
			ID:    internalID,
			Name:  tc.Function.Name,
			Input: json.RawMessage(args),
		})
	}
	observe.GlobalTrace("return: model.Response{\n\tID:\t\tresp.ID,\n\tModel:\t\tresp.Model,\n\tContent:\tparts,\n\tStopRea...")

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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch fr {
	case "stop":
		observe.GlobalTrace("case: \"stop\"")
		return model.StopEndTurn
	case "tool_calls":
		observe.GlobalTrace("case: \"tool_calls\"")
		return model.StopToolUse
	case "length":
		observe.GlobalTrace("case: \"length\"")
		return model.StopMaxTokens
	default:
		observe.GlobalTrace("default")
		return model.StopError
	}
}

// usageFromWire converts Groq wire usage to internal TokenUsage.
func usageFromWire(u wireUsage) model.TokenUsage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	usage := model.TokenUsage{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
	}
	if u.PromptTokensDetails != nil && u.PromptTokensDetails.CachedTokens > 0 {
		observe.GlobalTrace("if: u.PromptTokensDetails != nil && CachedTokens > 0")
		usage.CacheReadInputTokens = u.PromptTokensDetails.CachedTokens
		// Groq's prompt_tokens includes cached tokens. Subtract to avoid
		// double-counting: cost = (non_cached * input_rate) + (cached * cache_rate).
		usage.InputTokens -= usage.CacheReadInputTokens
	}
	observe.GlobalTrace("return: usage")
	return usage
}

// normalizeArguments ensures tool call arguments are valid JSON.
// Empty string or whitespace-only becomes "{}".
// Invalid JSON is replaced with "{}" and a warning is emitted.
func normalizeArguments(args string, bus *observe.EventBus) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if args == "" || args == "null" {
		observe.GlobalTrace("if: args == \"\" || args == \"null\"")
		observe.GlobalTrace("return: \"{}\"")
		return "{}"
	}
	if !json.Valid([]byte(args)) {
		observe.GlobalTrace("if: !json.Valid([]byte(args))")
		if bus != nil {
			observe.GlobalTrace("if: bus != nil")
			bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
				Severity:     "warning",
				Component:    "groq/translate_in",
				ErrorType:    "malformed_tool_arguments",
				ErrorMessage: fmt.Sprintf("tool call arguments are not valid JSON (%d bytes), replaced with {}", len(args)),
			})
		}
		observe.GlobalTrace("return: \"{}\"")
		return "{}"
	}
	observe.GlobalTrace("return: args")
	return args
}
