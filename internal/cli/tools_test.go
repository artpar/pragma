package cli

import (
	"testing"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/tool"
	"github.com/artpar/pragma/internal/toolset"
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

func TestBaseToolsUseLowercaseApplyPatchPrimary(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	deps := &Deps{
		Bus:      bus,
		Registry: tool.NewRegistry(bus),
	}

	var primary, legacy bool
	for _, desc := range BaseTools(deps) {
		switch desc.Name() {
		case "apply_patch":
			primary = true
		case "ApplyPatch":
			legacy = true
		}
	}
	if !primary {
		t.Fatal("BaseTools should include apply_patch")
	}
	if !legacy {
		t.Fatal("BaseTools should keep ApplyPatch compatibility alias")
	}
}

func TestToolsetHidesBuiltinTools(t *testing.T) {
	deps := &Deps{
		Toolset: &toolset.Compiled{
			Name: "idea",
			Definition: toolset.Definition{
				MCPServers:          []string{"jetbrains-*"},
				Tools:               []string{"com.intellij.*"},
				IncludeBuiltinTools: false,
			},
		},
	}

	if shouldRegisterBuiltinTool(deps, "Bash") {
		t.Fatal("Bash should not be registered for idea toolset")
	}
	if shouldRegisterBuiltinTool(deps, "Edit") {
		t.Fatal("Edit should not be registered for idea toolset")
	}
	if shouldRegisterBuiltinTool(deps, "ListMcpResourcesTool") {
		t.Fatal("ListMcpResourcesTool should not be registered for idea toolset")
	}
	if !shouldRegisterBuiltinTool(deps, "tool_result.read") {
		t.Fatal("tool_result.read should remain available as runtime plumbing")
	}
}

func TestToolsetLegacyApplyPatchSelectsPrimary(t *testing.T) {
	deps := &Deps{
		Toolset: &toolset.Compiled{
			Name: "legacy",
			Definition: toolset.Definition{
				Tools:               []string{"ApplyPatch"},
				IncludeBuiltinTools: true,
			},
		},
	}

	if !shouldRegisterBuiltinTool(deps, "apply_patch") {
		t.Fatal("legacy ApplyPatch selector should register apply_patch")
	}
	if !shouldRegisterBuiltinTool(deps, "ApplyPatch") {
		t.Fatal("legacy ApplyPatch selector should keep compatibility alias")
	}
}

func TestToolsetCanExposeSelectedBuiltins(t *testing.T) {
	deps := &Deps{
		Toolset: &toolset.Compiled{
			Name: "read-only",
			Definition: toolset.Definition{
				Tools:                   []string{"Read", "ListMcpResourcesTool"},
				IncludeBuiltinTools:     true,
				IncludeMCPResourceTools: true,
			},
		},
	}

	if !shouldRegisterBuiltinTool(deps, "Read") {
		t.Fatal("Read should be registered")
	}
	if !shouldRegisterBuiltinTool(deps, "ListMcpResourcesTool") {
		t.Fatal("ListMcpResourcesTool should be registered")
	}
	if shouldRegisterBuiltinTool(deps, "Bash") {
		t.Fatal("Bash should not be registered")
	}
}
