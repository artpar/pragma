package anthropic

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
)

// Sentinel errors for Anthropic API error categories.
var (
	ErrRateLimit       = errors.New("anthropic: rate limit exceeded")
	ErrOverloaded      = errors.New("anthropic: overloaded")
	ErrServerError     = errors.New("anthropic: server error")
	ErrAuthentication  = errors.New("anthropic: authentication failed")
	ErrInvalidRequest  = errors.New("anthropic: invalid request")
	ErrContextOverflow = errors.New("anthropic: context window exceeded")
)

// classifiedError holds the result of classifying an API error.
type classifiedError struct {
	wrapped    error
	retryable  bool
	errorType  string
	retryAfter time.Duration
}

// classifyError inspects an error and returns its classification.
// For SDK API errors, extracts status code and retry-after header.
// For other errors, classifies connection-level failures as retryable.
func classifyError(err error) classifiedError {
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		return classifyAPIError(apiErr)
	}

	// Connection-level errors (ECONNRESET, EPIPE, etc.)
	msg := err.Error()
	if strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "ECONNRESET") ||
		strings.Contains(msg, "EPIPE") {
		return classifiedError{
			wrapped:   fmt.Errorf("connection error: %w", ErrServerError),
			retryable: true,
			errorType: "connection",
		}
	}

	return classifiedError{
		wrapped:   err,
		retryable: false,
		errorType: "unknown",
	}
}

func classifyAPIError(apiErr *sdk.Error) classifiedError {
	retryAfter := parseRetryAfter(apiErr.Response)

	switch apiErr.StatusCode {
	case http.StatusTooManyRequests: // 429
		return classifiedError{
			wrapped:    fmt.Errorf("%w: %s", ErrRateLimit, apiErr.Error()),
			retryable:  true,
			errorType:  "rate_limit",
			retryAfter: retryAfter,
		}
	case 529: // Overloaded (Anthropic-specific)
		return classifiedError{
			wrapped:    fmt.Errorf("%w: %s", ErrOverloaded, apiErr.Error()),
			retryable:  true,
			errorType:  "overloaded",
			retryAfter: retryAfter,
		}
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		return classifiedError{
			wrapped:   fmt.Errorf("%w: %s", ErrServerError, apiErr.Error()),
			retryable: true,
			errorType: "server_error",
		}
	case http.StatusRequestTimeout: // 408
		return classifiedError{
			wrapped:   fmt.Errorf("%w: %s", ErrServerError, apiErr.Error()),
			retryable: true,
			errorType: "timeout",
		}
	case http.StatusBadRequest: // 400
		if isContextOverflow(apiErr) {
			return classifiedError{
				wrapped:   fmt.Errorf("%w: %s", ErrContextOverflow, apiErr.Error()),
				retryable: false,
				errorType: "context_overflow",
			}
		}
		return classifiedError{
			wrapped:   fmt.Errorf("%w: %s", ErrInvalidRequest, apiErr.Error()),
			retryable: false,
			errorType: "invalid_request",
		}
	case http.StatusUnauthorized, http.StatusForbidden:
		return classifiedError{
			wrapped:   fmt.Errorf("%w: %s", ErrAuthentication, apiErr.Error()),
			retryable: false,
			errorType: "authentication",
		}
	default:
		return classifiedError{
			wrapped:   fmt.Errorf("anthropic: HTTP %d: %s", apiErr.StatusCode, apiErr.Error()),
			retryable: false,
			errorType: "unknown",
		}
	}
}

// isContextOverflow checks if a 400 error is specifically about context window limits.
func isContextOverflow(apiErr *sdk.Error) bool {
	raw := apiErr.RawJSON()
	return strings.Contains(raw, "prompt is too long") ||
		strings.Contains(raw, "exceeds the maximum") ||
		strings.Contains(raw, "context length")
}

// parseRetryAfter extracts the Retry-After header value as a duration.
// Returns zero if the header is absent or unparseable.
func parseRetryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}
	val := resp.Header.Get("Retry-After")
	if val == "" {
		return 0
	}
	// Try parsing as integer seconds
	if secs, err := strconv.Atoi(val); err == nil {
		return time.Duration(secs) * time.Second
	}
	// Try parsing as HTTP-date
	if t, err := http.ParseTime(val); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}
