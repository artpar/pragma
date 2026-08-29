package openrouter

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/observe"
)

func TestGLM53ModelMetadata(t *testing.T) {
	provider, err := New("test-key", observe.NewEventBus(1), "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if DefaultModel != "z-ai/glm-5.3" {
		t.Fatalf("DefaultModel = %q, want z-ai/glm-5.3", DefaultModel)
	}
	for _, modelID := range []string{DefaultModel, FlashModel} {
		if got, ok := provider.ContextWindow(modelID); !ok || got != 1_310_720 {
			t.Fatalf("ContextWindow(%q) = (%d, %v), want (1310720, true)", modelID, got, ok)
		}
	}
	models := provider.ListModels()
	if !slices.Contains(models, DefaultModel) || !slices.Contains(models, FlashModel) {
		t.Fatalf("ListModels() = %v, want exact and Flash GLM-5.3 variants", models)
	}
}

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
