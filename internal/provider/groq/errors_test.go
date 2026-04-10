package groq

import (
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestClassifyHTTPError_RateLimit(t *testing.T) {
	resp := &http.Response{Header: http.Header{"Retry-After": {"5"}}}
	c := classifyHTTPError(429, []byte(`{"error":{"message":"rate limit"}}`), resp)
	if !c.retryable {
		t.Error("429 should be retryable")
	}
	if c.errorType != "rate_limit" {
		t.Errorf("errorType=%q, want rate_limit", c.errorType)
	}
	if c.retryAfter != 5*time.Second {
		t.Errorf("retryAfter=%v, want 5s", c.retryAfter)
	}
	if !errors.Is(c.wrapped, ErrRateLimit) {
		t.Error("should wrap ErrRateLimit")
	}
}

func TestClassifyHTTPError_ServerError(t *testing.T) {
	for _, code := range []int{500, 502, 503} {
		c := classifyHTTPError(code, []byte("error"), nil)
		if !c.retryable {
			t.Errorf("%d should be retryable", code)
		}
		if c.errorType != "server_error" {
			t.Errorf("%d errorType=%q, want server_error", code, c.errorType)
		}
	}
}

func TestClassifyHTTPError_Auth(t *testing.T) {
	for _, code := range []int{401, 403} {
		c := classifyHTTPError(code, []byte("unauthorized"), nil)
		if c.retryable {
			t.Errorf("%d should not be retryable", code)
		}
		if !errors.Is(c.wrapped, ErrAuthentication) {
			t.Errorf("%d should wrap ErrAuthentication", code)
		}
	}
}

func TestClassifyHTTPError_ContextOverflow(t *testing.T) {
	body := []byte(`{"error":{"message":"context_length_exceeded"}}`)
	c := classifyHTTPError(400, body, nil)
	if !errors.Is(c.wrapped, ErrContextOverflow) {
		t.Error("should wrap ErrContextOverflow")
	}
}

func TestClassifyHTTPError_InvalidRequest(t *testing.T) {
	body := []byte(`{"error":{"message":"bad parameter"}}`)
	c := classifyHTTPError(400, body, nil)
	if !errors.Is(c.wrapped, ErrInvalidRequest) {
		t.Error("should wrap ErrInvalidRequest")
	}
}

func TestClassifyError_EOF(t *testing.T) {
	c := classifyError(io.EOF)
	if !c.retryable {
		t.Error("EOF should be retryable")
	}
	if c.errorType != "connection" {
		t.Errorf("errorType=%q, want connection", c.errorType)
	}
}

func TestClassifyError_HTTPError(t *testing.T) {
	err := &httpError{statusCode: 429, body: []byte("rate limit"), message: "429"}
	c := classifyError(err)
	if c.errorType != "rate_limit" {
		t.Errorf("errorType=%q, want rate_limit", c.errorType)
	}
}

func TestExtractErrorMessage(t *testing.T) {
	body := []byte(`{"error":{"message":"something went wrong","type":"invalid_request_error"}}`)
	msg := extractErrorMessage(body)
	if msg != "something went wrong" {
		t.Errorf("got %q, want 'something went wrong'", msg)
	}
}

func TestExtractErrorMessage_InvalidJSON(t *testing.T) {
	body := []byte("not json")
	msg := extractErrorMessage(body)
	if msg != "not json" {
		t.Errorf("got %q, want 'not json'", msg)
	}
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	resp := &http.Response{Header: http.Header{"Retry-After": {"10"}}}
	d := parseRetryAfter(resp)
	if d != 10*time.Second {
		t.Errorf("got %v, want 10s", d)
	}
}

func TestParseRetryAfter_Nil(t *testing.T) {
	d := parseRetryAfter(nil)
	if d != 0 {
		t.Errorf("got %v, want 0", d)
	}
}
