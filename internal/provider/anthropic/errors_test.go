package anthropic

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
)

func makeAPIError(statusCode int) *sdk.Error {
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	resp := &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{},
		Request:    req,
	}
	return &sdk.Error{
		StatusCode: statusCode,
		Request:    req,
		Response:   resp,
	}
}

func makeAPIErrorWithRetryAfter(statusCode int, retryAfter string) *sdk.Error {
	apiErr := makeAPIError(statusCode)
	apiErr.Response.Header.Set("Retry-After", retryAfter)
	return apiErr
}

func TestClassifyErrorStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		retryable  bool
		errorType  string
		sentinel   error
	}{
		{"429 rate limit", 429, true, "rate_limit", ErrRateLimit},
		{"529 overloaded", 529, true, "overloaded", ErrOverloaded},
		{"500 server error", 500, true, "server_error", ErrServerError},
		{"502 bad gateway", 502, true, "server_error", ErrServerError},
		{"503 service unavailable", 503, true, "server_error", ErrServerError},
		{"408 timeout", 408, true, "timeout", ErrServerError},
		{"400 invalid request", 400, false, "invalid_request", ErrInvalidRequest},
		{"401 unauthorized", 401, false, "authentication", ErrAuthentication},
		{"403 forbidden", 403, false, "authentication", ErrAuthentication},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiErr := makeAPIError(tt.statusCode)
			result := classifyError(apiErr)

			if result.retryable != tt.retryable {
				t.Errorf("retryable: got %v, want %v", result.retryable, tt.retryable)
			}
			if result.errorType != tt.errorType {
				t.Errorf("errorType: got %q, want %q", result.errorType, tt.errorType)
			}
			if !errors.Is(result.wrapped, tt.sentinel) {
				t.Errorf("sentinel: got %v, want %v", result.wrapped, tt.sentinel)
			}
		})
	}
}

func TestClassifyErrorRetryAfter(t *testing.T) {
	apiErr := makeAPIErrorWithRetryAfter(429, "5")
	result := classifyError(apiErr)

	if result.retryAfter.Seconds() != 5 {
		t.Errorf("retryAfter: got %v, want 5s", result.retryAfter)
	}
}

func TestClassifyErrorRetryAfterMissing(t *testing.T) {
	apiErr := makeAPIError(429)
	result := classifyError(apiErr)

	if result.retryAfter != 0 {
		t.Errorf("retryAfter: got %v, want 0", result.retryAfter)
	}
}

func TestClassifyErrorConnectionReset(t *testing.T) {
	err := fmt.Errorf("read tcp: connection reset by peer")
	result := classifyError(err)

	if !result.retryable {
		t.Error("expected connection reset to be retryable")
	}
	if result.errorType != "connection" {
		t.Errorf("errorType: got %q, want %q", result.errorType, "connection")
	}
	if !errors.Is(result.wrapped, ErrServerError) {
		t.Errorf("sentinel: got %v, want ErrServerError", result.wrapped)
	}
}

func TestClassifyErrorDNSNotFound(t *testing.T) {
	err := &net.DNSError{Err: "no such host", Name: "api.anthropic.com", IsNotFound: true}
	result := classifyError(err)
	if result.retryable {
		t.Error("NXDOMAIN should not be retryable")
	}
	if result.errorType != "connection" {
		t.Errorf("errorType: got %q, want %q", result.errorType, "connection")
	}
}

func TestClassifyErrorDNSTransient(t *testing.T) {
	err := &net.DNSError{Err: "temporary failure", Name: "api.anthropic.com", IsNotFound: false}
	result := classifyError(err)
	if !result.retryable {
		t.Error("transient DNS error should be retryable")
	}
}

func TestClassifyErrorEOF(t *testing.T) {
	result := classifyError(io.ErrUnexpectedEOF)
	if !result.retryable {
		t.Error("unexpected EOF should be retryable")
	}
	if result.errorType != "connection" {
		t.Errorf("errorType: got %q, want %q", result.errorType, "connection")
	}
}

func TestClassifyErrorUnknown(t *testing.T) {
	err := fmt.Errorf("something completely different")
	result := classifyError(err)

	if result.retryable {
		t.Error("expected unknown error to not be retryable")
	}
	if result.errorType != "unknown" {
		t.Errorf("errorType: got %q, want %q", result.errorType, "unknown")
	}
}
