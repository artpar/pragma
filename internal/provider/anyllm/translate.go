// Package anyllm provides translation between gogent's internal model types
// and any-llm-go's provider types. Used by Google, OpenAI, and Groq adapters.
package anyllm

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
	"github.com/mozilla-ai/any-llm-go/providers"
)

// RequestToParams converts gogent's internal RequestParams to any-llm-go CompletionParams.
func RequestToParams(req provider.RequestParams) providers.CompletionParams {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	params := providers.CompletionParams{
		Model:    req.Model,
		Messages: MessagesToAnyLLM(req.System, req.Messages),
		Tools:    ToolsToAnyLLM(req.Tools),
		StreamOptions: &providers.StreamOptions{
			IncludeUsage: true,
		},
	}
	if req.MaxTokens > 0 {
		observe.GlobalTrace("if: req.MaxTokens > 0")
		params.MaxTokens = &req.MaxTokens
	}
	if req.Temperature != nil {
		observe.GlobalTrace("if: req.Temperature != nil")
		params.Temperature = req.Temperature
	}
	if req.Thinking != nil && req.Thinking.Enabled {
		observe.GlobalTrace("if: req.Thinking != nil && req.Thinking.Enabled")
		switch {
		case req.Thinking.BudgetTokens > 8192:
			observe.GlobalTrace("case: req.Thinking.BudgetTokens > 8192")
			params.ReasoningEffort = providers.ReasoningEffortHigh
		case req.Thinking.BudgetTokens > 1024:
			observe.GlobalTrace("case: req.Thinking.BudgetTokens > 1024")
			params.ReasoningEffort = providers.ReasoningEffortMedium
		default:
			observe.GlobalTrace("default")
			params.ReasoningEffort = providers.ReasoningEffortLow
		}
	}
	observe.GlobalTrace("return: params")
	return params
}

// MessagesToAnyLLM converts system prompt + internal messages to any-llm-go messages.
func MessagesToAnyLLM(system model.SystemPrompt, msgs []model.Message) []providers.Message {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var out []providers.Message

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
			out = append(out, providers.Message{
				Role:    providers.RoleSystem,
				Content: sb.String(),
			})
		}
	}

	toolNames := make(map[string]string)
	for _, m := range msgs {
		observe.GlobalTrace("range msgs")
		if m.Role == model.RoleAssistant {
			observe.GlobalTrace("if: m.Role == model.RoleAssistant")
			for _, p := range m.Content {
				observe.GlobalTrace("range m.Content")
				if tc, ok := p.(model.ToolCallPart); ok {
					observe.GlobalTrace("if: ok")
					toolNames[tc.ID] = tc.Name
				}
			}
		}
	}

	for _, m := range msgs {
		observe.GlobalTrace("range msgs")
		out = append(out, MessageToAnyLLM(m, toolNames)...)
	}
	observe.GlobalTrace("return: out")
	return out
}

// MessageToAnyLLM converts a single internal message to one or more any-llm-go messages.
func MessageToAnyLLM(m model.Message, toolNames map[string]string) []providers.Message {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	role := providers.RoleUser
	if m.Role == model.RoleAssistant {
		observe.GlobalTrace("if: m.Role == model.RoleAssistant")
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
		observe.GlobalTrace("range m.Content")
		switch part := p.(type) {
		case model.TextPart:
			observe.GlobalTrace("typecase: model.TextPart")
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
			observe.GlobalTrace("typecase: model.ImagePart")
			hasMultiPart = true
			parts = append(parts, providers.ContentPart{
				Type: "image_url",
				ImageURL: &providers.ImageURL{
					URL: dataURL(part.MimeType, part.Data),
				},
			})
		case model.ToolCallPart:
			observe.GlobalTrace("typecase: model.ToolCallPart")
			toolCalls = append(toolCalls, providers.ToolCall{
				ID:   part.ID,
				Type: "function",
				Function: providers.FunctionCall{
					Name:      part.Name,
					Arguments: string(part.Input),
				},
			})
		case model.ToolResultPart:
			observe.GlobalTrace("typecase: model.ToolResultPart")
			toolResults = append(toolResults, providers.Message{
				Role:       providers.RoleTool,
				Content:    part.Content,
				Name:       toolNames[part.ToolCallID],
				ToolCallID: part.ToolCallID,
			})
		case model.ThinkingPart:
			observe.GlobalTrace("typecase: model.ThinkingPart")
			reasoning = &providers.Reasoning{Content: part.Text}
		case model.DocumentPart:
			observe.GlobalTrace("typecase: model.DocumentPart")

			parts = append(parts, providers.ContentPart{
				Type: "text",
				Text: "[Document: " + part.MimeType + "]",
			})
			hasMultiPart = true
		}
	}

	var out []providers.Message

	hasContent := textContent != "" || hasMultiPart || len(toolCalls) > 0 || reasoning != nil
	if hasContent {
		observe.GlobalTrace("if: hasContent")
		msg := providers.Message{Role: role, Reasoning: reasoning}
		if len(toolCalls) > 0 {
			observe.GlobalTrace("if: len(toolCalls) > 0")
			msg.ToolCalls = toolCalls
		}
		if hasMultiPart {
			observe.GlobalTrace("if: hasMultiPart")
			if textContent != "" {
				observe.GlobalTrace("if: textContent != \"\"")
				parts = append([]providers.ContentPart{{Type: "text", Text: textContent}}, parts...)
			}
			msg.Content = parts
		} else {
			observe.GlobalTrace("else: hasMultiPart")
			msg.Content = textContent
		}
		out = append(out, msg)
	}

	out = append(out, toolResults...)
	observe.GlobalTrace("return: out")

	return out
}

// ToolsToAnyLLM converts gogent tool definitions to any-llm-go tools.
func ToolsToAnyLLM(tools []model.ToolDef) []providers.Tool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := make([]providers.Tool, len(tools))
	for i, t := range tools {
		observe.GlobalTrace("range tools")
		var params map[string]any
		if len(t.InputSchema) > 0 {
			observe.GlobalTrace("if: len(t.InputSchema) > 0")
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
	observe.GlobalTrace("return: out")
	return out
}

// ResponseFromCompletion converts an any-llm-go ChatCompletion to a gogent model.Response.
func ResponseFromCompletion(comp *providers.ChatCompletion) model.Response {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	resp := model.Response{Model: comp.Model}
	if comp.Usage != nil {
		observe.GlobalTrace("if: comp.Usage != nil")
		resp.Usage = UsageFromAnyLLM(comp.Usage)
	}
	if len(comp.Choices) == 0 {
		observe.GlobalTrace("if: len(comp.Choices) == 0")
		resp.StopReason = model.StopError
		observe.GlobalTrace("return: resp")
		return resp
	}
	choice := comp.Choices[0]
	resp.StopReason = StopReasonFromAnyLLM(choice.FinishReason)
	resp.Content = ContentFromMessage(choice.Message)

	if resp.StopReason == model.StopEndTurn {
		observe.GlobalTrace("if: resp.StopReason == model.StopEndTurn")
		for _, p := range resp.Content {
			observe.GlobalTrace("range resp.Content")
			if _, ok := p.(model.ToolCallPart); ok {
				observe.GlobalTrace("if: ok")
				resp.StopReason = model.StopToolUse
				break
			}
		}
	}
	observe.GlobalTrace("return: resp")
	return resp
}

// ContentFromMessage extracts gogent ContentParts from an any-llm-go Message.
func ContentFromMessage(msg providers.Message) []model.ContentPart {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var parts []model.ContentPart

	if msg.Reasoning != nil && msg.Reasoning.Content != "" {
		observe.GlobalTrace("if: msg.Reasoning != nil && msg.Reasoning.Content != \"\"")
		parts = append(parts, model.ThinkingPart{Text: msg.Reasoning.Content})
	}

	text := msg.ContentString()
	if text != "" {
		observe.GlobalTrace("if: text != \"\"")
		parts = append(parts, model.TextPart{Text: text})
	}

	for _, tc := range msg.ToolCalls {
		observe.GlobalTrace("range msg.ToolCalls")
		parts = append(parts, model.ToolCallPart{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: json.RawMessage(tc.Function.Arguments),
		})
	}
	observe.GlobalTrace("return: parts")

	return parts
}

// StopReasonFromAnyLLM maps any-llm-go finish reason to gogent StopReason.
func StopReasonFromAnyLLM(fr string) model.StopReason {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch fr {
	case providers.FinishReasonStop:
		observe.GlobalTrace("case: providers.FinishReasonStop")
		return model.StopEndTurn
	case providers.FinishReasonToolCalls:
		observe.GlobalTrace("case: providers.FinishReasonToolCalls")
		return model.StopToolUse
	case providers.FinishReasonLength:
		observe.GlobalTrace("case: providers.FinishReasonLength")
		return model.StopMaxTokens
	case providers.FinishReasonContentFilter:
		observe.GlobalTrace("case: providers.FinishReasonContentFilter")
		return model.StopError
	default:
		observe.GlobalTrace("default")
		return model.StopEndTurn
	}
}

// UsageFromAnyLLM converts any-llm-go Usage to gogent TokenUsage.
// Note: CacheCreationInputTokens and CacheReadInputTokens are NOT mapped
// because any-llm-go v0.9.0's Usage struct doesn't expose cache fields.
// This means cache efficiency is invisible for OpenAI/Groq providers.
func UsageFromAnyLLM(u *providers.Usage) model.TokenUsage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if u == nil {
		observe.GlobalTrace("if: u == nil")
		observe.GlobalTrace("return: model.TokenUsage{}")
		return model.TokenUsage{}
	}
	observe.GlobalTrace("return: model.TokenUsage{\n\tInputTokens:\tu.PromptTokens,\n\tOutputTokens:\tu.CompletionTo...")
	return model.TokenUsage{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
	}
}

// dataURL encodes binary data as a data: URL.
func dataURL(mimeType string, data []byte) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"data:\" + mimeType + \";base64,\" + base64.StdEncoding.EncodeToString(data)")
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
}
