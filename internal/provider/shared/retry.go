package shared

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/observe"
)

// ErrorClassification holds the result of classifying an API error.
type ErrorClassification struct {
	Wrapped    error
	Retryable  bool
	ErrorType  string
	RetryAfter time.Duration
}

// ClassifyFn inspects an error and returns its classification.
// Provider-specific: Anthropic uses SDK error inspection, others use status code string matching.
type ClassifyFn func(error) ErrorClassification

// WithRetry executes fn with exponential backoff retry for retryable errors.
// Uses the full retry algorithm: exponential backoff (500ms base, 32s cap, ±25% jitter),
// consecutive overloaded error tracking (fails after 3), retry-after hint support,
// and context cancellation.
func WithRetry(ctx context.Context, bus *observe.EventBus, maxRetries int, traceID, spanID string, classify ClassifyFn, fn func() error) error {
	observe.TraceCtx(ctx, "shared", "WithRetry", "enter")
	defer observe.TraceCtx(ctx, "shared", "WithRetry", "exit")
	var consecutiveOverloaded int

	for attempt := range maxRetries + 1 {
		observe.TraceCtx(ctx, "shared", "WithRetry", "range maxRetries + 1")
		err := fn()
		if err == nil {
			observe.TraceCtx(ctx, "shared", "WithRetry", "if: err == nil")
			observe.TraceCtx(ctx, "shared", "WithRetry", "return: nil")
			return nil
		}

		classified := classify(err)

		if !classified.Retryable || attempt >= maxRetries {
			observe.TraceCtx(ctx, "shared", "WithRetry", "if: !classified.Retryable || attempt >= maxRetries")
			bus.Emit(observe.APIRequestFailed{
				EventHeader:  observe.NewEventHeader("APIRequestFailed", traceID, spanID, ""),
				ErrorType:    classified.ErrorType,
				ErrorMessage: err.Error(),
				Retryable:    false,
				Attempt:      attempt + 1,
			})
			observe.TraceCtx(ctx, "shared", "WithRetry", "return: classified.Wrapped")
			return classified.Wrapped
		}

		if classified.ErrorType == "overloaded" {
			observe.TraceCtx(ctx, "shared", "WithRetry", "if: classified.ErrorType == \"overloaded\"")
			consecutiveOverloaded++
			if consecutiveOverloaded >= 3 {
				observe.TraceCtx(ctx, "shared", "WithRetry", "if: consecutiveOverloaded >= 3")
				bus.Emit(observe.APIRequestFailed{
					EventHeader:  observe.NewEventHeader("APIRequestFailed", traceID, spanID, ""),
					ErrorType:    classified.ErrorType,
					ErrorMessage: err.Error(),
					Retryable:    false,
					Attempt:      attempt + 1,
				})
				observe.TraceCtx(ctx, "shared", "WithRetry", "return: classified.Wrapped")
				return classified.Wrapped
			}
		} else {
			observe.TraceCtx(ctx, "shared", "WithRetry", "else: classified.ErrorType == \"overloaded\"")
			consecutiveOverloaded = 0
		}

		delay := classified.RetryAfter
		if delay == 0 {
			observe.TraceCtx(ctx, "shared", "WithRetry", "if: delay == 0")
			baseDelay := time.Duration(500*math.Pow(2, float64(attempt))) * time.Millisecond
			if baseDelay > 32*time.Second {
				observe.TraceCtx(ctx, "shared", "WithRetry", "if: baseDelay > 32*time.Second")
				baseDelay = 32 * time.Second
			}
			delay = baseDelay + time.Duration(rand.Float64()*0.25*float64(baseDelay))
		}

		bus.Emit(observe.APIRetryScheduled{
			EventHeader: observe.NewEventHeader("APIRetryScheduled", traceID, spanID, ""),
			Attempt:     attempt + 1,
			DelayMs:     delay.Milliseconds(),
			Reason:      classified.ErrorType,
		})

		bus.Emit(observe.APIRequestFailed{
			EventHeader:  observe.NewEventHeader("APIRequestFailed", traceID, spanID, ""),
			ErrorType:    classified.ErrorType,
			ErrorMessage: err.Error(),
			Retryable:    true,
			Attempt:      attempt + 1,
		})

		select {
		case <-time.After(delay):
			observe.TraceCtx(ctx, "shared", "WithRetry", "select: <-time.After(delay)")
		case <-ctx.Done():
			observe.TraceCtx(ctx, "shared", "WithRetry", "select: <-ctx.Done()")
			return ctx.Err()
		}
	}
	observe.TraceCtx(ctx, "shared", "WithRetry", "return: fmt.Errorf(\"exhausted %d retries\", maxRetries)")
	return fmt.Errorf("exhausted %d retries", maxRetries)
}

// ClassifyByStatusCodes returns a ClassifyFn that checks the error message
// for HTTP status code strings. Use for providers without structured error types.
func ClassifyByStatusCodes(codes []string) ClassifyFn {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: func(err error) ErrorClassification {\n\tmsg := err.Error()\n\tfor _, s := range ...")
	return func(err error) ErrorClassification {
		msg := err.Error()
		for _, s := range codes {
			if strings.Contains(msg, s) {
				errorType := "rate_limit"
				if s == "529" {
					errorType = "overloaded"
				} else if s != "429" {
					errorType = "server_error"
				}
				return ErrorClassification{
					Wrapped:   err,
					Retryable: true,
					ErrorType: errorType,
				}
			}
		}
		return ErrorClassification{
			Wrapped:   err,
			Retryable: false,
			ErrorType: "request_failed",
		}
	}
}
