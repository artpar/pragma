// Package lilac implements provider.Provider for Lilac (OpenAI-compatible) via any-llm-go.
package lilac

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
	"github.com/artpar/gogent/internal/provider/anyllm"
	"github.com/artpar/gogent/internal/provider/shared"
	"github.com/mozilla-ai/any-llm-go/config"
	"github.com/mozilla-ai/any-llm-go/providers"
	oai "github.com/mozilla-ai/any-llm-go/providers/openai"
)

const defaultBaseURL = "https://api.getlilac.com/v1"

// Provider wraps any-llm-go's OpenAI provider pointed at Lilac's endpoint.
type Provider struct {
	inner      providers.Provider
	bus        *observe.EventBus
	maxRetries int
}

// Option configures the Provider.
type Option func(*providerConfig)

type providerConfig struct {
	baseURL string
}

// WithBaseURL overrides the API base URL.
func WithBaseURL(url string) Option {
	return func(c *providerConfig) { c.baseURL = url }
}

// New creates a Lilac provider backed by any-llm-go.
func New(apiKey string, bus *observe.EventBus, opts ...Option) (*Provider, error) {
	pc := providerConfig{baseURL: defaultBaseURL}
	for _, opt := range opts {
		opt(&pc)
	}
	cfgOpts := []config.Option{
		config.WithAPIKey(apiKey),
		config.WithBaseURL(pc.baseURL),
	}
	inner, err := oai.New(cfgOpts...)
	if err != nil {
		return nil, fmt.Errorf("lilac: create provider: %w", err)
	}
	return &Provider{inner: inner, bus: bus, maxRetries: 10}, nil
}

func (p *Provider) Name() string { return "lilac" }

func (p *Provider) SupportsFeature(feature provider.Feature) bool {
	switch feature {
	case provider.FeatureToolUse, provider.FeatureStreaming:
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
	return 200_000, false
}

// lilacClassify classifies errors using HTTP status code string matching.
var lilacClassify = shared.ClassifyByStatusCodes([]string{"429", "500", "502", "503", "504"})

func (p *Provider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	observe.TraceCtx(ctx, "lilac", "Provider.Complete", "enter")
	defer observe.TraceCtx(ctx, "lilac", "Provider.Complete", "exit")
	traceID := observe.NewTraceID()
	spanID := observe.NewSpanID()
	p.emitStart(traceID, spanID, params)
	start := time.Now()

	llmParams := anyllm.RequestToParams(params)
	// Lilac (vLLM) rejects stream_options on non-streaming requests.
	llmParams.StreamOptions = nil

	var comp *providers.ChatCompletion
	err := shared.WithRetry(ctx, p.bus, p.maxRetries, traceID, spanID, lilacClassify, func() error {
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
		StopReason:  resp.StopReason,
		Usage:       resp.Usage,
		DurationMs:  time.Since(start).Milliseconds(),
		Model:       resp.Model,
		Content:     shared.MarshalContent(resp.Content),
	})
	return resp, nil
}

func (p *Provider) Stream(ctx context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	observe.TraceCtx(ctx, "lilac", "Provider.Stream", "enter")
	defer observe.TraceCtx(ctx, "lilac", "Provider.Stream", "exit")
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
			if chunk.Usage != nil {
				usage = anyllm.UsageFromAnyLLM(chunk.Usage)
			}
			if chunk.Model != "" {
				respModel = chunk.Model
			}
			for _, choice := range chunk.Choices {
				delta := choice.Delta
				if delta.Content != "" {
					accText.WriteString(delta.Content)
					ch <- provider.StreamChunk{TextDelta: delta.Content}
				}
				if delta.Reasoning != nil && delta.Reasoning.Content != "" {
					ch <- provider.StreamChunk{ThinkingDelta: delta.Reasoning.Content}
				}
				for _, tc := range delta.ToolCalls {
					if tc.ID != "" && !seenToolCalls[tc.ID] {
						seenToolCalls[tc.ID] = true
						toolCallIDs = append(toolCallIDs, tc.ID)
						accToolInputs[tc.ID] = &strings.Builder{}
						accToolNames[tc.ID] = tc.Function.Name
						ch <- provider.StreamChunk{
							ToolCallStart: &model.ToolCallPart{ID: tc.ID, Name: tc.Function.Name},
						}
					}
					if tc.Function.Arguments != "" {
						id := tc.ID
						if id == "" && len(toolCallIDs) > 0 {
							id = toolCallIDs[len(toolCallIDs)-1]
						}
						if b, ok := accToolInputs[id]; ok {
							b.WriteString(tc.Function.Arguments)
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
					var accContent []model.ContentPart
					if accText.Len() > 0 {
						accContent = append(accContent, model.TextPart{Text: accText.String()})
					}
					for _, id := range toolCallIDs {
						tc := model.ToolCallPart{ID: id, Name: accToolNames[id]}
						if b, ok := accToolInputs[id]; ok {
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
		Model:         params.Model,
		MessageCount:  len(params.Messages),
		ToolCount:     len(params.Tools),
		TokenEstimate: shared.EstimateTokens(params),
		Messages:      params.Messages,
		System:        shared.SystemText(params.System),
	})
}
