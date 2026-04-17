package anthropic

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

// Package-level aliases for shared provider sentinel errors.
// Kept for backward compatibility within this package and its tests.
var (
	ErrRateLimit       = provider.ErrRateLimit
	ErrOverloaded      = provider.ErrOverloaded
	ErrServerError     = provider.ErrServerError
	ErrAuthentication  = provider.ErrAuthentication
	ErrInvalidRequest  = provider.ErrInvalidRequest
	ErrContextOverflow = provider.ErrContextOverflow
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		observe.GlobalTrace("if: errors.As(err, &apiErr)")
		observe.GlobalTrace("return: classifyAPIError(apiErr)")
		return classifyAPIError(apiErr)
	}

	// Connection-level errors: check most specific types first.
	// DNS errors must come before OpError because DNSError is often
	// wrapped inside OpError, and errors.As unwraps the chain.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		observe.GlobalTrace("if: errors.As(err, &dnsErr)")
		observe.GlobalTrace("return: classifiedError{\n\twrapped:\tfmt.Errorf(\"DNS resolution failed: %w\", ErrServerE...")
		return classifiedError{
			wrapped:   fmt.Errorf("DNS resolution failed: %w", ErrServerError),
			retryable: !dnsErr.IsNotFound,
			errorType: "connection",
		}
	}

	var netErr *net.OpError
	if errors.As(err, &netErr) {
		observe.GlobalTrace("if: errors.As(err, &netErr)")
		observe.GlobalTrace("return: classifiedError{\n\twrapped:\tfmt.Errorf(\"connection error: %w\", ErrServerError)...")
		return classifiedError{
			wrapped:   fmt.Errorf("connection error: %w", ErrServerError),
			retryable: true,
			errorType: "connection",
		}
	}

	// URL errors (wraps net errors for HTTP client)
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		observe.GlobalTrace("if: errors.As(err, &urlErr)")
		observe.GlobalTrace("return: classifiedError{\n\twrapped:\tfmt.Errorf(\"connection error: %w\", ErrServerError)...")
		return classifiedError{
			wrapped:   fmt.Errorf("connection error: %w", ErrServerError),
			retryable: true,
			errorType: "connection",
		}
	}

	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		observe.GlobalTrace("if: errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)")
		observe.GlobalTrace("return: classifiedError{\n\twrapped:\tfmt.Errorf(\"connection closed: %w\", ErrServerError...")
		return classifiedError{
			wrapped:   fmt.Errorf("connection closed: %w", ErrServerError),
			retryable: true,
			errorType: "connection",
		}
	}

	msg := err.Error()
	if strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "ECONNRESET") ||
		strings.Contains(msg, "EPIPE") {
		observe.GlobalTrace("if: strings.Contains(msg, \"connection reset\") ||\n\tstrings.Contains(msg, \"broken p...")
		observe.GlobalTrace("return: classifiedError{\n\twrapped:\tfmt.Errorf(\"connection error: %w\", ErrServerError)...")
		return classifiedError{
			wrapped:   fmt.Errorf("connection error: %w", ErrServerError),
			retryable: true,
			errorType: "connection",
		}
	}
	observe.GlobalTrace("return: classifiedError{\n\twrapped:\terr,\n\tretryable:\tfalse,\n\terrorType:\t\"unknown\",\n}")

	return classifiedError{
		wrapped:   err,
		retryable: false,
		errorType: "unknown",
	}
}

func classifyAPIError(apiErr *sdk.Error) classifiedError {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	retryAfter := parseRetryAfter(apiErr.Response)

	switch apiErr.StatusCode {
	case http.StatusTooManyRequests:
		observe.GlobalTrace("case: http.StatusTooManyRequests")
		return classifiedError{
			wrapped:    fmt.Errorf("%w: %s", ErrRateLimit, apiErr.Error()),
			retryable:  true,
			errorType:  "rate_limit",
			retryAfter: retryAfter,
		}
	case 529:
		observe.GlobalTrace("case: 529")
		return classifiedError{
			wrapped:    fmt.Errorf("%w: %s", ErrOverloaded, apiErr.Error()),
			retryable:  true,
			errorType:  "overloaded",
			retryAfter: retryAfter,
		}
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		observe.GlobalTrace("case: http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnav...")
		return classifiedError{
			wrapped:   fmt.Errorf("%w: %s", ErrServerError, apiErr.Error()),
			retryable: true,
			errorType: "server_error",
		}
	case http.StatusRequestTimeout:
		observe.GlobalTrace("case: http.StatusRequestTimeout")
		return classifiedError{
			wrapped:   fmt.Errorf("%w: %s", ErrServerError, apiErr.Error()),
			retryable: true,
			errorType: "timeout",
		}
	case http.StatusBadRequest:
		observe.GlobalTrace("case: http.StatusBadRequest")
		if isContextOverflow(apiErr) {
			observe.GlobalTrace("return: classifiedError{\n\twrapped:\tfmt.Errorf(\"%w: %s\", ErrContextOverflow, apiErr.Er...")
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
		observe.GlobalTrace("case: http.StatusUnauthorized, http.StatusForbidden")
		return classifiedError{
			wrapped:   fmt.Errorf("%w: %s", ErrAuthentication, apiErr.Error()),
			retryable: false,
			errorType: "authentication",
		}
	default:
		observe.GlobalTrace("default")
		return classifiedError{
			wrapped:   fmt.Errorf("anthropic: HTTP %d: %s", apiErr.StatusCode, apiErr.Error()),
			retryable: false,
			errorType: "unknown",
		}
	}
}

// isContextOverflow checks if a 400 error is specifically about context window limits.
func isContextOverflow(apiErr *sdk.Error) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	raw := apiErr.RawJSON()
	observe.GlobalTrace("return: strings.Contains(raw, \"prompt is too long\") ||\n\tstrings.Contains(raw, \"exceed...")
	return strings.Contains(raw, "prompt is too long") ||
		strings.Contains(raw, "exceeds the maximum") ||
		strings.Contains(raw, "context length")
}

// parseRetryAfter extracts the Retry-After header value as a duration.
// Returns zero if the header is absent or unparseable.
func parseRetryAfter(resp *http.Response) time.Duration {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if resp == nil {
		observe.GlobalTrace("if: resp == nil")
		observe.GlobalTrace("return: 0")
		return 0
	}
	val := resp.Header.Get("Retry-After")
	if val == "" {
		observe.GlobalTrace("if: val == \"\"")
		observe.GlobalTrace("return: 0")
		return 0
	}

	if secs, err := strconv.Atoi(val); err == nil {
		observe.GlobalTrace("if: err == nil")
		observe.GlobalTrace("return: time.Duration(secs) * time.Second")
		return time.Duration(secs) * time.Second
	}

	if t, err := http.ParseTime(val); err == nil {
		observe.GlobalTrace("if: err == nil")
		d := time.Until(t)
		if d > 0 {
			observe.GlobalTrace("if: d > 0")
			observe.GlobalTrace("return: d")
			return d
		}
	}
	observe.GlobalTrace("return: 0")
	return 0
}
