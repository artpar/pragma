// Package groq implements provider.Provider for Groq via any-llm-go.
package groq

import (
	"context"
	"fmt"
	"time"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
	"github.com/artpar/gogent/internal/provider/anyllm"
	"github.com/artpar/gogent/internal/provider/shared"
	"github.com/mozilla-ai/any-llm-go/config"
	"github.com/mozilla-ai/any-llm-go/providers"
	groqprov "github.com/mozilla-ai/any-llm-go/providers/groq"
)

// Provider wraps any-llm-go's Groq provider for gogent.
type Provider struct {
	inner      providers.Provider
	bus        *observe.EventBus
	maxRetries int
}

// New creates a Groq provider backed by any-llm-go.
func New(apiKey string, bus *observe.EventBus) (*Provider, error) {
	inner, err := groqprov.New(config.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("groq: create provider: %w", err)
	}
	return &Provider{inner: inner, bus: bus, maxRetries: 10}, nil
}

func (p *Provider) Name() string { return "groq" }

func (p *Provider) SupportsFeature(feature provider.Feature) bool {
	switch feature {
	case provider.FeatureToolUse, provider.FeatureStreaming, provider.FeatureThinking:
		return true
	}
	return false
}

func (p *Provider) Pricing(modelID string) (model.Pricing, bool) {
	if info, ok := LookupModel(modelID); ok {
		return info.Pricing, true
	}
	return model.Pricing{}, false
}

func (p *Provider) ContextWindow(modelID string) (int, bool) {
	if info, ok := LookupModel(modelID); ok {
		return info.MaxContext, true
	}
	return 131_072, false
}

// groqClassify classifies errors using HTTP status code string matching.
// Includes 529 (Groq-specific overloaded status).
var groqClassify = shared.ClassifyByStatusCodes([]string{"429", "500", "502", "503", "504", "529"})

func (p *Provider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	traceID := observe.NewTraceID()
	spanID := observe.NewSpanID()
	p.emitStart(traceID, spanID, params)
	start := time.Now()

	llmParams := anyllm.RequestToParams(params)
	var comp *providers.ChatCompletion
	err := shared.WithRetry(ctx, p.bus, p.maxRetries, traceID, spanID, groqClassify, func() error {
		var reqErr error
		comp, reqErr = p.inner.Completion(ctx, llmParams)
		return reqErr
	})
	if err != nil {
		return model.Response{}, err
	}

	resp := anyllm.ResponseFromCompletion(comp)
	p.bus.Emit(observe.APIRequestCompleted{
		EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
		StopReason:  resp.StopReason, Usage: resp.Usage,
		DurationMs: time.Since(start).Milliseconds(), Model: resp.Model,
	})
	return resp, nil
}

func (p *Provider) Stream(ctx context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	traceID := observe.NewTraceID()
	spanID := observe.NewSpanID()
	p.emitStart(traceID, spanID, params)

	llmParams := anyllm.RequestToParams(params)
	chunks, errs := p.inner.CompletionStream(ctx, llmParams)

	ch := make(chan provider.StreamChunk, 32)
	go func() {
		defer close(ch)
		start := time.Now()
		var usage model.TokenUsage
		var respModel string
		var toolCallIDs []string
		seenToolCalls := make(map[string]bool)

		for chunk := range chunks {
			if chunk.Usage != nil {
				usage = anyllm.UsageFromAnyLLM(chunk.Usage)
			}
			if chunk.Model != "" {
				respModel = chunk.Model
			}
			for _, choice := range chunk.Choices {
				delta := choice.Delta
				if delta.Content != "" {
					ch <- provider.StreamChunk{TextDelta: delta.Content}
				}
				if delta.Reasoning != nil && delta.Reasoning.Content != "" {
					ch <- provider.StreamChunk{ThinkingDelta: delta.Reasoning.Content}
				}
				for _, tc := range delta.ToolCalls {
					if tc.ID != "" && !seenToolCalls[tc.ID] {
						seenToolCalls[tc.ID] = true
						toolCallIDs = append(toolCallIDs, tc.ID)
						ch <- provider.StreamChunk{
							ToolCallStart: &model.ToolCallPart{ID: tc.ID, Name: tc.Function.Name},
						}
					}
					if tc.Function.Arguments != "" {
						id := tc.ID
						if id == "" && len(toolCallIDs) > 0 {
							id = toolCallIDs[len(toolCallIDs)-1]
						}
						ch <- provider.StreamChunk{
							ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: id, JSONDelta: tc.Function.Arguments},
						}
					}
				}
				if choice.FinishReason != "" {
					stopReason := anyllm.StopReasonFromAnyLLM(choice.FinishReason)
					if stopReason == model.StopEndTurn && len(seenToolCalls) > 0 {
						stopReason = model.StopToolUse
					}
					ch <- provider.StreamChunk{
						Done: &provider.StreamDone{StopReason: stopReason, Usage: usage, Model: respModel},
					}
					p.bus.Emit(observe.APIRequestCompleted{
						EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
						StopReason:  stopReason, Usage: usage,
						DurationMs: time.Since(start).Milliseconds(), Model: respModel,
					})
				}
			}
		}
		select {
		case err, ok := <-errs:
			if ok && err != nil {
				ch <- provider.StreamChunk{Error: err}
			}
		default:
		}
	}()
	return ch, nil
}

func (p *Provider) emitStart(traceID, spanID string, params provider.RequestParams) {
	p.bus.Emit(observe.APIRequestStarted{
		EventHeader:   observe.NewEventHeader("APIRequestStarted", traceID, spanID, ""),
		Model:         params.Model, MessageCount: len(params.Messages),
		ToolCount:     len(params.Tools), TokenEstimate: shared.EstimateTokens(params),
	})
}
