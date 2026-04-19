package query

import (
	"errors"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/observe"
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
	ErrorKindInvalidRequest  ErrorKind = "invalid_request"
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	if errors.Is(err, provider.ErrRateLimit) {
		observe.GlobalTrace("if: errors.Is(err, provider.ErrRateLimit)")
		observe.GlobalTrace("return: ClassifiedError{Err: err, Kind: ErrorKindRateLimit, Retryable: true, Guidance: ...}")
		return ClassifiedError{Err: err, Kind: ErrorKindRateLimit, Retryable: true, Guidance: "Rate limited — retrying automatically. If persistent, check your API plan tier"}
	}
	if errors.Is(err, provider.ErrOverloaded) {
		observe.GlobalTrace("if: errors.Is(err, provider.ErrOverloaded)")
		observe.GlobalTrace("return: ClassifiedError{Err: err, Kind: ErrorKindOverloaded, Retryable: true, Guidance: ...}")
		return ClassifiedError{Err: err, Kind: ErrorKindOverloaded, Retryable: true, Guidance: "Provider overloaded — retrying. Try again later if persistent"}
	}
	if errors.Is(err, provider.ErrAuthentication) {
		observe.GlobalTrace("if: errors.Is(err, provider.ErrAuthentication)")
		observe.GlobalTrace("return: ClassifiedError{\n\tErr:\t\terr,\n\tKind:\t\tErrorKindAuthentication,\n\tGuidance:\t\"Che...")
		return ClassifiedError{
			Err:      err,
			Kind:     ErrorKindAuthentication,
			Guidance: "Check your API key or run /doctor",
		}
	}
	if errors.Is(err, provider.ErrContextOverflow) {
		observe.GlobalTrace("if: errors.Is(err, provider.ErrContextOverflow)")
		observe.GlobalTrace("return: ClassifiedError{\n\tErr:\t\terr,\n\tKind:\t\tErrorKindContextOverflow,\n\tGuidance:\t\"Ru...")
		return ClassifiedError{
			Err:      err,
			Kind:     ErrorKindContextOverflow,
			Guidance: "Run /compact to reduce context size",
		}
	}
	if errors.Is(err, provider.ErrServerError) {
		observe.GlobalTrace("if: errors.Is(err, provider.ErrServerError)")
		observe.GlobalTrace("return: ClassifiedError{Err: err, Kind: ErrorKindServerError, Retryable: true, Guidance: ...}")
		return ClassifiedError{Err: err, Kind: ErrorKindServerError, Retryable: true, Guidance: "Provider server error — retrying. Check provider status page if persistent"}
	}
	if errors.Is(err, provider.ErrInvalidRequest) {
		observe.GlobalTrace("if: errors.Is(err, provider.ErrInvalidRequest)")
		observe.GlobalTrace("return: ClassifiedError{Err: err, Kind: ErrorKindInvalidRequest, Guidance: ...}")
		return ClassifiedError{Err: err, Kind: ErrorKindInvalidRequest, Guidance: "Check model name with /model. Run /doctor to verify provider configuration"}
	}

	msg := strings.ToLower(err.Error())
	observe.GlobalTrace("return: classifyByString(err, msg)")
	return classifyByString(err, msg)
}

// classifyByString inspects the error message for known patterns.
func classifyByString(err error, msg string) ClassifiedError {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch {
	case strings.Contains(msg, "429") || strings.Contains(msg, "rate limit"):
		observe.GlobalTrace("case: strings.Contains(msg, \"429\") || strings.Contains(msg, \"rate limit\")")
		return ClassifiedError{Err: err, Kind: ErrorKindRateLimit, Retryable: true, Guidance: "Rate limited — retrying automatically. If persistent, check your API plan tier"}
	case strings.Contains(msg, "529") || strings.Contains(msg, "overloaded"):
		observe.GlobalTrace("case: strings.Contains(msg, \"529\") || strings.Contains(msg, \"overloaded\")")
		return ClassifiedError{Err: err, Kind: ErrorKindOverloaded, Retryable: true, Guidance: "Provider overloaded — retrying. Try again later if persistent"}
	case strings.Contains(msg, "401") || strings.Contains(msg, "403") || strings.Contains(msg, "authentication") || strings.Contains(msg, "unauthorized"):
		observe.GlobalTrace("case: strings.Contains(msg, \"401\") || strings.Contains(msg, \"403\") || strings.Conta...")
		return ClassifiedError{Err: err, Kind: ErrorKindAuthentication, Guidance: "Check your API key or run /doctor"}
	case strings.Contains(msg, "prompt is too long") || strings.Contains(msg, "context length") || strings.Contains(msg, "context window"):
		observe.GlobalTrace("case: strings.Contains(msg, \"prompt is too long\") || strings.Contains(msg, \"context...")
		return ClassifiedError{Err: err, Kind: ErrorKindContextOverflow, Guidance: "Run /compact to reduce context size"}
	case strings.Contains(msg, "connection") || strings.Contains(msg, "dns") || strings.Contains(msg, "eof") || strings.Contains(msg, "econnreset"):
		observe.GlobalTrace("case: strings.Contains(msg, \"connection\") || strings.Contains(msg, \"dns\") || string...")
		return ClassifiedError{Err: err, Kind: ErrorKindConnection, Retryable: true, Guidance: "Check internet connection and proxy settings"}
	case strings.Contains(msg, "500") || strings.Contains(msg, "502") || strings.Contains(msg, "503") || strings.Contains(msg, "server error"):
		observe.GlobalTrace("case: strings.Contains(msg, \"500\") || strings.Contains(msg, \"502\") || strings.Conta...")
		return ClassifiedError{Err: err, Kind: ErrorKindServerError, Retryable: true, Guidance: "Provider server error — retrying. Check provider status page if persistent"}
	case strings.Contains(msg, "not_found") || strings.Contains(msg, "404") || strings.Contains(msg, "invalid model") || strings.Contains(msg, "model not found"):
		observe.GlobalTrace("case: strings.Contains(msg, \"not_found\") || strings.Contains(msg, \"404\") || ...")
		return ClassifiedError{Err: err, Kind: ErrorKindInvalidRequest, Guidance: "Check model name with /model. Run /doctor to verify provider configuration"}
	default:
		observe.GlobalTrace("default")
		return ClassifiedError{Err: err, Kind: ErrorKindUnknown}
	}
}

// retryDelay calculates exponential backoff delay for a given attempt.
// Formula matches shared.WithRetry: 500ms * 2^attempt, cap 32s, +0-25% jitter.
func retryDelay(attempt int) time.Duration {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	baseDelay := time.Duration(500*math.Pow(2, float64(attempt))) * time.Millisecond
	if baseDelay > 32*time.Second {
		observe.GlobalTrace("if: baseDelay > 32*time.Second")
		baseDelay = 32 * time.Second
	}
	jitter := time.Duration(rand.Float64() * 0.25 * float64(baseDelay))
	observe.GlobalTrace("return: baseDelay + jitter")
	return baseDelay + jitter
}
