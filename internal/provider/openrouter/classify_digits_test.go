package openrouter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	oaisdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// TestClassifyPortDigitsDoNotTriggerRetry reproduces RTY-001 deterministically:
// a structured 400 whose request URL port contains a retryable-looking code
// substring ("25003" contains "500") must not be classified retryable.
func TestClassifyPortDigitsDoNotTriggerRetry(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:25003/v1/chat/completions", nil)
	apiErr := &oaisdk.Error{
		StatusCode: http.StatusBadRequest,
		Request:    req,
		Response:   &http.Response{StatusCode: http.StatusBadRequest},
	}
	wrapped := fmt.Errorf("complete: %w", apiErr)
	if got := openrouterClassify(wrapped); got.Retryable {
		t.Fatalf("openrouterClassify() = %#v, want non-retryable: code-like digits in the URL port must not trigger a retry storm", got)
	}
}

// TestClassifyBodyDigitsDoNotTriggerRetry reproduces the same mechanism through
// a real SDK round trip: a 400 whose body text mentions "500" must not retry.
func TestClassifyBodyDigitsDoNotTriggerRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"max_tokens must be <= 500"}}`))
	}))
	defer srv.Close()
	client := oaisdk.NewClient(option.WithAPIKey("test"), option.WithBaseURL(srv.URL))
	var data []byte
	err := client.Post(t.Context(), "chat/completions", map[string]any{
		"model":    "x",
		"messages": []map[string]any{{"role": "user", "content": "hi"}},
	}, &data)
	if err == nil {
		t.Fatal("expected the 400 to surface as an SDK error")
	}
	if got := openrouterClassify(err); got.Retryable {
		t.Fatalf("openrouterClassify() = %#v, want non-retryable: body digits must not retry a permanent 400", got)
	}
}

// TestClassifyStructuredStatusCodesAreHonored is the positive control:
// structured statuses retry exactly where the substring policy intends.
func TestClassifyStructuredStatusCodesAreHonored(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8137/v1/chat/completions", nil)
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
		apiErr := &oaisdk.Error{
			StatusCode: tc.status,
			Request:    req,
			Response:   &http.Response{StatusCode: tc.status},
		}
		got := openrouterClassify(fmt.Errorf("complete: %w", apiErr))
		if got.Retryable != tc.retryable || got.ErrorType != tc.errorType {
			t.Errorf("status %d: openrouterClassify() = %#v, want retryable=%v errorType=%s", tc.status, got, tc.retryable, tc.errorType)
		}
	}
}
