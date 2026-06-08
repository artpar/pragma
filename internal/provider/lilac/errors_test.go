package lilac

import (
	"errors"
	"net/url"
	"testing"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestLilacClassifyRetryableTimeouts(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{
			name: "client timeout text",
			err:  errors.New(`Post "https://api.getlilac.com/v1/chat/completions": net/http: request canceled (Client.Timeout exceeded while awaiting headers)`),
		},
		{
			name: "url timeout",
			err: &url.Error{
				Op:  "Post",
				URL: "https://api.getlilac.com/v1/chat/completions",
				Err: timeoutError{},
			},
		},
		{
			name: "net timeout",
			err:  timeoutError{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := lilacClassify(tc.err)
			if !got.Retryable {
				t.Fatalf("Retryable = false, want true")
			}
			if got.ErrorType != "timeout" {
				t.Fatalf("ErrorType = %q, want timeout", got.ErrorType)
			}
		})
	}
}

func TestLilacClassifyStatusCodes(t *testing.T) {
	for _, msg := range []string{
		"lilac: status 429: rate limited",
		"lilac: status 500: internal server error",
		"lilac: status 502: bad gateway",
		"lilac: status 503: service unavailable",
		"lilac: status 504: gateway timeout",
	} {
		got := lilacClassify(errors.New(msg))
		if !got.Retryable {
			t.Fatalf("%q Retryable = false, want true", msg)
		}
	}
}

func TestLilacClassifyUnknownErrorNotRetryable(t *testing.T) {
	got := lilacClassify(errors.New("invalid request"))
	if got.Retryable {
		t.Fatal("Retryable = true, want false")
	}
	if got.ErrorType != "request_failed" {
		t.Fatalf("ErrorType = %q, want request_failed", got.ErrorType)
	}
}
