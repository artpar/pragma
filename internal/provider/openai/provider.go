// Package openai implements provider.Provider for OpenAI via any-llm-go.
package openai

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
	"github.com/artpar/gogent/internal/provider/anyllm"
	"github.com/mozilla-ai/any-llm-go/config"
	"github.com/mozilla-ai/any-llm-go/providers"
	oai "github.com/mozilla-ai/any-llm-go/providers/openai"
)

// Provider wraps any-llm-go's OpenAI provider for gogent.
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

// New creates an OpenAI provider backed by any-llm-go.
func New(apiKey string, bus *observe.EventBus, opts ...Option) (*Provider, error) {
	var pc providerConfig
	for _, opt := range opts {
		opt(&pc)
	}
	cfgOpts := []config.Option{config.WithAPIKey(apiKey)}
	if pc.baseURL != "" {
		cfgOpts = append(cfgOpts, config.WithBaseURL(pc.baseURL))
	}
	inner, err := oai.New(cfgOpts...)
	if err != nil {
		return nil, fmt.Errorf("openai: create provider: %w", err)
	}
	return &Provider{inner: inner, bus: bus, maxRetries: 10}, nil
}

func (p *Provider) Name() string { return "openai" }

func (p *Provider) SupportsFeature(feature provider.Feature) bool {
	switch feature {
	case provider.FeatureToolUse, provider.FeatureStreaming, provider.FeatureImages:
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
	return 128_000, false
}

func (p *Provider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	traceID := observe.NewTraceID()
	spanID := observe.NewSpanID()
	p.emitStart(traceID, spanID, params)
	start := time.Now()

	llmParams := anyllm.RequestToParams(params)

	var comp *providers.ChatCompletion
	err := p.withRetry(ctx, traceID, spanID, func() error {
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
		var toolCallIDs []string // ordered list of tool call IDs
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
		Model:         params.Model,
		MessageCount:  len(params.Messages),
		ToolCount:     len(params.Tools),
		TokenEstimate: estimateTokens(params),
	})
}

func estimateTokens(params provider.RequestParams) int {
	total := 0
	for _, block := range params.System.Blocks {
		total += len(block.Text) / 4
	}
	for _, m := range params.Messages {
		for _, part := range m.Content {
			switch p := part.(type) {
			case model.TextPart:
				total += len(p.Text) / 4
			case model.ToolCallPart:
				total += len(p.Input) / 4
			case model.ToolResultPart:
				total += len(p.Content) / 4
			}
		}
	}
	return total
}

func (p *Provider) withRetry(ctx context.Context, traceID, spanID string, fn func() error) error {
	for attempt := range p.maxRetries + 1 {
		err := fn()
		if err == nil {
			return nil
		}
		if attempt >= p.maxRetries || !isRetryable(err) {
			p.bus.Emit(observe.APIRequestFailed{
				EventHeader: observe.NewEventHeader("APIRequestFailed", traceID, spanID, ""),
				ErrorType: "request_failed", ErrorMessage: err.Error(),
				Retryable: false, Attempt: attempt + 1,
			})
			return err
		}
		baseDelay := time.Duration(500*math.Pow(2, float64(attempt))) * time.Millisecond
		if baseDelay > 32*time.Second {
			baseDelay = 32 * time.Second
		}
		delay := baseDelay + time.Duration(rand.Float64()*0.25*float64(baseDelay))
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("exhausted %d retries", p.maxRetries)
}

func isRetryable(err error) bool {
	msg := err.Error()
	for _, s := range []string{"429", "500", "502", "503", "504"} {
		if len(msg) >= len(s) {
			for i := 0; i <= len(msg)-len(s); i++ {
				if msg[i:i+len(s)] == s {
					return true
				}
			}
		}
	}
	return false
}
