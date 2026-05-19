// Package lilac implements provider.Provider for Lilac (OpenAI-compatible) via any-llm-go.
package lilac

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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: func(c *providerConfig) { c.baseURL = url }")
	return func(c *providerConfig) { c.baseURL = url }
}

// New creates a Lilac provider backed by any-llm-go.
func New(apiKey string, bus *observe.EventBus, opts ...Option) (*Provider, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	pc := providerConfig{baseURL: defaultBaseURL}
	for _, opt := range opts {
		observe.GlobalTrace("range opts")
		opt(&pc)
	}
	cfgOpts := []config.Option{
		config.WithAPIKey(apiKey),
		config.WithBaseURL(pc.baseURL),
		config.WithTimeout(10 * time.Minute),
	}
	inner, err := oai.New(cfgOpts...)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"lilac: create provider: %w\", err)")
		return nil, fmt.Errorf("lilac: create provider: %w", err)
	}
	observe.GlobalTrace("return: &Provider{inner: inner, bus: bus, maxRetries: 10}, nil")
	return &Provider{inner: inner, bus: bus, maxRetries: 10}, nil
}

func (p *Provider) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"lilac\"")
	return "lilac"
}

func (p *Provider) SupportsFeature(feature provider.Feature) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch feature {
	case provider.FeatureToolUse, provider.FeatureStreaming, provider.FeatureImages, provider.FeatureThinking:
		observe.GlobalTrace("case: provider.FeatureToolUse, provider.FeatureStreaming, provider.FeatureImages, provider.FeatureThinking")
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

// ListModels returns sorted IDs of all known Lilac models.
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
	observe.GlobalTrace("return: 200_000, false")
	return 200_000, false
}

// lilacClassify classifies errors using HTTP status code string matching.
var lilacClassify = shared.ClassifyByStatusCodes([]string{"429", "500", "502", "503", "504"})

func (p *Provider) ensureMaxTokens(params *provider.RequestParams) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	info, known := LookupModel(params.Model)
	if params.MaxTokens == 0 && known && info.MaxOutput > 0 {
		observe.GlobalTrace("if: params.MaxTokens == 0 && known && info.MaxOutput > 0")
		params.MaxTokens = info.MaxOutput
	}

	if known && info.MaxContext > 0 && params.MaxTokens > 0 {
		observe.GlobalTrace("if: known && info.MaxContext > 0 && params.MaxTokens > 0")
		promptEst := shared.EstimateTokens(*params)
		headroom := info.MaxContext - promptEst
		if headroom < 1024 {
			observe.GlobalTrace("if: headroom < 1024")
			headroom = 1024
		}
		if params.MaxTokens > headroom {
			observe.GlobalTrace("if: params.MaxTokens > headroom")
			params.MaxTokens = headroom
		}
	}
}

func (p *Provider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	observe.TraceCtx(ctx, "lilac", "Provider.Complete", "enter")
	defer observe.TraceCtx(ctx, "lilac", "Provider.Complete", "exit")
	traceID := observe.NewTraceID()
	spanID := observe.NewSpanID()
	p.ensureMaxTokens(&params)
	p.emitStart(traceID, spanID, params)
	start := time.Now()

	llmParams := anyllm.RequestToParams(params)

	llmParams.StreamOptions = nil

	var comp *providers.ChatCompletion
	err := shared.WithRetry(ctx, p.bus, p.maxRetries, traceID, spanID, lilacClassify, func() error {
		var reqErr error
		comp, reqErr = p.inner.Completion(ctx, llmParams)
		return reqErr
	})
	if err != nil {
		observe.TraceCtx(ctx, "lilac", "Provider.Complete", "if: err != nil")
		observe.TraceCtx(ctx, "lilac", "Provider.Complete", "return: model.Response{}, err")
		return model.Response{}, err
	}

	resp := anyllm.ResponseFromCompletion(comp)
	if resp.Usage.OutputTokens == 0 {
		observe.TraceCtx(ctx, "lilac", "Provider.Complete", "if: resp.Usage.OutputTokens == 0")
		resp.Usage.OutputTokens = shared.EstimateOutputTokens(resp.Content)
	}
	if resp.Usage.InputTokens == 0 {
		observe.TraceCtx(ctx, "lilac", "Provider.Complete", "if: resp.Usage.InputTokens == 0")
		resp.Usage.InputTokens = shared.EstimateTokens(params)
	}
	p.bus.Emit(observe.APIRequestCompleted{
		EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
		StopReason:  resp.StopReason,
		Usage:       resp.Usage,
		DurationMs:  time.Since(start).Milliseconds(),
		Model:       resp.Model,
		Content:     shared.MarshalContent(resp.Content),
	})
	observe.TraceCtx(ctx, "lilac", "Provider.Complete", "return: resp, nil")
	return resp, nil
}

func (p *Provider) Stream(ctx context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	observe.TraceCtx(ctx, "lilac", "Provider.Stream", "enter")
	defer observe.TraceCtx(ctx, "lilac", "Provider.Stream", "exit")
	traceID := observe.NewTraceID()
	spanID := observe.NewSpanID()
	p.ensureMaxTokens(&params)
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
			observe.TraceCtx(ctx, "lilac", "Provider.Stream", "range chunks")
			if chunk.Usage != nil {
				observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: chunk.Usage != nil")
				usage = anyllm.UsageFromAnyLLM(chunk.Usage)
			}
			if chunk.Model != "" {
				observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: chunk.Model != \"\"")
				respModel = chunk.Model
			}
			for _, choice := range chunk.Choices {
				observe.TraceCtx(ctx, "lilac", "Provider.Stream", "range chunk.Choices")
				delta := choice.Delta
				if delta.Content != "" {
					observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: delta.Content != \"\"")
					accText.WriteString(delta.Content)
					ch <- provider.StreamChunk{TextDelta: delta.Content}
				}
				if delta.Reasoning != nil && delta.Reasoning.Content != "" {
					observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: delta.Reasoning != nil && delta.Reasoning.Content != \"\"")
					ch <- provider.StreamChunk{ThinkingDelta: delta.Reasoning.Content}
				}
				for _, tc := range delta.ToolCalls {
					observe.TraceCtx(ctx, "lilac", "Provider.Stream", "range delta.ToolCalls")
					if tc.ID != "" && !seenToolCalls[tc.ID] {
						observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: tc.ID != \"\" && !seenToolCalls[tc.ID]")
						seenToolCalls[tc.ID] = true
						toolCallIDs = append(toolCallIDs, tc.ID)
						accToolInputs[tc.ID] = &strings.Builder{}
						accToolNames[tc.ID] = tc.Function.Name
						ch <- provider.StreamChunk{
							ToolCallStart: &model.ToolCallPart{ID: tc.ID, Name: tc.Function.Name},
						}
					}
					if tc.Function.Arguments != "" {
						observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: tc.Function.Arguments != \"\"")
						id := tc.ID
						if id == "" && len(toolCallIDs) > 0 {
							observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: id == \"\" && len(toolCallIDs) > 0")
							id = toolCallIDs[len(toolCallIDs)-1]
						}
						if id == "" {
							observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: id == \"\"")

							continue
						}
						if b, ok := accToolInputs[id]; ok {
							observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: ok")
							b.WriteString(tc.Function.Arguments)
						}
						ch <- provider.StreamChunk{
							ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: id, JSONDelta: tc.Function.Arguments},
						}
					}
				}
				if choice.FinishReason != "" {
					observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: choice.FinishReason != \"\"")
					stopReason := anyllm.StopReasonFromAnyLLM(choice.FinishReason)
					if stopReason == model.StopEndTurn && len(seenToolCalls) > 0 {
						observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: stopReason == model.StopEndTurn && len(seenToolCalls) > 0")
						stopReason = model.StopToolUse
					}

					if usage.OutputTokens == 0 {
						observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: usage.OutputTokens == 0")
						outputChars := accText.Len()
						for _, b := range accToolInputs {
							observe.TraceCtx(ctx, "lilac", "Provider.Stream", "range accToolInputs")
							outputChars += b.Len()
						}
						if outputChars > 0 {
							observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: outputChars > 0")
							usage.OutputTokens = outputChars / 4
						}
					}
					if usage.InputTokens == 0 {
						observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: usage.InputTokens == 0")
						usage.InputTokens = shared.EstimateTokens(params)
					}
					ch <- provider.StreamChunk{
						Done: &provider.StreamDone{StopReason: stopReason, Usage: usage, Model: respModel},
					}
					var accContent []model.ContentPart
					if accText.Len() > 0 {
						observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: accText.Len() > 0")
						accContent = append(accContent, model.TextPart{Text: accText.String()})
					}
					for _, id := range toolCallIDs {
						observe.TraceCtx(ctx, "lilac", "Provider.Stream", "range toolCallIDs")
						tc := model.ToolCallPart{ID: id, Name: accToolNames[id]}
						if b, ok := accToolInputs[id]; ok {
							observe.TraceCtx(ctx, "lilac", "Provider.Stream", "if: ok")
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
			observe.TraceCtx(ctx, "lilac", "Provider.Stream", "select: err, ok := <-errs")
			if ok && err != nil {
				ch <- provider.StreamChunk{Error: err}
			}
		default:
			observe.TraceCtx(ctx, "lilac", "Provider.Stream", "select: default")
		}
	}()
	observe.TraceCtx(ctx, "lilac", "Provider.Stream", "return: ch, nil")
	return ch, nil
}

func (p *Provider) emitStart(traceID, spanID string, params provider.RequestParams) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	p.bus.Emit(shared.RequestStartedEvent(traceID, spanID, params))
}
