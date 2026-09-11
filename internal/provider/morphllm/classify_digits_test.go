package morphllm

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	llmerrors "github.com/mozilla-ai/any-llm-go/errors"
	oaisdk "github.com/openai/openai-go"
)

func apiErrorWithPort(t *testing.T, status int, port string) *oaisdk.Error {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:"+port+"/v1/chat/completions", nil)
	return &oaisdk.Error{
		StatusCode: status,
		Request:    req,
		Response:   &http.Response{StatusCode: status},
	}
}

// TestClassifyPortDigitsDoNotTriggerRetry reproduces RTY-002 for the current
// completeWire shape: a raw SDK 400 whose URL port contains a retryable-looking
// code substring ("25003" contains "500") must not be classified retryable.
func TestClassifyPortDigitsDoNotTriggerRetry(t *testing.T) {
	apiErr := apiErrorWithPort(t, http.StatusBadRequest, "25003")
	if got := morphClassify(apiErr); got.Retryable {
		t.Fatalf("morphClassify() = %#v, want non-retryable: code-like digits in the URL port must not trigger a retry storm", got)
	}
}

// TestClassifyWrappedPortDigitsDoNotTriggerRetry reproduces RTY-002 for the
// any-llm adapter shape captured in the authentic 2026-09-11T07:53:58Z event:
// an InvalidRequestError-wrapped 400 must be non-retryable regardless of the
// digits embedded in the underlying URL or body.
func TestClassifyWrappedPortDigitsDoNotTriggerRetry(t *testing.T) {
	apiErr := apiErrorWithPort(t, http.StatusBadRequest, "35001")
	wrapped := llmerrors.NewInvalidRequestError("openai-compatible", apiErr)
	if got := morphClassify(fmt.Errorf("complete: %w", wrapped)); got.Retryable {
		t.Fatalf("morphClassify() = %#v, want non-retryable: wrapped 400 with code-like port digits must not retry", got)
	}
}

// TestClassifyStructuredStatusCodesAreHonored is the positive control:
// structured statuses retry exactly where the substring policy intends.
func TestClassifyStructuredStatusCodesAreHonored(t *testing.T) {
	cases := []struct {
		status    int
		retryable bool
		errorType string
	}{
		{http.StatusTooManyRequests, true, "rate_limit"},
		{http.StatusInternalServerError, true, "server_error"},
		{http.StatusBadGateway, true, "server_error"},
		{http.StatusServiceUnavailable, true, "server_error"},
		{http.StatusGatewayTimeout, true, "server_error"},
		{http.StatusBadRequest, false, "request_failed"},
		{http.StatusUnauthorized, false, "request_failed"},
		{http.StatusPaymentRequired, false, "request_failed"},
	}
	for _, tc := range cases {
		apiErr := apiErrorWithPort(t, tc.status, "8137")
		got := morphClassify(fmt.Errorf("complete: %w", apiErr))
		if got.Retryable != tc.retryable || got.ErrorType != tc.errorType {
			t.Errorf("status %d: morphClassify() = %#v, want retryable=%v errorType=%s", tc.status, got, tc.retryable, tc.errorType)
		}
	}
}

// TestClassifyStructuredRateLimitKeepsRetryAfter verifies the any-llm typed
// rate limit carries its structured Retry-After into the classification.
func TestClassifyStructuredRateLimitKeepsRetryAfter(t *testing.T) {
	apiErr := apiErrorWithPort(t, http.StatusTooManyRequests, "8137")
	rate := llmerrors.NewRateLimitError("openai-compatible", apiErr)
	rate.RetryAfter = 120
	got := morphClassify(rate)
	if !got.Retryable || got.ErrorType != "rate_limit" || got.RetryAfter != 120*time.Second {
		t.Fatalf("morphClassify() = %#v, want retryable rate_limit with 120s RetryAfter", got)
	}
}

// TestClassifyPlainStringFallbackPreserved guards the substring fallback for
// transport-level errors without structured data (same contract as the
// openrouter classifier's plain-string test).
func TestClassifyPlainStringFallbackPreserved(t *testing.T) {
	if got := morphClassify(errors.New("429 Too Many Requests")); !got.Retryable || got.ErrorType != "rate_limit" {
		t.Fatalf("morphClassify() = %#v, want retryable rate_limit via fallback", got)
	}
}
