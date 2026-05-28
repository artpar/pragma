package cli

import (
	"testing"

	"github.com/artpar/pragma/internal/mcp"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/tool"
	toolbash "github.com/artpar/pragma/internal/tools/bash"
	"github.com/artpar/pragma/internal/toolset"
	"github.com/spf13/cobra"
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

func TestBaseToolsDoesNotExposeReadTool(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	deps := &Deps{
		Bus:      bus,
		Registry: tool.NewRegistry(bus),
	}

	for _, desc := range BaseTools(deps) {
		switch desc.Name() {
		case "Read", "ReadMcpResourceTool":
			t.Fatalf("BaseTools should not expose %s", desc.Name())
		}
	}
}

func TestBaseToolsDoesNotExposeMCPResourceReadTool(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	deps := &Deps{
		Bus:      bus,
		Registry: tool.NewRegistry(bus),
	}
	deps.McpManager = mcp.NewManager(bus, deps.Registry)

	var foundList bool
	for _, desc := range BaseTools(deps) {
		switch desc.Name() {
		case "ListMcpResourcesTool":
			foundList = true
		case "ReadMcpResourceTool":
			t.Fatal("BaseTools should not expose ReadMcpResourceTool")
		}
	}
	if !foundList {
		t.Fatal("BaseTools should still expose ListMcpResourcesTool when MCP is configured")
	}
}

func TestBaseToolsExposeCodexPlanToolOnly(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	deps := &Deps{
		Bus:      bus,
		Registry: tool.NewRegistry(bus),
	}

	var foundUpdatePlan bool
	for _, desc := range BaseTools(deps) {
		switch desc.Name() {
		case "update_plan":
			foundUpdatePlan = true
		case "TodoWrite", "EnterPlanMode", "ExitPlanMode":
			t.Fatalf("BaseTools exposed old plan tool %q", desc.Name())
		}
	}
	if !foundUpdatePlan {
		t.Fatal("BaseTools should expose update_plan")
	}
}

func TestApplyToolFiltersCanExposeBenchMinimalSurface(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	registry := tool.NewRegistry(bus)
	deps := &Deps{
		Bus:      bus,
		Registry: registry,
	}

	for _, desc := range BaseTools(deps) {
		if err := registry.Register(desc); err != nil {
			t.Fatalf("register %s: %v", desc.Name(), err)
		}
	}
	registry.SetHidden(map[string]bool{"ApplyPatch": true})

	cmd := &cobra.Command{}
	cmd.Flags().String("allowed-tools", "", "")
	cmd.Flags().String("disallowed-tools", "", "")
	if err := cmd.Flags().Set("allowed-tools", "Bash"); err != nil {
		t.Fatal(err)
	}

	applyToolFilters(cmd, registry)

	got := make(map[string]bool)
	for _, desc := range registry.List() {
		got[desc.Name()] = true
	}

	if len(got) != 1 {
		t.Fatalf("filtered registry has %d tools, want 1: %v", len(got), got)
	}
	if !got["Bash"] {
		t.Fatalf("filtered registry missing Bash: %v", got)
	}
	if got["update_plan"] {
		t.Fatalf("filtered registry should not expose update_plan: %v", got)
	}

	bashDesc, ok := registry.Get("Bash")
	if !ok {
		t.Fatal("filtered registry missing Bash")
	}
	bashTool, ok := bashDesc.(*toolbash.Tool)
	if !ok {
		t.Fatalf("Bash descriptor type = %T, want *bash.Tool", bashDesc)
	}
	if bashTool.PatchMode {
		t.Fatal("Bash PatchMode should be disabled when apply_patch is filtered out")
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
				Tools:                   []string{"Grep", "ListMcpResourcesTool"},
				IncludeBuiltinTools:     true,
				IncludeMCPResourceTools: true,
			},
		},
	}

	if !shouldRegisterBuiltinTool(deps, "Grep") {
		t.Fatal("Grep should be registered")
	}
	if !shouldRegisterBuiltinTool(deps, "ListMcpResourcesTool") {
		t.Fatal("ListMcpResourcesTool should be registered")
	}
	if shouldRegisterBuiltinTool(deps, "Bash") {
		t.Fatal("Bash should not be registered")
	}
}
