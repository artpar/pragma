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

// New creates a Groq provider with the given API key and options.
func New(apiKey string, bus *observe.EventBus, opts ...Option) *Provider {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	p := &Provider{
		apiKey:      apiKey,
		baseURL:     defaultBaseURL,
		client:      &http.Client{Timeout: 5 * time.Minute},
		bus:         bus,
		maxRetries:  10,
		idleTimeout: 90 * time.Second,
	}
	for _, opt := range opts {
		observe.GlobalTrace("range opts")
		opt(p)
	}
	observe.GlobalTrace("return: p")
	return p
}

// Name returns "groq".
func (p *Provider) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"groq\"")
	return "groq"
}

// SupportsFeature returns true for features Groq supports.
func (p *Provider) SupportsFeature(feature provider.Feature) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch feature {
	case provider.FeatureToolUse,
		provider.FeatureStreaming,
		provider.FeatureImages,
		provider.FeatureThinking:
		observe.GlobalTrace("case: provider.FeatureToolUse, provider.FeatureStreaming, provider.FeatureImages, p...")
		return true
	case provider.FeaturePrefixCaching:
		observe.GlobalTrace("case: provider.FeaturePrefixCaching")
		return false
	}
	observe.GlobalTrace("return: false")
	return false
}

// Pricing returns pricing info for a known model.
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

// ContextWindow returns the context window size for a known model.
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

// Complete sends a non-streaming request and returns the complete response.
func (p *Provider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	observe.TraceCtx(ctx, "groq", "Provider.Complete", "enter")
	defer observe.TraceCtx(ctx, "groq", "Provider.Complete", "exit")
	mapper := NewIDMapper()
	prePopulateMapper(params.Messages, mapper)
	wireReq := buildWireRequest(params, mapper, false, p.bus)

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
		defer func() { _ = resp.Body.Close() }()

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
		observe.TraceCtx(ctx, "groq", "Provider.Complete", "if: err != nil")
		observe.TraceCtx(ctx, "groq", "Provider.Complete", "return: model.Response{}, err")

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
	observe.TraceCtx(ctx, "groq", "Provider.Complete", "return: resp, nil")
	return resp, nil
}

// Stream starts a streaming request and returns a channel of StreamChunks.
func (p *Provider) Stream(ctx context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	observe.TraceCtx(ctx, "groq", "Provider.Stream", "enter")
	defer observe.TraceCtx(ctx, "groq", "Provider.Stream", "exit")
	mapper := NewIDMapper()
	prePopulateMapper(params.Messages, mapper)
	wireReq := buildWireRequest(params, mapper, true, p.bus)

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
		observe.TraceCtx(ctx, "groq", "Provider.Stream", "if: err != nil")
		observe.TraceCtx(ctx, "groq", "Provider.Stream", "return: nil, err")

		return nil, err
	}

	ch := p.startStream(ctx, httpResp, mapper, p.bus, traceID, spanID)
	observe.TraceCtx(ctx, "groq", "Provider.Stream", "return: ch, nil")
	return ch, nil
}

// withRetry executes fn with exponential backoff retry for retryable errors.
// Gives up after 3 consecutive 529 (overloaded) responses to avoid hammering
// a service that is under pressure.
func (p *Provider) withRetry(ctx context.Context, traceID, spanID string, fn func(attempt int) error) error {
	observe.TraceCtx(ctx, "groq", "Provider.withRetry", "enter")
	defer observe.TraceCtx(ctx, "groq", "Provider.withRetry", "exit")
	var consecutiveOverloaded int
	for attempt := range p.maxRetries + 1 {
		observe.TraceCtx(ctx, "groq", "Provider.withRetry", "range p.maxRetries + 1")
		err := fn(attempt)
		if err == nil {
			observe.TraceCtx(ctx, "groq", "Provider.withRetry", "if: err == nil")
			observe.TraceCtx(ctx, "groq", "Provider.withRetry", "return: nil")
			return nil
		}

		classified := classifyError(err)

		if classified.errorType == "overloaded" {
			observe.TraceCtx(ctx, "groq", "Provider.withRetry", "if: classified.errorType == \"overloaded\"")
			consecutiveOverloaded++
			if consecutiveOverloaded >= 3 {
				observe.TraceCtx(ctx, "groq", "Provider.withRetry", "if: consecutiveOverloaded >= 3")
				classified.retryable = false
			}
		} else {
			observe.TraceCtx(ctx, "groq", "Provider.withRetry", "else: classified.errorType == \"overloaded\"")
			consecutiveOverloaded = 0
		}

		if !classified.retryable || attempt >= p.maxRetries {
			observe.TraceCtx(ctx, "groq", "Provider.withRetry", "if: !classified.retryable || attempt >= p.maxRetries")

			p.bus.Emit(observe.APIRequestFailed{
				EventHeader:  observe.NewEventHeader("APIRequestFailed", traceID, spanID, ""),
				ErrorType:    classified.errorType,
				ErrorMessage: classified.wrapped.Error(),
				Retryable:    false,
				Attempt:      attempt + 1,
			})
			observe.TraceCtx(ctx, "groq", "Provider.withRetry", "return: classified.wrapped")
			return classified.wrapped
		}

		delay := classified.retryAfter
		if delay == 0 {
			observe.TraceCtx(ctx, "groq", "Provider.withRetry", "if: delay == 0")
			baseDelay := time.Duration(500*math.Pow(2, float64(attempt))) * time.Millisecond
			if baseDelay > 32*time.Second {
				observe.TraceCtx(ctx, "groq", "Provider.withRetry", "if: baseDelay > 32*time.Second")
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
			observe.TraceCtx(ctx, "groq", "Provider.withRetry", "select: <-time.After(delay)")
		case <-ctx.Done():
			observe.TraceCtx(ctx, "groq", "Provider.withRetry", "select: <-ctx.Done()")
			return ctx.Err()
		}
	}
	observe.TraceCtx(ctx, "groq", "Provider.withRetry", "return: fmt.Errorf(\"exhausted %d retries\", p.maxRetries)")
	return fmt.Errorf("exhausted %d retries", p.maxRetries)
}

// estimateTokens provides a rough token estimate for observability events.
func estimateTokens(params provider.RequestParams) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	total := 0
	for _, block := range params.System.Blocks {
		observe.GlobalTrace("range params.System.Blocks")
		total += len(block.Text) / 4
	}
	for _, m := range params.Messages {
		observe.GlobalTrace("range params.Messages")
		for _, part := range m.Content {
			observe.GlobalTrace("range m.Content")
			switch p := part.(type) {
			case model.TextPart:
				observe.GlobalTrace("typecase: model.TextPart")
				total += len(p.Text) / 4
			case model.ToolCallPart:
				observe.GlobalTrace("typecase: model.ToolCallPart")
				total += len(p.Input) / 4
			case model.ToolResultPart:
				observe.GlobalTrace("typecase: model.ToolResultPart")
				total += len(p.Content) / 4
			case model.ThinkingPart:
				observe.GlobalTrace("typecase: model.ThinkingPart")
				total += len(p.Text) / 4
			case model.ImagePart:
				observe.GlobalTrace("typecase: model.ImagePart")
				total += 1000
			case model.DocumentPart:
				observe.GlobalTrace("typecase: model.DocumentPart")
				total += len(p.Data) / 4
			}
		}
	}
	observe.GlobalTrace("return: total")
	return total
}
