package openrouter

import (
	"errors"
	"testing"
	"time"
)

func TestClassifyTransientInFlightBudgetExhaustion(t *testing.T) {
	err := errors.New(`POST "https://openrouter.ai/api/v1/chat/completions": 402 Payment Required {"metadata":{"reason":"in_flight_budget_exhausted","headers":{"Retry-After":"120"}}}`)
	got := openrouterClassify(err)
	if !got.Retryable || got.ErrorType != "rate_limit" || got.RetryAfter != 120*time.Second {
		t.Fatalf("openrouterClassify() = %#v, want retryable rate_limit after 120s", got)
	}
}

func TestClassifyPermanentCreditExhaustion(t *testing.T) {
	err := errors.New(`POST "https://openrouter.ai/api/v1/chat/completions": 402 Payment Required {"metadata":{"limit_source":"openrouter_credits"}}`)
	got := openrouterClassify(err)
	if got.Retryable {
		t.Fatalf("openrouterClassify() = %#v, want permanent failure", got)
	}
}

func TestClassifyStandardRateLimit(t *testing.T) {
	got := openrouterClassify(errors.New("429 Too Many Requests"))
	if !got.Retryable || got.ErrorType != "rate_limit" {
		t.Fatalf("openrouterClassify() = %#v, want retryable rate_limit", got)
	}
}
