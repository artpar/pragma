// Package anthropic implements the provider.Provider interface for the
// Anthropic Messages API, translating between internal model types and
// Anthropic wire format.
package anthropic

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
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
func WithMaxRetries(n int) Option { return func(p *Provider) { p.maxRetries = n } }

// WithBaseURL overrides the API base URL (for testing).
func WithBaseURL(url string) Option { return func(p *Provider) { p.baseURL = url } }

// WithIdleTimeout sets the stream idle timeout. Default: 90s.
func WithIdleTimeout(d time.Duration) Option { return func(p *Provider) { p.idleTimeout = d } }

// New creates an Anthropic provider with the given API key and options.
func New(apiKey string, bus *observe.EventBus, opts ...Option) *Provider {
	p := &Provider{
		bus:         bus,
		maxRetries:  10,
		idleTimeout: 90 * time.Second,
	}
	for _, opt := range opts {
		opt(p)
	}

	clientOpts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithMaxRetries(0), // we handle retries ourselves
	}
	if p.baseURL != "" {
		clientOpts = append(clientOpts, option.WithBaseURL(p.baseURL))
	}
	p.client = sdk.NewClient(clientOpts...)
	return p
}

// Name returns "anthropic".
func (p *Provider) Name() string { return "anthropic" }

// SupportsFeature returns true for all features Anthropic supports.
func (p *Provider) SupportsFeature(feature provider.Feature) bool {
	switch feature {
	case provider.FeaturePrefixCaching,
		provider.FeatureThinking,
		provider.FeatureImages,
		provider.FeatureToolUse,
		provider.FeatureStreaming:
		return true
	}
	return false
}

// Pricing returns pricing info for a known model, or zero for unknown models.
func (p *Provider) Pricing(modelID string) model.Pricing {
	if info, ok := LookupModel(modelID); ok {
		return info.Pricing
	}
	return model.Pricing{}
}

// Complete sends a non-streaming request and returns the complete response.
func (p *Provider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	mapper := NewIDMapper()
	prePopulateMapper(params.Messages, mapper)
	wireParams := buildWireParams(params, mapper)
	applyCacheBreakpoints(&wireParams)

	traceID := observe.NewTraceID()
	spanID := observe.NewSpanID()

	p.bus.Emit(observe.APIRequestStarted{
		EventHeader:   observe.NewEventHeader("APIRequestStarted", traceID, spanID, ""),
		Model:         params.Model,
		MessageCount:  len(params.Messages),
		ToolCount:     len(params.Tools),
		TokenEstimate: estimateTokens(params),
	})

	start := time.Now()
	var msg *sdk.Message

	err := p.withRetry(ctx, traceID, spanID, func(attempt int) error {
		var apiErr error
		msg, apiErr = p.client.Messages.New(ctx, wireParams)
		return apiErr
	})
	if err != nil {
		classified := classifyError(err)
		p.bus.Emit(observe.APIRequestFailed{
			EventHeader:  observe.NewEventHeader("APIRequestFailed", traceID, spanID, ""),
			ErrorType:    classified.errorType,
			ErrorMessage: err.Error(),
			Retryable:    classified.retryable,
			Attempt:      p.maxRetries + 1,
		})
		return model.Response{}, classified.wrapped
	}

	resp := responseFromWire(msg, mapper)
	p.bus.Emit(observe.APIRequestCompleted{
		EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
		StopReason:  resp.StopReason,
		Usage:       resp.Usage,
		DurationMs:  time.Since(start).Milliseconds(),
		Model:       resp.Model,
	})
	return resp, nil
}

// Stream starts a streaming request and returns a channel of StreamChunks.
func (p *Provider) Stream(ctx context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	mapper := NewIDMapper()
	prePopulateMapper(params.Messages, mapper)
	wireParams := buildWireParams(params, mapper)
	applyCacheBreakpoints(&wireParams)

	traceID := observe.NewTraceID()
	spanID := observe.NewSpanID()

	p.bus.Emit(observe.APIRequestStarted{
		EventHeader:   observe.NewEventHeader("APIRequestStarted", traceID, spanID, ""),
		Model:         params.Model,
		MessageCount:  len(params.Messages),
		ToolCount:     len(params.Tools),
		TokenEstimate: estimateTokens(params),
	})

	stream := p.client.Messages.NewStreaming(ctx, wireParams)
	ch := p.startStream(ctx, stream, mapper, p.bus, traceID, spanID)
	return ch, nil
}

// withRetry executes fn with exponential backoff retry for retryable errors.
func (p *Provider) withRetry(ctx context.Context, traceID, spanID string, fn func(attempt int) error) error {
	var consecutive529 int

	for attempt := range p.maxRetries + 1 {
		err := fn(attempt)
		if err == nil {
			return nil
		}

		classified := classifyError(err)

		if !classified.retryable || attempt >= p.maxRetries {
			return classified.wrapped
		}

		// 529 consecutive limit: give up after 3
		if classified.errorType == "overloaded" {
			consecutive529++
			if consecutive529 >= 3 {
				return classified.wrapped
			}
		} else {
			consecutive529 = 0
		}

		// Calculate delay
		delay := classified.retryAfter
		if delay == 0 {
			baseDelay := time.Duration(500*math.Pow(2, float64(attempt))) * time.Millisecond
			if baseDelay > 32*time.Second {
				baseDelay = 32 * time.Second
			}
			jitter := time.Duration(rand.Float64()*0.25*float64(baseDelay))
			delay = baseDelay + jitter
		}

		p.bus.Emit(observe.APIRetryScheduled{
			EventHeader: observe.NewEventHeader("APIRetryScheduled", traceID, spanID, ""),
			Attempt:     attempt + 1,
			DelayMs:     delay.Milliseconds(),
			Reason:      classified.errorType,
		})

		p.bus.Emit(observe.APIRequestFailed{
			EventHeader:  observe.NewEventHeader("APIRequestFailed", traceID, spanID, ""),
			ErrorType:    classified.errorType,
			ErrorMessage: err.Error(),
			Retryable:    true,
			Attempt:      attempt + 1,
		})

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("exhausted %d retries", p.maxRetries)
}

// estimateTokens provides a rough token estimate for observability events.
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
			case model.ThinkingPart:
				total += len(p.Text) / 4
			case model.ImagePart:
				total += 1000
			}
		}
	}
	return total
}
