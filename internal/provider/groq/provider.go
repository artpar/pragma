package groq

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
)

const defaultBaseURL = "https://api.groq.com/openai/v1"

// Provider implements provider.Provider for the Groq API.
type Provider struct {
	apiKey      string
	baseURL     string
	client      *http.Client
	bus         *observe.EventBus
	maxRetries  int
	idleTimeout time.Duration
}

// Option configures the Provider.
type Option func(*Provider)

// WithMaxRetries sets the maximum number of retry attempts. Default: 10.
func WithMaxRetries(n int) Option { return func(p *Provider) { p.maxRetries = n } }

// WithBaseURL overrides the API base URL (for testing).
func WithBaseURL(url string) Option { return func(p *Provider) { p.baseURL = url } }

// WithIdleTimeout sets the stream idle timeout. Default: 90s.
func WithIdleTimeout(d time.Duration) Option { return func(p *Provider) { p.idleTimeout = d } }

// New creates a Groq provider with the given API key and options.
func New(apiKey string, bus *observe.EventBus, opts ...Option) *Provider {
	p := &Provider{
		apiKey:      apiKey,
		baseURL:     defaultBaseURL,
		client:      &http.Client{Timeout: 5 * time.Minute},
		bus:         bus,
		maxRetries:  10,
		idleTimeout: 90 * time.Second,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name returns "groq".
func (p *Provider) Name() string { return "groq" }

// SupportsFeature returns true for features Groq supports.
func (p *Provider) SupportsFeature(feature provider.Feature) bool {
	switch feature {
	case provider.FeatureToolUse,
		provider.FeatureStreaming,
		provider.FeatureImages,
		provider.FeatureThinking:
		return true
	case provider.FeaturePrefixCaching:
		return false // automatic, no explicit control
	}
	return false
}

// Pricing returns pricing info for a known model.
func (p *Provider) Pricing(modelID string) (model.Pricing, bool) {
	if info, ok := LookupModel(modelID); ok {
		return info.Pricing, true
	}
	return model.Pricing{}, false
}

// Complete sends a non-streaming request and returns the complete response.
func (p *Provider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	mapper := NewIDMapper()
	prePopulateMapper(params.Messages, mapper)
	wireReq := buildWireRequest(params, mapper, false)

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
	var wireResp wireResponse

	err := p.withRetry(ctx, traceID, spanID, func(_ int) error {
		body, marshalErr := json.Marshal(wireReq)
		if marshalErr != nil {
			return fmt.Errorf("marshal request: %w", marshalErr)
		}
		req, reqErr := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
		if reqErr != nil {
			return fmt.Errorf("create request: %w", reqErr)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+p.apiKey)

		resp, doErr := p.client.Do(req)
		if doErr != nil {
			return doErr
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return &httpError{
				statusCode: resp.StatusCode,
				body:       respBody,
				resp:       resp,
				message:    fmt.Sprintf("groq: HTTP %d: %s", resp.StatusCode, extractErrorMessage(respBody)),
			}
		}

		return json.NewDecoder(resp.Body).Decode(&wireResp)
	})

	if err != nil {
		// withRetry already emitted APIRequestFailed with correct error type
		return model.Response{}, err
	}

	resp := responseFromWire(&wireResp, mapper, p.bus)
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
	wireReq := buildWireRequest(params, mapper, true)

	traceID := observe.NewTraceID()
	spanID := observe.NewSpanID()

	p.bus.Emit(observe.APIRequestStarted{
		EventHeader:   observe.NewEventHeader("APIRequestStarted", traceID, spanID, ""),
		Model:         params.Model,
		MessageCount:  len(params.Messages),
		ToolCount:     len(params.Tools),
		TokenEstimate: estimateTokens(params),
	})

	var httpResp *http.Response
	err := p.withRetry(ctx, traceID, spanID, func(_ int) error {
		body, marshalErr := json.Marshal(wireReq)
		if marshalErr != nil {
			return fmt.Errorf("marshal request: %w", marshalErr)
		}
		req, reqErr := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
		if reqErr != nil {
			return fmt.Errorf("create request: %w", reqErr)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+p.apiKey)

		resp, doErr := p.client.Do(req)
		if doErr != nil {
			return doErr
		}

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return &httpError{
				statusCode: resp.StatusCode,
				body:       respBody,
				resp:       resp,
				message:    fmt.Sprintf("groq: HTTP %d: %s", resp.StatusCode, extractErrorMessage(respBody)),
			}
		}

		httpResp = resp
		return nil
	})

	if err != nil {
		// withRetry already emitted APIRequestFailed with correct error type
		return nil, err
	}

	ch := p.startStream(ctx, httpResp, mapper, p.bus, traceID, spanID)
	return ch, nil
}

// withRetry executes fn with exponential backoff retry for retryable errors.
func (p *Provider) withRetry(ctx context.Context, traceID, spanID string, fn func(attempt int) error) error {
	for attempt := range p.maxRetries + 1 {
		err := fn(attempt)
		if err == nil {
			return nil
		}

		classified := classifyError(err)

		if !classified.retryable || attempt >= p.maxRetries {
			// Emit final failure event with correct error type (before wrapping loses it)
			p.bus.Emit(observe.APIRequestFailed{
				EventHeader:  observe.NewEventHeader("APIRequestFailed", traceID, spanID, ""),
				ErrorType:    classified.errorType,
				ErrorMessage: classified.wrapped.Error(),
				Retryable:    false,
				Attempt:      attempt + 1,
			})
			return classified.wrapped
		}

		delay := classified.retryAfter
		if delay == 0 {
			baseDelay := time.Duration(500*math.Pow(2, float64(attempt))) * time.Millisecond
			if baseDelay > 32*time.Second {
				baseDelay = 32 * time.Second
			}
			jitter := time.Duration(rand.Float64() * 0.25 * float64(baseDelay))
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
			case model.DocumentPart:
				total += len(p.Data) / 4
			}
		}
	}
	return total
}
