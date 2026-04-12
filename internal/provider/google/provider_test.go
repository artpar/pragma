package google

import (
	"testing"

	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
)

func TestProviderName(t *testing.T) {
	bus := observe.NewEventBus(16)
	p := New("test-key", bus)
	if p.Name() != "google" {
		t.Errorf("Name()=%q, want google", p.Name())
	}
}

func TestSupportsFeature(t *testing.T) {
	bus := observe.NewEventBus(16)
	p := New("test-key", bus)

	tests := []struct {
		feature provider.Feature
		want    bool
	}{
		{provider.FeatureToolUse, true},
		{provider.FeatureStreaming, true},
		{provider.FeatureImages, true},
		{provider.FeatureThinking, true},
		{provider.FeaturePrefixCaching, false},
	}

	for _, tt := range tests {
		got := p.SupportsFeature(tt.feature)
		if got != tt.want {
			t.Errorf("SupportsFeature(%v)=%v, want %v", tt.feature, got, tt.want)
		}
	}
}

func TestPricing(t *testing.T) {
	bus := observe.NewEventBus(16)
	p := New("test-key", bus)

	pricing, ok := p.Pricing("gemini-2.5-flash")
	if !ok {
		t.Fatal("pricing not found")
	}
	if pricing.InputPerMToken != 0.30 {
		t.Errorf("input price=%v, want 0.30", pricing.InputPerMToken)
	}
	if pricing.OutputPerMToken != 2.50 {
		t.Errorf("output price=%v, want 2.50", pricing.OutputPerMToken)
	}

	_, ok = p.Pricing("nonexistent")
	if ok {
		t.Error("should not find nonexistent model")
	}
}

func TestContextWindow(t *testing.T) {
	bus := observe.NewEventBus(16)
	p := New("test-key", bus)

	cw, ok := p.ContextWindow("gemini-2.5-pro")
	if !ok {
		t.Fatal("context window not found")
	}
	if cw != 1_048_576 {
		t.Errorf("context window=%d, want 1048576", cw)
	}

	cw, ok = p.ContextWindow("unknown-model")
	if ok {
		t.Error("should not find unknown model")
	}
	if cw != 1_048_576 {
		t.Errorf("fallback context window=%d, want 1048576", cw)
	}
}

func TestWithOptions(t *testing.T) {
	bus := observe.NewEventBus(16)
	p := New("key", bus,
		WithMaxRetries(3),
		WithBaseURL("http://localhost:8080"),
	)

	if p.maxRetries != 3 {
		t.Errorf("maxRetries=%d, want 3", p.maxRetries)
	}
	if p.baseURL != "http://localhost:8080" {
		t.Errorf("baseURL=%q", p.baseURL)
	}
}
