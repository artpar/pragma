package groq

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/artpar/gogent/internal/observe"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors for Groq API error categories.
var (
	ErrRateLimit       = errors.New("groq: rate limit exceeded")
	ErrOverloaded      = errors.New("groq: service overloaded")
	ErrServerError     = errors.New("groq: server error")
	ErrAuthentication  = errors.New("groq: authentication failed")
	ErrInvalidRequest  = errors.New("groq: invalid request")
	ErrContextOverflow = errors.New("groq: context window exceeded")
)

// classifiedError holds the result of classifying an API error.
type classifiedError struct {
	wrapped    error
	retryable  bool
	errorType  string
	retryAfter time.Duration
}

// httpError wraps an HTTP response error with status code and body.
type httpError struct {
	statusCode int
	body       []byte
	resp       *http.Response
	message    string
}

func (e *httpError) Error() string { return e.message }

// classifyError inspects an error and returns its classification.
func classifyError(err error) classifiedError {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var he *httpError
	if errors.As(err, &he) {
		observe.GlobalTrace("if: errors.As(err, &he)")
		observe.GlobalTrace("return: classifyHTTPError(he.statusCode, he.body, he.resp)")
		return classifyHTTPError(he.statusCode, he.body, he.resp)
	}

	// Connection-level errors: check most specific types first.
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

// classifyHTTPError classifies an error based on HTTP status code and response body.
func classifyHTTPError(statusCode int, body []byte, resp *http.Response) classifiedError {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	retryAfter := parseRetryAfter(resp)
	errMsg := extractErrorMessage(body)

	switch statusCode {
	case http.StatusTooManyRequests:
		observe.GlobalTrace("case: http.StatusTooManyRequests")
		return classifiedError{
			wrapped:    fmt.Errorf("%w: %s", ErrRateLimit, errMsg),
			retryable:  true,
			errorType:  "rate_limit",
			retryAfter: retryAfter,
		}
	case 529:
		observe.GlobalTrace("case: 529")
		return classifiedError{
			wrapped:    fmt.Errorf("%w: %s", ErrOverloaded, errMsg),
			retryable:  true,
			errorType:  "overloaded",
			retryAfter: retryAfter,
		}
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		observe.GlobalTrace("case: http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnav...")
		return classifiedError{
			wrapped:   fmt.Errorf("%w: %s", ErrServerError, errMsg),
			retryable: true,
			errorType: "server_error",
		}
	case http.StatusRequestTimeout:
		observe.GlobalTrace("case: http.StatusRequestTimeout")
		return classifiedError{
			wrapped:   fmt.Errorf("%w: %s", ErrServerError, errMsg),
			retryable: true,
			errorType: "timeout",
		}
	case http.StatusBadRequest:
		observe.GlobalTrace("case: http.StatusBadRequest")
		if isContextOverflow(body) {
			observe.GlobalTrace("return: classifiedError{\n\twrapped:\tfmt.Errorf(\"%w: %s\", ErrContextOverflow, errMsg),\n...")
			return classifiedError{
				wrapped:   fmt.Errorf("%w: %s", ErrContextOverflow, errMsg),
				retryable: false,
				errorType: "context_overflow",
			}
		}
		return classifiedError{
			wrapped:   fmt.Errorf("%w: %s", ErrInvalidRequest, errMsg),
			retryable: false,
			errorType: "invalid_request",
		}
	case http.StatusUnauthorized, http.StatusForbidden:
		observe.GlobalTrace("case: http.StatusUnauthorized, http.StatusForbidden")
		return classifiedError{
			wrapped:   fmt.Errorf("%w: %s", ErrAuthentication, errMsg),
			retryable: false,
			errorType: "authentication",
		}
	default:
		observe.GlobalTrace("default")
		return classifiedError{
			wrapped:   fmt.Errorf("groq: HTTP %d: %s", statusCode, errMsg),
			retryable: false,
			errorType: "unknown",
		}
	}
}

// isContextOverflow checks if a 400 error body indicates context window overflow.
func isContextOverflow(body []byte) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	s := string(body)
	observe.GlobalTrace("return: strings.Contains(s, \"context_length_exceeded\") ||\n\tstrings.Contains(s, \"promp...")
	return strings.Contains(s, "context_length_exceeded") ||
		strings.Contains(s, "prompt is too long") ||
		strings.Contains(s, "exceeds the maximum") ||
		strings.Contains(s, "context length")
}

// extractErrorMessage parses the Groq JSON error body for a human-readable message.
func extractErrorMessage(body []byte) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var errResp wireErrorResponse
	if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
		observe.GlobalTrace("if: json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != \"\"")
		observe.GlobalTrace("return: errResp.Error.Message")
		return errResp.Error.Message
	}
	if len(body) > 200 {
		observe.GlobalTrace("if: len(body) > 200")
		observe.GlobalTrace("return: string(body[:200])")
		return string(body[:200])
	}
	observe.GlobalTrace("return: string(body)")
	return string(body)
}

// parseRetryAfter extracts the Retry-After header value as a duration.
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
