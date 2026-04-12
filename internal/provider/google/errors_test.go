package google

import (
	"errors"
	"net/http"
	"testing"
)

func TestClassifyHTTPError(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantType  string
		wantRetry bool
	}{
		{"rate_limit", 429, `{"error":{"message":"quota exceeded"}}`, "rate_limit", true},
		{"overloaded", 503, `{"error":{"message":"overloaded"}}`, "overloaded", true},
		{"server_error_500", 500, `{"error":{"message":"internal"}}`, "server_error", true},
		{"server_error_502", 502, `{"error":{"message":"bad gateway"}}`, "server_error", true},
		{"timeout", 408, `{"error":{"message":"timeout"}}`, "timeout", true},
		{"auth_401", 401, `{"error":{"message":"invalid key"}}`, "authentication", false},
		{"auth_403", 403, `{"error":{"message":"forbidden"}}`, "authentication", false},
		{"bad_request", 400, `{"error":{"message":"invalid"}}`, "invalid_request", false},
		{"context_overflow", 400, `{"error":{"message":"exceeds the maximum number of tokens"}}`, "context_overflow", false},
		{"unknown", 418, `{"error":{"message":"teapot"}}`, "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			got := classifyHTTPError(tt.status, []byte(tt.body), resp)
			if got.errorType != tt.wantType {
				t.Errorf("type=%q, want %q", got.errorType, tt.wantType)
			}
			if got.retryable != tt.wantRetry {
				t.Errorf("retryable=%v, want %v", got.retryable, tt.wantRetry)
			}
		})
	}
}

func TestClassifyError_Sentinel(t *testing.T) {
	got := classifyHTTPError(429, []byte(`{"error":{"message":"limit"}}`), nil)
	if !errors.Is(got.wrapped, ErrRateLimit) {
		t.Error("429 should wrap ErrRateLimit")
	}

	got = classifyHTTPError(401, []byte(`{"error":{"message":"bad key"}}`), nil)
	if !errors.Is(got.wrapped, ErrAuthentication) {
		t.Error("401 should wrap ErrAuthentication")
	}
}

func TestIsContextOverflow(t *testing.T) {
	tests := []struct {
		body string
		want bool
	}{
		{`exceeds the maximum`, true},
		{`token limit reached`, true},
		{`too many tokens in request`, true},
		{`context length exceeded`, true},
		{`invalid parameter`, false},
	}

	for _, tt := range tests {
		if got := isContextOverflow([]byte(tt.body)); got != tt.want {
			t.Errorf("isContextOverflow(%q)=%v, want %v", tt.body, got, tt.want)
		}
	}
}

func TestExtractErrorMessage(t *testing.T) {
	got := extractErrorMessage([]byte(`{"error":{"code":400,"message":"bad input","status":"INVALID_ARGUMENT"}}`))
	if got != "bad input" {
		t.Errorf("got %q, want %q", got, "bad input")
	}

	got = extractErrorMessage([]byte("plain text"))
	if got != "plain text" {
		t.Errorf("got %q, want %q", got, "plain text")
	}
}
