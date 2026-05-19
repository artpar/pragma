// Package anthropic implements the provider.Provider interface for the
// Anthropic Messages API, translating between internal model types and
// Anthropic wire format.
package anthropic

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/provider/shared"
)

// Provider implements provider.Provider for the Anthropic Messages API.
type Provider struct {
	client      sdk.Client
	bus         *observe.EventBus
	maxRetries  int
	idleTimeout time.Duration
	baseURL     string // stored for testing visibility
}

// Option configures the Provider.
type Option func(*Provider)

// WithMaxRetries sets the maximum number of retry attempts. Default: 10.
func WithMaxRetries(n int) Option {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: func(p *Provider) { p.maxRetries = n }")
	return func(p *Provider) { p.maxRetries = n }
}

// WithBaseURL overrides the API base URL (for testing).
func WithBaseURL(url string) Option {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: func(p *Provider) { p.baseURL = url }")
	return func(p *Provider) { p.baseURL = url }
}

// WithIdleTimeout sets the stream idle timeout. Default: 90s.
func WithIdleTimeout(d time.Duration) Option {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: func(p *Provider) { p.idleTimeout = d }")
	return func(p *Provider) { p.idleTimeout = d }
}

// New creates an Anthropic provider with the given API key and options.
func New(apiKey string, bus *observe.EventBus, opts ...Option) *Provider {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	p := &Provider{
		bus:         bus,
		maxRetries:  10,
		idleTimeout: 90 * time.Second,
	}
	for _, opt := range opts {
		observe.GlobalTrace("range opts")
		opt(p)
	}

	clientOpts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithMaxRetries(0),
	}
	if p.baseURL != "" {
		observe.GlobalTrace("if: p.baseURL != \"\"")
		clientOpts = append(clientOpts, option.WithBaseURL(p.baseURL))
	}
	p.client = sdk.NewClient(clientOpts...)
	observe.GlobalTrace("return: p")
	return p
}

// Name returns "anthropic".
func (p *Provider) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"anthropic\"")
	return "anthropic"
}

// SupportsFeature returns true for all features Anthropic supports.
func (p *Provider) SupportsFeature(feature provider.Feature) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch feature {
	case provider.FeaturePrefixCaching,
		provider.FeatureThinking,
		provider.FeatureImages,
		provider.FeatureToolUse,
		provider.FeatureStreaming:
		observe.GlobalTrace("case: provider.FeaturePrefixCaching, provider.FeatureThinking, provider.FeatureImag...")
		return true
	}
	observe.GlobalTrace("return: false")
	return false
}

// Pricing returns pricing info for a known model.
// Returns false if the model is not recognized.
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

// ListModels returns sorted IDs of all known Anthropic models.
func (p *Provider) ListModels() []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: ListModels()")
	return ListModels()
}

// ContextWindow returns the context window size in tokens for the given model.
// Parses [Xm] suffix for extended context variants (GitHub issue #41984, #39467).
// E.g., "claude-sonnet-4-20250514[1m]" returns 1,000,000.
func (p *Provider) ContextWindow(modelID string) (int, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	if idx := strings.Index(modelID, "["); idx != -1 {
		observe.GlobalTrace("if: idx != -1")
		suffix := modelID[idx:]
		if strings.HasSuffix(suffix, "m]") {
			observe.GlobalTrace("if: strings.HasSuffix(suffix, \"m]\")")
			multiplierStr := suffix[1 : len(suffix)-2]
			if n, err := strconv.Atoi(multiplierStr); err == nil {
				observe.GlobalTrace("if: err == nil")
				observe.GlobalTrace("return: n * 1_000_000, true")
				return n * 1_000_000, true
			}
		}
		modelID = modelID[:idx]
	}

	if info, ok := LookupModel(modelID); ok {
		observe.GlobalTrace("if: ok")
		cw := info.ContextWindow
		if cw == 0 {
			observe.GlobalTrace("if: cw == 0")
			cw = 200_000
		}
		observe.GlobalTrace("return: cw, true")
		return cw, true
	}
	observe.GlobalTrace("return: 200_000, false")
	return 200_000, false
}

// Complete sends a non-streaming request and returns the complete response.
func (p *Provider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	observe.TraceCtx(ctx, "anthropic", "Provider.Complete", "enter")
	defer observe.TraceCtx(ctx, "anthropic", "Provider.Complete", "exit")
	mapper := NewIDMapper()
	prePopulateMapper(params.Messages, mapper)
	wireParams, err := buildWireParams(params, mapper)
	if err != nil {
		observe.TraceCtx(ctx, "anthropic", "Provider.Complete", "if: err != nil")
		observe.TraceCtx(ctx, "anthropic", "Provider.Complete", "return: model.Response{}, fmt.Errorf(\"building wire params: %w\", err)")
		return model.Response{}, fmt.Errorf("building wire params: %w", err)
	}
	applyCacheBreakpoints(&wireParams)

	traceID := observe.NewTraceID()
	spanID := observe.NewSpanID()

	p.bus.Emit(shared.RequestStartedEvent(traceID, spanID, params))

	start := time.Now()
	var msg *sdk.Message

	err = shared.WithRetry(ctx, p.bus, p.maxRetries, traceID, spanID, anthropicClassify, func() error {
		var apiErr error
		msg, apiErr = p.client.Messages.New(ctx, wireParams)
		return apiErr
	})
	if err != nil {
		observe.TraceCtx(ctx, "anthropic", "Provider.Complete", "if: err != nil")
		observe.TraceCtx(ctx, "anthropic", "Provider.Complete", "return: model.Response{}, err")
		return model.Response{}, err
	}

	resp := responseFromWire(msg, mapper, p.bus)
	p.bus.Emit(observe.APIRequestCompleted{
		EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
		StopReason:  resp.StopReason,
		Usage:       resp.Usage,
		DurationMs:  time.Since(start).Milliseconds(),
		Model:       resp.Model,
		Content:     shared.MarshalContent(resp.Content),
	})
	observe.TraceCtx(ctx, "anthropic", "Provider.Complete", "return: resp, nil")
	return resp, nil
}

// Stream starts a streaming request and returns a channel of StreamChunks.
func (p *Provider) Stream(ctx context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	observe.TraceCtx(ctx, "anthropic", "Provider.Stream", "enter")
	defer observe.TraceCtx(ctx, "anthropic", "Provider.Stream", "exit")
	mapper := NewIDMapper()
	prePopulateMapper(params.Messages, mapper)
	wireParams, err := buildWireParams(params, mapper)
	if err != nil {
		observe.TraceCtx(ctx, "anthropic", "Provider.Stream", "if: err != nil")
		observe.TraceCtx(ctx, "anthropic", "Provider.Stream", "return: nil, fmt.Errorf(\"building wire params: %w\", err)")
		return nil, fmt.Errorf("building wire params: %w", err)
	}
	applyCacheBreakpoints(&wireParams)

	traceID := observe.NewTraceID()
	spanID := observe.NewSpanID()

	p.bus.Emit(shared.RequestStartedEvent(traceID, spanID, params))

	stream := p.client.Messages.NewStreaming(ctx, wireParams)
	ch := p.startStream(ctx, stream, mapper, p.bus, traceID, spanID)
	observe.TraceCtx(ctx, "anthropic", "Provider.Stream", "return: ch, nil")
	return ch, nil
}

// anthropicClassify wraps classifyError to match the shared.ClassifyFn signature.
func anthropicClassify(err error) shared.ErrorClassification {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c := classifyError(err)
	observe.GlobalTrace("return: shared.ErrorClassification{\n\tWrapped:\tc.wrapped,\n\tRetryable:\tc.retryable,\n\tEr...")
	return shared.ErrorClassification{
		Wrapped:    c.wrapped,
		Retryable:  c.retryable,
		ErrorType:  c.errorType,
		RetryAfter: c.retryAfter,
	}
}
