package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/provider/anyllm"
	"github.com/artpar/pragma/internal/provider/rawcapture"
	"github.com/artpar/pragma/internal/provider/shared"
	llmerrors "github.com/mozilla-ai/any-llm-go/errors"
	"github.com/mozilla-ai/any-llm-go/providers"
)

// wireMessage keeps OpenRouter's extension fields out of the OpenAI SDK's
// lossy message conversion. Details may include encrypted blocks and must not
// be reconstructed from the display text.
type wireMessage struct {
	Role             string               `json:"role"`
	Content          any                  `json:"content"`
	ToolCalls        []providers.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string               `json:"tool_call_id,omitempty"`
	Reasoning        *string              `json:"reasoning,omitempty"`
	ReasoningDetails json.RawMessage      `json:"reasoning_details,omitempty"`
}

func requestMessages(params provider.RequestParams) []wireMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var messages []wireMessage
	appendMessages := func(converted []providers.Message, details json.RawMessage) {
		for _, m := range converted {
			w := wireMessage{Role: m.Role, Content: m.Content, ToolCalls: m.ToolCalls, ToolCallID: m.ToolCallID}
			if m.Reasoning != nil {
				text := m.Reasoning.Content
				w.Reasoning = &text
			}
			if m.Role == providers.RoleAssistant {
				w.ReasoningDetails = details
			}
			messages = append(messages, w)
		}
	}
	appendMessages(anyllm.MessagesToAnyLLM(params.System, nil), nil)
	toolNames := make(map[string]string)
	for _, m := range params.Messages {
		observe.GlobalTrace("range params.Messages")
		for _, part := range m.Content {
			observe.GlobalTrace("range m.Content")
			if call, ok := part.(model.ToolCallPart); ok {
				observe.GlobalTrace("if: ok")
				toolNames[call.ID] = call.Name
			}
		}
	}
	for _, m := range params.Messages {
		observe.GlobalTrace("range params.Messages")
		var details json.RawMessage
		for _, part := range m.Content {
			observe.GlobalTrace("range m.Content")
			if thinking, ok := part.(model.ThinkingPart); ok && len(thinking.OpenRouterReasoningDetails) > 0 {
				observe.GlobalTrace("if: ok && len(thinking.OpenRouterReasoningDetails) > 0")
				details = thinking.OpenRouterReasoningDetails
			}
		}
		appendMessages(anyllm.MessageToAnyLLM(m, toolNames), details)
	}
	observe.GlobalTrace("return: messages")
	return messages
}

// Complete preserves OpenRouter reasoning on the nonstreaming provider-tools
// path. Streaming continues to use the embedded adapter.
func (p *Provider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "enter")
	defer observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "exit")
	traceID, spanID := observe.NewTraceID(), observe.NewSpanID()
	ctx = rawcapture.WithTrace(ctx, traceID, spanID)
	p.bus.Emit(shared.RequestStartedEvent(traceID, spanID, params))
	start := time.Now()
	llm := anyllm.RequestToParams(params)

	if llm.Model == "" {
		observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "if: llm.Model == \"\"")
		observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "return: model.Response{}, llmerrors.NewInvalidRequestError(\"\", fmt.Errorf(\"model is r...")
		return model.Response{}, llmerrors.NewInvalidRequestError("", fmt.Errorf("model is required"))
	}
	if len(llm.Messages) == 0 {
		observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "if: len(llm.Messages) == 0")
		observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "return: model.Response{}, llmerrors.NewInvalidRequestError(\"\", fmt.Errorf(\"at least o...")
		return model.Response{}, llmerrors.NewInvalidRequestError("", fmt.Errorf("at least one message is required"))
	}
	for _, m := range llm.Messages {
		observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "range llm.Messages")
		switch m.Role {
		case "system", "user", "assistant", "tool":
			observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "case: \"system\", \"user\", \"assistant\", \"tool\"")
		default:
			observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "default")
			return model.Response{}, llmerrors.NewInvalidRequestError("", fmt.Errorf("unsupported message role: %s", m.Role))
		}
	}

	body := map[string]any{"model": params.Model, "messages": requestMessages(params)}
	if llm.MaxTokens != nil {
		observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "if: llm.MaxTokens != nil")
		body["max_completion_tokens"] = *llm.MaxTokens
	}
	if llm.Temperature != nil {
		observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "if: llm.Temperature != nil")
		body["temperature"] = *llm.Temperature
	}
	if len(llm.Tools) > 0 {
		observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "if: len(llm.Tools) > 0")
		body["tools"] = llm.Tools
	}
	if llm.ReasoningEffort != "" && llm.ReasoningEffort != providers.ReasoningEffortNone {
		observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "if: llm.ReasoningEffort != \"\" && llm.ReasoningEffort != providers.ReasoningEffort...")
		body["reasoning_effort"] = llm.ReasoningEffort
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "if: err != nil")
		observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "return: model.Response{}, fmt.Errorf(\"openrouter encode request: %w\", err)")
		return model.Response{}, fmt.Errorf("openrouter encode request: %w", err)
	}
	var response model.Response
	err = shared.WithRetry(ctx, p.bus, 10, traceID, spanID, openrouterClassify, func() error {
		var requestErr error
		response, requestErr = p.completeWire(ctx, encoded)
		return requestErr
	})
	if err != nil {
		observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "if: err != nil")
		observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "return: model.Response{}, err")
		return model.Response{}, err
	}
	p.bus.Emit(observe.APIRequestCompleted{EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""), StopReason: response.StopReason, Usage: response.Usage, DurationMs: time.Since(start).Milliseconds(), Model: response.Model, Content: shared.MarshalContent(response.Content)})
	observe.TraceCtx(ctx, "openrouter", "Provider.Complete", "return: response, nil")
	return response, nil
}

func (p *Provider) completeWire(ctx context.Context, body []byte) (model.Response, error) {
	observe.TraceCtx(ctx, "openrouter", "Provider.completeWire", "enter")
	defer observe.TraceCtx(ctx, "openrouter", "Provider.completeWire", "exit")
	// Use the same SDK transport as the embedded adapter, including its retry
	// policy and HTTP error representation, while retaining extension fields.
	var data []byte
	if err := p.wireClient.Post(ctx, "chat/completions", json.RawMessage(body), &data); err != nil {
		observe.TraceCtx(ctx, "openrouter", "Provider.completeWire", "if: err != nil")
		observe.TraceCtx(ctx, "openrouter", "Provider.completeWire", "return: model.Response{}, p.Provider.ConvertError(err)")
		return model.Response{}, p.Provider.ConvertError(err)
	}
	var wire struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Index        int         `json:"index"`
			Message      wireMessage `json:"message"`
			FinishReason string      `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			providers.Usage
			CompletionTokensDetails struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		observe.TraceCtx(ctx, "openrouter", "Provider.completeWire", "if: err != nil")
		observe.TraceCtx(ctx, "openrouter", "Provider.completeWire", "return: model.Response{}, fmt.Errorf(\"openrouter decode response: %w\", err)")
		return model.Response{}, fmt.Errorf("openrouter decode response: %w", err)
	}
	if len(wire.Choices) == 0 {
		observe.TraceCtx(ctx, "openrouter", "Provider.completeWire", "if: len(wire.Choices) == 0")
		observe.TraceCtx(ctx, "openrouter", "Provider.completeWire", "return: model.Response{}, fmt.Errorf(\"openrouter response contained no choices\")")
		return model.Response{}, fmt.Errorf("openrouter response contained no choices")
	}
	completion := &providers.ChatCompletion{ID: wire.ID, Model: wire.Model}
	if wire.Usage != nil {
		observe.TraceCtx(ctx, "openrouter", "Provider.completeWire", "if: wire.Usage != nil")
		wire.Usage.ReasoningTokens = wire.Usage.CompletionTokensDetails.ReasoningTokens
		completion.Usage = &wire.Usage.Usage
	}
	for _, c := range wire.Choices {
		observe.TraceCtx(ctx, "openrouter", "Provider.completeWire", "range wire.Choices")
		m := providers.Message{Role: c.Message.Role, Content: c.Message.Content, ToolCalls: c.Message.ToolCalls}
		completion.Choices = append(completion.Choices, providers.Choice{Index: c.Index, Message: m, FinishReason: c.FinishReason})
	}
	response := anyllm.ResponseFromCompletion(completion)
	m := wire.Choices[0].Message
	if bytes.Equal(bytes.TrimSpace(m.ReasoningDetails), []byte("null")) {
		observe.TraceCtx(ctx, "openrouter", "Provider.completeWire", "if: bytes.Equal(bytes.TrimSpace(m.ReasoningDetails), []byte(\"null\"))")
		m.ReasoningDetails = nil
	}
	if m.Reasoning != nil || len(m.ReasoningDetails) > 0 {
		observe.TraceCtx(ctx, "openrouter", "Provider.completeWire", "if: m.Reasoning != nil || len(m.ReasoningDetails) > 0")
		thinking := model.ThinkingPart{OpenRouterReasoningDetails: append(json.RawMessage(nil), m.ReasoningDetails...)}
		if m.Reasoning != nil {
			observe.TraceCtx(ctx, "openrouter", "Provider.completeWire", "if: m.Reasoning != nil")
			thinking.Text = *m.Reasoning
		}
		response.Content = append([]model.ContentPart{thinking}, response.Content...)
	}
	observe.TraceCtx(ctx, "openrouter", "Provider.completeWire", "return: response, nil")
	return response, nil
}
