package groq

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
)

// buildWireRequest converts internal RequestParams to a Groq wire request.
func buildWireRequest(params provider.RequestParams, mapper *IDMapper, stream bool, bus *observe.EventBus) wireRequest {
	normalized := normalizeMessages(params.Messages)

	modelInfo, _ := LookupModel(params.Model)

	req := wireRequest{
		Model:    params.Model,
		Messages: messagesToWire(params.System, normalized, mapper, bus),
		Stream:   stream,
	}

	// Max tokens, capped at model limit
	maxTokens := params.MaxTokens
	if modelInfo.MaxOutput > 0 && maxTokens > modelInfo.MaxOutput {
		maxTokens = modelInfo.MaxOutput
	}
	if maxTokens > 0 {
		req.MaxCompletionTokens = maxTokens
	}

	// Temperature — suppress when reasoning is enabled (Groq recommends 0.5-0.7 for reasoning;
	// sending custom temperature with reasoning_format may cause errors)
	reasoningEnabled := params.Thinking != nil && params.Thinking.Enabled && modelInfo.SupportsReasoning
	if params.Temperature != nil && !reasoningEnabled {
		req.Temperature = params.Temperature
	}

	// Tools
	if len(params.Tools) > 0 {
		req.Tools = toolsToWire(params.Tools)
		req.ToolChoice = "auto"
		if modelInfo.ParallelTools {
			t := true
			req.ParallelToolCalls = &t
		}
	}

	// Reasoning config
	if reasoningEnabled {
		req.ReasoningFormat = "parsed"
		// Pick the best reasoning effort for this model
		if len(modelInfo.ReasoningEfforts) > 0 {
			req.ReasoningEffort = modelInfo.ReasoningEfforts[len(modelInfo.ReasoningEfforts)-1]
		}
	}

	// Stream options
	if stream {
		req.StreamOptions = &wireStreamOpt{IncludeUsage: true}
	}

	return req
}

// messagesToWire converts system prompt + internal messages to Groq wire messages.
func messagesToWire(system model.SystemPrompt, msgs []model.Message, mapper *IDMapper, bus *observe.EventBus) []wireMessage {
	var out []wireMessage

	// System prompt as first message
	if len(system.Blocks) > 0 {
		var sb strings.Builder
		for i, block := range system.Blocks {
			if i > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(block.Text)
		}
		if sb.Len() > 0 {
			out = append(out, wireMessage{Role: "system", Content: sb.String()})
		}
	}

	// Build toolCallID -> toolName map for tool result name resolution
	toolNameMap := buildToolNameMap(msgs)

	for _, m := range msgs {
		switch m.Role {
		case model.RoleAssistant:
			out = append(out, assistantToWire(m, mapper))
		case model.RoleUser:
			out = append(out, userToWire(m, mapper, toolNameMap, bus)...)
		}
	}

	return out
}

// buildToolNameMap scans all assistant messages and builds toolCallID -> toolName.
func buildToolNameMap(msgs []model.Message) map[string]string {
	names := make(map[string]string)
	for _, m := range msgs {
		if m.Role != model.RoleAssistant {
			continue
		}
		for _, p := range m.Content {
			if tc, ok := p.(model.ToolCallPart); ok {
				names[tc.ID] = tc.Name
			}
		}
	}
	return names
}

// assistantToWire converts an assistant message to a wire message.
func assistantToWire(m model.Message, mapper *IDMapper) wireMessage {
	msg := wireMessage{Role: "assistant"}

	var textParts []string
	var toolCalls []wireToolCall
	var reasoning string

	for _, p := range m.Content {
		switch part := p.(type) {
		case model.TextPart:
			textParts = append(textParts, part.Text)
		case model.ToolCallPart:
			wireID := mapper.ToWire(part.ID)
			args := string(part.Input)
			if args == "" || args == "null" {
				args = "{}"
			}
			toolCalls = append(toolCalls, wireToolCall{
				ID:   wireID,
				Type: "function",
				Function: wireFunction{
					Name:      part.Name,
					Arguments: args,
				},
			})
		case model.ThinkingPart:
			if part.Text != "" {
				reasoning = part.Text
			}
		case model.ImagePart, model.DocumentPart, model.ToolResultPart:
			// Not valid in assistant messages, skip
		}
	}

	if len(textParts) > 0 {
		msg.Content = strings.Join(textParts, "")
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}
	if reasoning != "" {
		msg.Reasoning = reasoning
	}

	return msg
}

// userToWire converts a user message to one or more wire messages.
// Tool results become separate role:"tool" messages.
func userToWire(m model.Message, mapper *IDMapper, toolNameMap map[string]string, bus *observe.EventBus) []wireMessage {
	var toolResults []wireMessage
	var contentParts []wireContentPart
	hasMultipart := false

	for _, p := range m.Content {
		switch part := p.(type) {
		case model.ToolResultPart:
			wireID := mapper.ToWire(part.ToolCallID)
			toolResults = append(toolResults, wireMessage{
				Role:       "tool",
				ToolCallID: wireID,
				Name:       toolNameMap[part.ToolCallID],
				Content:    part.Content,
			})
		case model.TextPart:
			contentParts = append(contentParts, wireContentPart{Type: "text", Text: part.Text})
		case model.ImagePart:
			hasMultipart = true
			dataURI := fmt.Sprintf("data:%s;base64,%s", part.MimeType, base64.StdEncoding.EncodeToString(part.Data))
			contentParts = append(contentParts, wireContentPart{
				Type:     "image_url",
				ImageURL: &wireImageURL{URL: dataURI},
			})
		case model.DocumentPart:
			if bus != nil {
				bus.Emit(observe.ErrorOccurred{
					EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
					Severity:     "warning",
					Component:    "groq/translate_out",
					ErrorType:    "unsupported_content",
					ErrorMessage: fmt.Sprintf("DocumentPart (%s) not supported by Groq, degraded to text placeholder", part.MimeType),
				})
			}
			contentParts = append(contentParts, wireContentPart{
				Type: "text",
				Text: fmt.Sprintf("[Document: %s, not supported by this provider]", part.MimeType),
			})
		case model.ThinkingPart:
			// Not valid in user messages, skip
		case model.ToolCallPart:
			// Not valid in user messages, skip
		}
	}

	var out []wireMessage

	// Tool results first (matching previous assistant's tool calls)
	out = append(out, toolResults...)

	// Then user content (if any)
	if len(contentParts) > 0 {
		msg := wireMessage{Role: "user"}
		if !hasMultipart && len(contentParts) == 1 && contentParts[0].Type == "text" {
			// Simple string content (most common case)
			msg.Content = contentParts[0].Text
		} else {
			msg.Content = contentParts
		}
		out = append(out, msg)
	}

	return out
}

// toolsToWire converts internal tool definitions to Groq wire format.
func toolsToWire(tools []model.ToolDef) []wireTool {
	out := make([]wireTool, len(tools))
	for i, t := range tools {
		out[i] = wireTool{
			Type: "function",
			Function: wireToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		}
	}
	return out
}

// prePopulateMapper walks history messages and registers synthetic wire IDs
// for all ToolCallParts that don't already have mappings, ensuring consistent
// IDs when history is sent back without clobbering real wire IDs from responses.
func prePopulateMapper(msgs []model.Message, mapper *IDMapper) {
	for _, m := range msgs {
		if m.Role != model.RoleAssistant {
			continue
		}
		for _, p := range m.Content {
			if tc, ok := p.(model.ToolCallPart); ok {
				// Only register if not already mapped (preserves real wire IDs from responses)
				if !mapper.HasInternal(tc.ID) {
					wireID := syntheticWireID(tc.ID)
					mapper.RegisterPair(tc.ID, wireID)
				}
			}
		}
	}
}
