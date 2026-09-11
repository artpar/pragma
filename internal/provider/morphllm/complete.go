package morphllm

import (
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

// wireMessage retains Morph's reasoning_content extension across agent turns.
type wireMessage struct {
	Role             string               `json:"role"`
	Content          any                  `json:"content"`
	ToolCalls        []providers.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string               `json:"tool_call_id,omitempty"`
	ReasoningContent *string              `json:"reasoning_content,omitempty"`
}

func requestMessages(params provider.RequestParams) []wireMessage {
	var messages []wireMessage
	appendConverted := func(converted []providers.Message) {
		for _, message := range converted {
			wire := wireMessage{
				Role: message.Role, Content: message.Content,
				ToolCalls: message.ToolCalls, ToolCallID: message.ToolCallID,
			}
			if message.Reasoning != nil {
				text := message.Reasoning.Content
				wire.ReasoningContent = &text
			}
			messages = append(messages, wire)
		}
	}
	appendConverted(anyllm.MessagesToAnyLLM(params.System, nil))
	toolNames := make(map[string]string)
	for _, message := range params.Messages {
		for _, part := range message.Content {
			if call, ok := part.(model.ToolCallPart); ok {
				toolNames[call.ID] = call.Name
			}
		}
	}
	for _, message := range params.Messages {
		appendConverted(anyllm.MessageToAnyLLM(message, toolNames))
	}
	return messages
}

// Complete preserves Morph reasoning on the non-streaming provider-tools path.
func (p *Provider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	traceID, spanID := observe.NewTraceID(), observe.NewSpanID()
	ctx = rawcapture.WithTrace(ctx, traceID, spanID)
	if p.bus != nil {
		p.bus.Emit(shared.RequestStartedEvent(traceID, spanID, params))
	}
	started := time.Now()
	llm := anyllm.RequestToParams(params)
	if llm.Model == "" {
		return model.Response{}, llmerrors.NewInvalidRequestError("", fmt.Errorf("model is required"))
	}
	if len(llm.Messages) == 0 {
		return model.Response{}, llmerrors.NewInvalidRequestError("", fmt.Errorf("at least one message is required"))
	}
	for _, message := range llm.Messages {
		switch message.Role {
		case "system", "user", "assistant", "tool":
		default:
			return model.Response{}, llmerrors.NewInvalidRequestError("", fmt.Errorf("unsupported message role: %s", message.Role))
		}
	}

	body := map[string]any{"model": params.Model, "messages": requestMessages(params)}
	if llm.MaxTokens != nil {
		body["max_tokens"] = *llm.MaxTokens
	}
	if llm.Temperature != nil {
		body["temperature"] = *llm.Temperature
	}
	if len(llm.Tools) > 0 {
		body["tools"] = llm.Tools
	}
	if llm.ReasoningEffort != "" && llm.ReasoningEffort != providers.ReasoningEffortNone {
		body["reasoning_effort"] = llm.ReasoningEffort
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return model.Response{}, fmt.Errorf("morphllm encode request: %w", err)
	}

	var response model.Response
	err = shared.WithRetry(ctx, p.bus, 10, traceID, spanID, morphClassify, func() error {
		var requestErr error
		response, requestErr = p.completeWire(ctx, encoded)
		return requestErr
	})
	if err != nil {
		return model.Response{}, err
	}
	if p.bus != nil {
		p.bus.Emit(observe.APIRequestCompleted{
			EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
			StopReason:  response.StopReason, Usage: response.Usage,
			DurationMs: time.Since(started).Milliseconds(), Model: response.Model,
			Content: shared.MarshalContent(response.Content),
		})
	}
	return response, nil
}

func (p *Provider) completeWire(ctx context.Context, body []byte) (model.Response, error) {
	var data []byte
	if err := p.wireClient.Post(ctx, "chat/completions", json.RawMessage(body), &data); err != nil {
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
		return model.Response{}, fmt.Errorf("morphllm decode response: %w", err)
	}
	if len(wire.Choices) == 0 {
		return model.Response{}, fmt.Errorf("morphllm response contained no choices")
	}
	completion := &providers.ChatCompletion{ID: wire.ID, Model: wire.Model}
	if wire.Usage != nil {
		wire.Usage.ReasoningTokens = wire.Usage.CompletionTokensDetails.ReasoningTokens
		completion.Usage = &wire.Usage.Usage
	}
	for _, choice := range wire.Choices {
		message := providers.Message{
			Role: choice.Message.Role, Content: choice.Message.Content,
			ToolCalls: choice.Message.ToolCalls,
		}
		completion.Choices = append(completion.Choices, providers.Choice{
			Index: choice.Index, Message: message, FinishReason: choice.FinishReason,
		})
	}
	response := anyllm.ResponseFromCompletion(completion)
	reasoning := wire.Choices[0].Message.ReasoningContent
	if reasoning != nil {
		response.Content = append([]model.ContentPart{model.ThinkingPart{Text: *reasoning}}, response.Content...)
	}
	return response, nil
}
