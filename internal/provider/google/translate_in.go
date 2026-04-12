package google

import (
	"encoding/json"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
)

// responseFromWire converts a Google Gemini wire response to an internal model.Response.
func responseFromWire(resp *wireResponse, mapper *IDMapper, bus *observe.EventBus) model.Response {
	if len(resp.Candidates) == 0 {
		return model.Response{
			Model:      resp.ModelVersion,
			StopReason: model.StopError,
			Usage:      usageFromWire(resp.UsageMetadata),
		}
	}

	candidate := resp.Candidates[0]
	parts := partsFromWire(candidate.Content.Parts, mapper, bus)

	stopReason := stopReasonFromWire(candidate.FinishReason)
	// If we have function calls but stop reason is STOP, it's actually tool use
	if stopReason == model.StopEndTurn {
		for _, p := range parts {
			if _, ok := p.(model.ToolCallPart); ok {
				stopReason = model.StopToolUse
				break
			}
		}
	}

	return model.Response{
		Model:      resp.ModelVersion,
		Content:    parts,
		StopReason: stopReason,
		Usage:      usageFromWire(resp.UsageMetadata),
	}
}

// partsFromWire converts Google wire parts to internal ContentParts.
func partsFromWire(wireParts []wirePart, mapper *IDMapper, bus *observe.EventBus) []model.ContentPart {
	var parts []model.ContentPart

	for _, wp := range wireParts {
		if wp.FunctionCall != nil {
			internalID := model.NewUUID()
			// Gemini 3 provides a unique id per function call; use it as the wire ID.
			// Older models don't provide id, so fall back to positional.
			wireID := wp.FunctionCall.Id
			if wireID == "" {
				wireID = mapper.NextWireID(wp.FunctionCall.Name)
			}
			mapper.RegisterPair(internalID, wireID)

			args := normalizeArguments(wp.FunctionCall.Args, bus)
			parts = append(parts, model.ToolCallPart{
				ID:    internalID,
				Name:  wp.FunctionCall.Name,
				Input: args,
			})
			continue
		}

		if wp.Text != "" {
			if wp.Thought != nil && *wp.Thought {
				tp := model.ThinkingPart{Text: wp.Text}
				// Gemini 3: thoughtSignature must round-trip for function calling
				if wp.ThoughtSignature != "" {
					tp.Signature = wp.ThoughtSignature
				}
				parts = append(parts, tp)
			} else {
				parts = append(parts, model.TextPart{Text: wp.Text})
			}
			continue
		}
	}

	return parts
}

// stopReasonFromWire maps Google Gemini finishReason to internal StopReason.
// Source: https://ai.google.dev/api/rest/v1beta/Candidate
func stopReasonFromWire(fr string) model.StopReason {
	switch fr {
	case "STOP":
		return model.StopEndTurn
	case "MAX_TOKENS":
		return model.StopMaxTokens
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII",
		"MALFORMED_FUNCTION_CALL", "OTHER":
		return model.StopError
	default:
		return model.StopError
	}
}

// usageFromWire converts Google wire usage metadata to internal TokenUsage.
// Thinking tokens are folded into OutputTokens since model.TokenUsage has no
// separate thinking field.
func usageFromWire(u wireUsageMetadata) model.TokenUsage {
	return model.TokenUsage{
		InputTokens:          u.PromptTokenCount,
		OutputTokens:         u.CandidatesTokenCount + u.ThoughtsTokenCount,
		CacheReadInputTokens: u.CachedContentTokenCount,
	}
}

// normalizeArguments ensures tool call arguments are valid JSON.
func normalizeArguments(args json.RawMessage, bus *observe.EventBus) json.RawMessage {
	if len(args) == 0 || string(args) == "null" {
		return json.RawMessage("{}")
	}
	if !json.Valid(args) {
		if bus != nil {
			bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
				Severity:     "warning",
				Component:    "google/translate_in",
				ErrorType:    "malformed_tool_arguments",
				ErrorMessage: "tool call arguments are not valid JSON, replaced with {}",
			})
		}
		return json.RawMessage("{}")
	}
	return args
}
