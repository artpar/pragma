// Package groq implements provider.Provider for Groq via any-llm-go.
package groq

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/provider/anyllm"
	"github.com/artpar/pragma/internal/provider/shared"
	"github.com/mozilla-ai/any-llm-go/config"
	"github.com/mozilla-ai/any-llm-go/providers"
	groqprov "github.com/mozilla-ai/any-llm-go/providers/groq"
)

// Provider wraps any-llm-go's Groq provider for pragma.
type Provider struct {
	inner      providers.Provider
	bus        *observe.EventBus
	maxRetries int
}

// New creates a Groq provider backed by any-llm-go.
func New(apiKey string, bus *observe.EventBus) (*Provider, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	inner, err := groqprov.New(config.WithAPIKey(apiKey), config.WithTimeout(10*time.Minute))
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"groq: create provider: %w\", err)")
		return nil, fmt.Errorf("groq: create provider: %w", err)
	}
	observe.GlobalTrace("return: &Provider{inner: inner, bus: bus, maxRetries: 10}, nil")
	return &Provider{inner: inner, bus: bus, maxRetries: 10}, nil
}

func (p *Provider) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"groq\"")
	return "groq"
}

func (p *Provider) SupportsFeature(feature provider.Feature) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch feature {
	case provider.FeatureToolUse, provider.FeatureStreaming, provider.FeatureThinking:
		observe.GlobalTrace("case: provider.FeatureToolUse, provider.FeatureStreaming, provider.FeatureThinking")
		return true
	}
	observe.GlobalTrace("return: false")
	return false
}

func (p *Provider) Pricing(modelID string) (model.Pricing, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if info, ok := LookupModel(modelID); ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: info.Pricing, true")
		return info.Pricing, true
	}
	observe.GlobalTrace("return: model.Pricing{}, false")
	return model.Pricing{}, false
}

// ListModels returns sorted IDs of all known Groq models.
func (p *Provider) ListModels() []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: ListModels()")
	return ListModels()
}

func (p *Provider) ContextWindow(modelID string) (int, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if info, ok := LookupModel(modelID); ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: info.MaxContext, true")
		return info.MaxContext, true
	}
	observe.GlobalTrace("return: 131_072, false")
	return 131_072, false
}

// groqClassify classifies errors using HTTP status code string matching.
// Includes 529 (Groq-specific overloaded status).
var groqClassify = shared.ClassifyByStatusCodes([]string{"429", "500", "502", "503", "504", "529"})

func (p *Provider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	observe.TraceCtx(ctx, "groq", "Provider.Complete", "enter")
	defer observe.TraceCtx(ctx, "groq", "Provider.Complete", "exit")
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
		observe.TraceCtx(ctx, "groq", "Provider.Complete", "if: err != nil")
		observe.TraceCtx(ctx, "groq", "Provider.Complete", "return: model.Response{}, err")
		return model.Response{}, err
	}

	resp := anyllm.ResponseFromCompletion(comp)
	p.bus.Emit(observe.APIRequestCompleted{
		EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
		StopReason:  resp.StopReason, Usage: resp.Usage,
		DurationMs: time.Since(start).Milliseconds(), Model: resp.Model,
		Content: shared.MarshalContent(resp.Content),
	})
	observe.TraceCtx(ctx, "groq", "Provider.Complete", "return: resp, nil")
	return resp, nil
}

func (p *Provider) Stream(ctx context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	observe.TraceCtx(ctx, "groq", "Provider.Stream", "enter")
	defer observe.TraceCtx(ctx, "groq", "Provider.Stream", "exit")
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
		var accText strings.Builder
		accToolInputs := make(map[string]*strings.Builder)
		accToolNames := make(map[string]string)

		for chunk := range chunks {
			observe.TraceCtx(ctx, "groq", "Provider.Stream", "range chunks")
			if chunk.Usage != nil {
				observe.TraceCtx(ctx, "groq", "Provider.Stream", "if: chunk.Usage != nil")
				usage = anyllm.UsageFromAnyLLM(chunk.Usage)
			}
			if chunk.Model != "" {
				observe.TraceCtx(ctx, "groq", "Provider.Stream", "if: chunk.Model != \"\"")
				respModel = chunk.Model
			}
			for _, choice := range chunk.Choices {
				observe.TraceCtx(ctx, "groq", "Provider.Stream", "range chunk.Choices")
				delta := choice.Delta
				if delta.Content != "" {
					observe.TraceCtx(ctx, "groq", "Provider.Stream", "if: delta.Content != \"\"")
					accText.WriteString(delta.Content)
					ch <- provider.StreamChunk{TextDelta: delta.Content}
				}
				if delta.Reasoning != nil && delta.Reasoning.Content != "" {
					observe.TraceCtx(ctx, "groq", "Provider.Stream", "if: delta.Reasoning != nil && delta.Reasoning.Content != \"\"")
					ch <- provider.StreamChunk{ThinkingDelta: delta.Reasoning.Content}
				}
				for _, tc := range delta.ToolCalls {
					observe.TraceCtx(ctx, "groq", "Provider.Stream", "range delta.ToolCalls")
					if tc.ID != "" && !seenToolCalls[tc.ID] {
						observe.TraceCtx(ctx, "groq", "Provider.Stream", "if: tc.ID != \"\" && !seenToolCalls[tc.ID]")
						seenToolCalls[tc.ID] = true
						toolCallIDs = append(toolCallIDs, tc.ID)
						accToolInputs[tc.ID] = &strings.Builder{}
						accToolNames[tc.ID] = tc.Function.Name
						ch <- provider.StreamChunk{
							ToolCallStart: &model.ToolCallPart{ID: tc.ID, Name: tc.Function.Name},
						}
					}
					if tc.Function.Arguments != "" {
						observe.TraceCtx(ctx, "groq", "Provider.Stream", "if: tc.Function.Arguments != \"\"")
						id := tc.ID
						if id == "" && len(toolCallIDs) > 0 {
							observe.TraceCtx(ctx, "groq", "Provider.Stream", "if: id == \"\" && len(toolCallIDs) > 0")
							id = toolCallIDs[len(toolCallIDs)-1]
							observe.TraceCtx(ctx, "groq", "Provider.Stream", "warn: tool call input delta has no ID, falling back to last tool call")
						}
						if b, ok := accToolInputs[id]; ok {
							observe.TraceCtx(ctx, "groq", "Provider.Stream", "if: ok")
							b.WriteString(tc.Function.Arguments)
						}
						ch <- provider.StreamChunk{
							ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: id, JSONDelta: tc.Function.Arguments},
						}
					}
				}
				if choice.FinishReason != "" {
					observe.TraceCtx(ctx, "groq", "Provider.Stream", "if: choice.FinishReason != \"\"")
					stopReason := anyllm.StopReasonFromAnyLLM(choice.FinishReason)
					if stopReason == model.StopEndTurn && len(seenToolCalls) > 0 {
						observe.TraceCtx(ctx, "groq", "Provider.Stream", "if: stopReason == model.StopEndTurn && len(seenToolCalls) > 0")
						stopReason = model.StopToolUse
					}
					ch <- provider.StreamChunk{
						Done: &provider.StreamDone{StopReason: stopReason, Usage: usage, Model: respModel},
					}
					var accContent []model.ContentPart
					if accText.Len() > 0 {
						observe.TraceCtx(ctx, "groq", "Provider.Stream", "if: accText.Len() > 0")
						accContent = append(accContent, model.TextPart{Text: accText.String()})
					}
					for _, id := range toolCallIDs {
						observe.TraceCtx(ctx, "groq", "Provider.Stream", "range toolCallIDs")
						tc := model.ToolCallPart{ID: id, Name: accToolNames[id]}
						if b, ok := accToolInputs[id]; ok {
							observe.TraceCtx(ctx, "groq", "Provider.Stream", "if: ok")
							tc.Input = json.RawMessage(b.String())
						}
						accContent = append(accContent, tc)
					}
					p.bus.Emit(observe.APIRequestCompleted{
						EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
						StopReason:  stopReason, Usage: usage,
						DurationMs: time.Since(start).Milliseconds(), Model: respModel,
						Content: shared.MarshalContent(accContent),
					})
				}
			}
		}
		select {
		case err, ok := <-errs:
			observe.TraceCtx(ctx, "groq", "Provider.Stream", "select: err, ok := <-errs")
			if ok && err != nil {
				ch <- provider.StreamChunk{Error: err}
			}
		default:
			observe.TraceCtx(ctx, "groq", "Provider.Stream", "select: default")
		}
	}()
	observe.TraceCtx(ctx, "groq", "Provider.Stream", "return: ch, nil")
	return ch, nil
}

func (p *Provider) emitStart(traceID, spanID string, params provider.RequestParams) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	p.bus.Emit(shared.RequestStartedEvent(traceID, spanID, params))
}
