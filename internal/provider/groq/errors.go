package groq

import (
	"encoding/json"
	"errors"
	"fmt"
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
	var he *httpError
	if errors.As(err, &he) {
		return classifyHTTPError(he.statusCode, he.body, he.resp)
	}

	// Connection-level errors: check most specific types first.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return classifiedError{
			wrapped:   fmt.Errorf("DNS resolution failed: %w", ErrServerError),
			retryable: !dnsErr.IsNotFound,
			errorType: "connection",
		}
	}

	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return classifiedError{
			wrapped:   fmt.Errorf("connection error: %w", ErrServerError),
			retryable: true,
			errorType: "connection",
		}
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return classifiedError{
			wrapped:   fmt.Errorf("connection error: %w", ErrServerError),
			retryable: true,
			errorType: "connection",
		}
	}

	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
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

// classifyHTTPError classifies an error based on HTTP status code and response body.
func classifyHTTPError(statusCode int, body []byte, resp *http.Response) classifiedError {
	retryAfter := parseRetryAfter(resp)
	errMsg := extractErrorMessage(body)

	switch statusCode {
	case http.StatusTooManyRequests: // 429
		return classifiedError{
			wrapped:    fmt.Errorf("%w: %s", ErrRateLimit, errMsg),
			retryable:  true,
			errorType:  "rate_limit",
			retryAfter: retryAfter,
		}
	case 529: // overloaded
		return classifiedError{
			wrapped:    fmt.Errorf("%w: %s", ErrOverloaded, errMsg),
			retryable:  true,
			errorType:  "overloaded",
			retryAfter: retryAfter,
		}
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		return classifiedError{
			wrapped:   fmt.Errorf("%w: %s", ErrServerError, errMsg),
			retryable: true,
			errorType: "server_error",
		}
	case http.StatusRequestTimeout: // 408
		return classifiedError{
			wrapped:   fmt.Errorf("%w: %s", ErrServerError, errMsg),
			retryable: true,
			errorType: "timeout",
		}
	case http.StatusBadRequest: // 400
		if isContextOverflow(body) {
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
		return classifiedError{
			wrapped:   fmt.Errorf("%w: %s", ErrAuthentication, errMsg),
			retryable: false,
			errorType: "authentication",
		}
	default:
		return classifiedError{
			wrapped:   fmt.Errorf("groq: HTTP %d: %s", statusCode, errMsg),
			retryable: false,
			errorType: "unknown",
		}
	}
}

// isContextOverflow checks if a 400 error body indicates context window overflow.
func isContextOverflow(body []byte) bool {
	s := string(body)
	return strings.Contains(s, "context_length_exceeded") ||
		strings.Contains(s, "prompt is too long") ||
		strings.Contains(s, "exceeds the maximum") ||
		strings.Contains(s, "context length")
}

// extractErrorMessage parses the Groq JSON error body for a human-readable message.
func extractErrorMessage(body []byte) string {
	var errResp wireErrorResponse
	if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
		return errResp.Error.Message
	}
	if len(body) > 200 {
		return string(body[:200])
	}
	return string(body)
}

// parseRetryAfter extracts the Retry-After header value as a duration.
func parseRetryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}
	val := resp.Header.Get("Retry-After")
	if val == "" {
		return 0
	}
	if secs, err := strconv.Atoi(val); err == nil {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(val); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}
