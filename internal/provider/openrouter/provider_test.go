package openrouter

import (
	"slices"
	"testing"

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
	if got, ok := provider.ContextWindow(DefaultModel); !ok || got != 1_048_576 {
		t.Fatalf("ContextWindow(%q) = (%d, %v), want (1048576, true)", DefaultModel, got, ok)
	}
	if got, ok := provider.ContextWindow(FlashModel); !ok || got != 1_310_720 {
		t.Fatalf("ContextWindow(%q) = (%d, %v), want (1310720, true)", FlashModel, got, ok)
	}
	models := provider.ListModels()
	if !slices.Contains(models, DefaultModel) || !slices.Contains(models, FlashModel) {
		t.Fatalf("ListModels() = %v, want exact and Flash GLM-5.3 variants", models)
	}
}
