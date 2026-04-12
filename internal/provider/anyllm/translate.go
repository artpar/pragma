// Package anyllm provides translation between gogent's internal model types
// and any-llm-go's provider types. Used by Google, OpenAI, and Groq adapters.
package anyllm

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/provider"
	"github.com/mozilla-ai/any-llm-go/providers"
)

// RequestToParams converts gogent's internal RequestParams to any-llm-go CompletionParams.
func RequestToParams(req provider.RequestParams) providers.CompletionParams {
	params := providers.CompletionParams{
		Model:    req.Model,
		Messages: MessagesToAnyLLM(req.System, req.Messages),
		Tools:    ToolsToAnyLLM(req.Tools),
		StreamOptions: &providers.StreamOptions{
			IncludeUsage: true,
		},
	}
	if req.MaxTokens > 0 {
		params.MaxTokens = &req.MaxTokens
	}
	if req.Temperature != nil {
		params.Temperature = req.Temperature
	}
	if req.Thinking != nil && req.Thinking.Enabled {
		switch {
		case req.Thinking.BudgetTokens > 8192:
			params.ReasoningEffort = providers.ReasoningEffortHigh
		case req.Thinking.BudgetTokens > 1024:
			params.ReasoningEffort = providers.ReasoningEffortMedium
		default:
			params.ReasoningEffort = providers.ReasoningEffortLow
		}
	}
	return params
}

// MessagesToAnyLLM converts system prompt + internal messages to any-llm-go messages.
func MessagesToAnyLLM(system model.SystemPrompt, msgs []model.Message) []providers.Message {
	var out []providers.Message

	// System prompt → system message
	if len(system.Blocks) > 0 {
		var sb strings.Builder
		for i, block := range system.Blocks {
			if i > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(block.Text)
		}
		if sb.Len() > 0 {
			out = append(out, providers.Message{
				Role:    providers.RoleSystem,
				Content: sb.String(),
			})
		}
	}

	// Build tool call ID → name map for tool results
	toolNames := make(map[string]string)
	for _, m := range msgs {
		if m.Role == model.RoleAssistant {
			for _, p := range m.Content {
				if tc, ok := p.(model.ToolCallPart); ok {
					toolNames[tc.ID] = tc.Name
				}
			}
		}
	}

	for _, m := range msgs {
		out = append(out, MessageToAnyLLM(m, toolNames)...)
	}
	return out
}

// MessageToAnyLLM converts a single internal message to one or more any-llm-go messages.
func MessageToAnyLLM(m model.Message, toolNames map[string]string) []providers.Message {
	role := providers.RoleUser
	if m.Role == model.RoleAssistant {
		role = providers.RoleAssistant
	}

	// Collect tool calls and tool results separately
	var toolCalls []providers.ToolCall
	var toolResults []providers.Message
	var parts []providers.ContentPart
	var textContent string
	var reasoning *providers.Reasoning
	hasMultiPart := false

	for _, p := range m.Content {
		switch part := p.(type) {
		case model.TextPart:
			if len(parts) == 0 && !hasMultiPart {
				textContent = part.Text
			} else {
				hasMultiPart = true
				parts = append(parts, providers.ContentPart{
					Type: "text",
					Text: part.Text,
				})
			}
		case model.ImagePart:
			hasMultiPart = true
			parts = append(parts, providers.ContentPart{
				Type: "image_url",
				ImageURL: &providers.ImageURL{
					URL: dataURL(part.MimeType, part.Data),
				},
			})
		case model.ToolCallPart:
			toolCalls = append(toolCalls, providers.ToolCall{
				ID:   part.ID,
				Type: "function",
				Function: providers.FunctionCall{
					Name:      part.Name,
					Arguments: string(part.Input),
				},
			})
		case model.ToolResultPart:
			toolResults = append(toolResults, providers.Message{
				Role:       providers.RoleTool,
				Content:    part.Content,
				Name:       toolNames[part.ToolCallID],
				ToolCallID: part.ToolCallID,
			})
		case model.ThinkingPart:
			reasoning = &providers.Reasoning{Content: part.Text}
		case model.DocumentPart:
			// any-llm-go doesn't support documents; fall back to text note
			parts = append(parts, providers.ContentPart{
				Type: "text",
				Text: "[Document: " + part.MimeType + "]",
			})
			hasMultiPart = true
		}
	}

	var out []providers.Message

	// Build the main message — only if it has meaningful content
	hasContent := textContent != "" || hasMultiPart || len(toolCalls) > 0 || reasoning != nil
	if hasContent {
		msg := providers.Message{Role: role, Reasoning: reasoning}
		if len(toolCalls) > 0 {
			msg.ToolCalls = toolCalls
		}
		if hasMultiPart {
			if textContent != "" {
				parts = append([]providers.ContentPart{{Type: "text", Text: textContent}}, parts...)
			}
			msg.Content = parts
		} else {
			msg.Content = textContent
		}
		out = append(out, msg)
	}

	// Tool results as separate messages
	out = append(out, toolResults...)

	return out
}

// ToolsToAnyLLM converts gogent tool definitions to any-llm-go tools.
func ToolsToAnyLLM(tools []model.ToolDef) []providers.Tool {
	out := make([]providers.Tool, len(tools))
	for i, t := range tools {
		var params map[string]any
		if len(t.InputSchema) > 0 {
			_ = json.Unmarshal(t.InputSchema, &params)
		}
		out[i] = providers.Tool{
			Type: "function",
			Function: providers.Function{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		}
	}
	return out
}

// ResponseFromCompletion converts an any-llm-go ChatCompletion to a gogent model.Response.
func ResponseFromCompletion(comp *providers.ChatCompletion) model.Response {
	resp := model.Response{Model: comp.Model}
	if comp.Usage != nil {
		resp.Usage = UsageFromAnyLLM(comp.Usage)
	}
	if len(comp.Choices) == 0 {
		resp.StopReason = model.StopError
		return resp
	}
	choice := comp.Choices[0]
	resp.StopReason = StopReasonFromAnyLLM(choice.FinishReason)
	resp.Content = ContentFromMessage(choice.Message)

	// Override stop reason if tool calls present
	if resp.StopReason == model.StopEndTurn {
		for _, p := range resp.Content {
			if _, ok := p.(model.ToolCallPart); ok {
				resp.StopReason = model.StopToolUse
				break
			}
		}
	}
	return resp
}

// ContentFromMessage extracts gogent ContentParts from an any-llm-go Message.
func ContentFromMessage(msg providers.Message) []model.ContentPart {
	var parts []model.ContentPart

	// Reasoning → ThinkingPart
	if msg.Reasoning != nil && msg.Reasoning.Content != "" {
		parts = append(parts, model.ThinkingPart{Text: msg.Reasoning.Content})
	}

	// Text content
	text := msg.ContentString()
	if text != "" {
		parts = append(parts, model.TextPart{Text: text})
	}

	// Tool calls
	for _, tc := range msg.ToolCalls {
		parts = append(parts, model.ToolCallPart{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: json.RawMessage(tc.Function.Arguments),
		})
	}

	return parts
}

// StopReasonFromAnyLLM maps any-llm-go finish reason to gogent StopReason.
func StopReasonFromAnyLLM(fr string) model.StopReason {
	switch fr {
	case providers.FinishReasonStop:
		return model.StopEndTurn
	case providers.FinishReasonToolCalls:
		return model.StopToolUse
	case providers.FinishReasonLength:
		return model.StopMaxTokens
	case providers.FinishReasonContentFilter:
		return model.StopError
	default:
		return model.StopEndTurn
	}
}

// UsageFromAnyLLM converts any-llm-go Usage to gogent TokenUsage.
func UsageFromAnyLLM(u *providers.Usage) model.TokenUsage {
	if u == nil {
		return model.TokenUsage{}
	}
	return model.TokenUsage{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
	}
}

// dataURL encodes binary data as a data: URL.
func dataURL(mimeType string, data []byte) string {
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
}
