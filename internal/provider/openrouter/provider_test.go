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
	for _, modelID := range []string{DefaultModel, FlashModel} {
		if got, ok := provider.ContextWindow(modelID); !ok || got != 1_048_576 {
			t.Fatalf("ContextWindow(%q) = (%d, %v), want (1048576, true)", modelID, got, ok)
		}
	}
	models := provider.ListModels()
	if !slices.Contains(models, DefaultModel) || !slices.Contains(models, FlashModel) {
		t.Fatalf("ListModels() = %v, want exact and Flash GLM-5.3 variants", models)
	}
}
