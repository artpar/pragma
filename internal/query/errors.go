package query

import (
	"errors"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/provider"
)

// ErrorKind classifies provider errors for TUI rendering and retry decisions.
type ErrorKind string

const (
	ErrorKindRateLimit       ErrorKind = "rate_limit"
	ErrorKindOverloaded      ErrorKind = "overloaded"
	ErrorKindAuthentication  ErrorKind = "authentication"
	ErrorKindContextOverflow ErrorKind = "context_overflow"
	ErrorKindConnection      ErrorKind = "connection"
	ErrorKindServerError     ErrorKind = "server_error"
	ErrorKindUnknown         ErrorKind = "unknown"
)

// ClassifiedError carries the error plus its classification for retry and TUI rendering.
type ClassifiedError struct {
	Err        error
	Kind       ErrorKind
	Retryable  bool
	Guidance   string        // actionable hint for the user
	RetryAfter time.Duration // from provider Retry-After header if available
}

// ClassifyStreamError inspects a provider error and returns its classification.
// Uses errors.Is against shared provider sentinels first (Anthropic wraps these),
// then falls back to string matching for non-Anthropic providers.
func ClassifyStreamError(err error) ClassifiedError {
	// Sentinel-based classification (works for Anthropic via %w wrapping)
	if errors.Is(err, provider.ErrRateLimit) {
		return ClassifiedError{Err: err, Kind: ErrorKindRateLimit, Retryable: true}
	}
	if errors.Is(err, provider.ErrOverloaded) {
		return ClassifiedError{Err: err, Kind: ErrorKindOverloaded, Retryable: true}
	}
	if errors.Is(err, provider.ErrAuthentication) {
		return ClassifiedError{
			Err:      err,
			Kind:     ErrorKindAuthentication,
			Guidance: "Check your API key or run /doctor",
		}
	}
	if errors.Is(err, provider.ErrContextOverflow) {
		return ClassifiedError{
			Err:      err,
			Kind:     ErrorKindContextOverflow,
			Guidance: "Run /compact to reduce context size",
		}
	}
	if errors.Is(err, provider.ErrServerError) {
		return ClassifiedError{Err: err, Kind: ErrorKindServerError, Retryable: true}
	}
	if errors.Is(err, provider.ErrInvalidRequest) {
		return ClassifiedError{Err: err, Kind: ErrorKindUnknown}
	}

	// String-based fallback for non-Anthropic providers (OpenAI/Groq/Lilac via any-llm-go).
	// Same patterns as shared.ClassifyByStatusCodes.
	msg := strings.ToLower(err.Error())
	return classifyByString(err, msg)
}

// classifyByString inspects the error message for known patterns.
func classifyByString(err error, msg string) ClassifiedError {
	switch {
	case strings.Contains(msg, "429") || strings.Contains(msg, "rate limit"):
		return ClassifiedError{Err: err, Kind: ErrorKindRateLimit, Retryable: true}
	case strings.Contains(msg, "529") || strings.Contains(msg, "overloaded"):
		return ClassifiedError{Err: err, Kind: ErrorKindOverloaded, Retryable: true}
	case strings.Contains(msg, "401") || strings.Contains(msg, "403") || strings.Contains(msg, "authentication") || strings.Contains(msg, "unauthorized"):
		return ClassifiedError{Err: err, Kind: ErrorKindAuthentication, Guidance: "Check your API key or run /doctor"}
	case strings.Contains(msg, "prompt is too long") || strings.Contains(msg, "context length") || strings.Contains(msg, "context window"):
		return ClassifiedError{Err: err, Kind: ErrorKindContextOverflow, Guidance: "Run /compact to reduce context size"}
	case strings.Contains(msg, "connection") || strings.Contains(msg, "dns") || strings.Contains(msg, "eof") || strings.Contains(msg, "econnreset"):
		return ClassifiedError{Err: err, Kind: ErrorKindConnection, Retryable: true}
	case strings.Contains(msg, "500") || strings.Contains(msg, "502") || strings.Contains(msg, "503") || strings.Contains(msg, "server error"):
		return ClassifiedError{Err: err, Kind: ErrorKindServerError, Retryable: true}
	default:
		return ClassifiedError{Err: err, Kind: ErrorKindUnknown}
	}
}

// retryDelay calculates exponential backoff delay for a given attempt.
// Formula matches shared.WithRetry: 500ms * 2^attempt, cap 32s, +0-25% jitter.
func retryDelay(attempt int) time.Duration {
	baseDelay := time.Duration(500*math.Pow(2, float64(attempt))) * time.Millisecond
	if baseDelay > 32*time.Second {
		baseDelay = 32 * time.Second
	}
	jitter := time.Duration(rand.Float64() * 0.25 * float64(baseDelay))
	return baseDelay + jitter
}
