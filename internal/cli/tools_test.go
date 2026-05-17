package cli

import (
	"testing"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/tool"
)

func TestBaseToolsDoesNotExposeLegacyLSPTool(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	deps := &Deps{
		Bus:      bus,
		Registry: tool.NewRegistry(bus),
	}

	for _, desc := range BaseTools(deps) {
		if desc.Name() == "LSP" {
			t.Fatal("BaseTools exposed legacy LSP tool")
		}
	}
}
