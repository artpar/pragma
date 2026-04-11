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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	normalized := normalizeMessages(params.Messages)

	modelInfo, _ := LookupModel(params.Model)

	req := wireRequest{
		Model:    params.Model,
		Messages: messagesToWire(params.System, normalized, mapper, bus),
		Stream:   stream,
	}

	maxTokens := params.MaxTokens
	if modelInfo.MaxOutput > 0 && maxTokens > modelInfo.MaxOutput {
		observe.GlobalTrace("if: modelInfo.MaxOutput > 0 && maxTokens > modelInfo.MaxOutput")
		maxTokens = modelInfo.MaxOutput
	}
	if maxTokens > 0 {
		observe.GlobalTrace("if: maxTokens > 0")
		req.MaxCompletionTokens = maxTokens
	}

	reasoningEnabled := params.Thinking != nil && params.Thinking.Enabled && modelInfo.SupportsReasoning
	if params.Temperature != nil && !reasoningEnabled {
		observe.GlobalTrace("if: params.Temperature != nil && !reasoningEnabled")
		req.Temperature = params.Temperature
	}

	if len(params.Tools) > 0 {
		observe.GlobalTrace("if: len(params.Tools) > 0")
		req.Tools = toolsToWire(params.Tools)
		req.ToolChoice = "auto"
		if modelInfo.ParallelTools {
			observe.GlobalTrace("if: modelInfo.ParallelTools")
			t := true
			req.ParallelToolCalls = &t
		}
	}

	if reasoningEnabled {
		observe.GlobalTrace("if: reasoningEnabled")
		req.ReasoningFormat = "parsed"

		if len(modelInfo.ReasoningEfforts) > 0 {
			observe.GlobalTrace("if: len(modelInfo.ReasoningEfforts) > 0")
			req.ReasoningEffort = modelInfo.ReasoningEfforts[len(modelInfo.ReasoningEfforts)-1]
		}
	}

	if stream {
		observe.GlobalTrace("if: stream")
		req.StreamOptions = &wireStreamOpt{IncludeUsage: true}
	}
	observe.GlobalTrace("return: req")

	return req
}

// messagesToWire converts system prompt + internal messages to Groq wire messages.
func messagesToWire(system model.SystemPrompt, msgs []model.Message, mapper *IDMapper, bus *observe.EventBus) []wireMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var out []wireMessage

	if len(system.Blocks) > 0 {
		observe.GlobalTrace("if: len(system.Blocks) > 0")
		var sb strings.Builder
		for i, block := range system.Blocks {
			observe.GlobalTrace("range system.Blocks")
			if i > 0 {
				observe.GlobalTrace("if: i > 0")
				sb.WriteString("\n\n")
			}
			sb.WriteString(block.Text)
		}
		if sb.Len() > 0 {
			observe.GlobalTrace("if: sb.Len() > 0")
			out = append(out, wireMessage{Role: "system", Content: sb.String()})
		}
	}

	toolNameMap := buildToolNameMap(msgs)

	for _, m := range msgs {
		observe.GlobalTrace("range msgs")
		switch m.Role {
		case model.RoleAssistant:
			observe.GlobalTrace("case: model.RoleAssistant")
			out = append(out, assistantToWire(m, mapper))
		case model.RoleUser:
			observe.GlobalTrace("case: model.RoleUser")
			out = append(out, userToWire(m, mapper, toolNameMap, bus)...)
		}
	}
	observe.GlobalTrace("return: out")

	return out
}

// buildToolNameMap scans all assistant messages and builds toolCallID -> toolName.
func buildToolNameMap(msgs []model.Message) map[string]string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	names := make(map[string]string)
	for _, m := range msgs {
		observe.GlobalTrace("range msgs")
		if m.Role != model.RoleAssistant {
			observe.GlobalTrace("if: m.Role != model.RoleAssistant")
			continue
		}
		for _, p := range m.Content {
			observe.GlobalTrace("range m.Content")
			if tc, ok := p.(model.ToolCallPart); ok {
				observe.GlobalTrace("if: ok")
				names[tc.ID] = tc.Name
			}
		}
	}
	observe.GlobalTrace("return: names")
	return names
}

// assistantToWire converts an assistant message to a wire message.
func assistantToWire(m model.Message, mapper *IDMapper) wireMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	msg := wireMessage{Role: "assistant"}

	var textParts []string
	var toolCalls []wireToolCall
	var reasoning string

	for _, p := range m.Content {
		observe.GlobalTrace("range m.Content")
		switch part := p.(type) {
		case model.TextPart:
			observe.GlobalTrace("typecase: model.TextPart")
			textParts = append(textParts, part.Text)
		case model.ToolCallPart:
			observe.GlobalTrace("typecase: model.ToolCallPart")
			wireID := mapper.ToWire(part.ID)
			if wireID == "" {
				wireID = syntheticWireID(part.ID)
				mapper.RegisterPair(part.ID, wireID)
			}
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
			observe.GlobalTrace("typecase: model.ThinkingPart")
			if part.Text != "" {
				reasoning = part.Text
			}
		case model.ImagePart, model.DocumentPart, model.ToolResultPart:
			observe.GlobalTrace("typecase: model.ImagePart, model.DocumentPart, model.ToolResultPart")

		}
	}

	if len(textParts) > 0 {
		observe.GlobalTrace("if: len(textParts) > 0")
		msg.Content = strings.Join(textParts, "")
	}
	if len(toolCalls) > 0 {
		observe.GlobalTrace("if: len(toolCalls) > 0")
		msg.ToolCalls = toolCalls
	}
	if reasoning != "" {
		observe.GlobalTrace("if: reasoning != \"\"")
		msg.Reasoning = reasoning
	}
	observe.GlobalTrace("return: msg")

	return msg
}

// userToWire converts a user message to one or more wire messages.
// Tool results become separate role:"tool" messages.
func userToWire(m model.Message, mapper *IDMapper, toolNameMap map[string]string, bus *observe.EventBus) []wireMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var toolResults []wireMessage
	var contentParts []wireContentPart
	hasMultipart := false

	for _, p := range m.Content {
		observe.GlobalTrace("range m.Content")
		switch part := p.(type) {
		case model.ToolResultPart:
			observe.GlobalTrace("typecase: model.ToolResultPart")
			wireID := mapper.ToWire(part.ToolCallID)
			if wireID == "" {
				wireID = syntheticWireID(part.ToolCallID)
				mapper.RegisterPair(part.ToolCallID, wireID)
			}
			toolResults = append(toolResults, wireMessage{
				Role:       "tool",
				ToolCallID: wireID,
				Name:       toolNameMap[part.ToolCallID],
				Content:    part.Content,
			})
		case model.TextPart:
			observe.GlobalTrace("typecase: model.TextPart")
			contentParts = append(contentParts, wireContentPart{Type: "text", Text: part.Text})
		case model.ImagePart:
			observe.GlobalTrace("typecase: model.ImagePart")
			hasMultipart = true
			dataURI := fmt.Sprintf("data:%s;base64,%s", part.MimeType, base64.StdEncoding.EncodeToString(part.Data))
			contentParts = append(contentParts, wireContentPart{
				Type:     "image_url",
				ImageURL: &wireImageURL{URL: dataURI},
			})
		case model.DocumentPart:
			observe.GlobalTrace("typecase: model.DocumentPart")
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
			observe.GlobalTrace("typecase: model.ThinkingPart")

		case model.ToolCallPart:
			observe.GlobalTrace("typecase: model.ToolCallPart")

		}
	}

	var out []wireMessage

	out = append(out, toolResults...)

	if len(contentParts) > 0 {
		observe.GlobalTrace("if: len(contentParts) > 0")
		msg := wireMessage{Role: "user"}
		if !hasMultipart && len(contentParts) == 1 && contentParts[0].Type == "text" {
			observe.GlobalTrace("if: !hasMultipart && len(contentParts) == 1 && contentParts[0].Type == \"text\"")

			msg.Content = contentParts[0].Text
		} else {
			observe.GlobalTrace("else: !hasMultipart && len(contentParts) == 1 && contentParts[0].Type == \"text\"")
			msg.Content = contentParts
		}
		out = append(out, msg)
	}
	observe.GlobalTrace("return: out")

	return out
}

// toolsToWire converts internal tool definitions to Groq wire format.
func toolsToWire(tools []model.ToolDef) []wireTool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := make([]wireTool, len(tools))
	for i, t := range tools {
		observe.GlobalTrace("range tools")
		out[i] = wireTool{
			Type: "function",
			Function: wireToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		}
	}
	observe.GlobalTrace("return: out")
	return out
}

// prePopulateMapper walks history messages and registers synthetic wire IDs
// for all ToolCallParts that don't already have mappings, ensuring consistent
// IDs when history is sent back without clobbering real wire IDs from responses.
func prePopulateMapper(msgs []model.Message, mapper *IDMapper) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, m := range msgs {
		observe.GlobalTrace("range msgs")
		if m.Role != model.RoleAssistant {
			observe.GlobalTrace("if: m.Role != model.RoleAssistant")
			continue
		}
		for _, p := range m.Content {
			observe.GlobalTrace("range m.Content")
			if tc, ok := p.(model.ToolCallPart); ok {
				observe.GlobalTrace("if: ok")

				if !mapper.HasInternal(tc.ID) {
					observe.GlobalTrace("if: !mapper.HasInternal(tc.ID)")
					wireID := syntheticWireID(tc.ID)
					mapper.RegisterPair(tc.ID, wireID)
				}
			}
		}
	}
}
