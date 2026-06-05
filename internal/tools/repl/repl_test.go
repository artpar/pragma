package repl

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
)

// echoTool is a minimal tool that echoes its input.
type echoTool struct {
	name string
}

func (e *echoTool) Name() string                 { return e.name }
func (e *echoTool) Description() string          { return "echo tool" }
func (e *echoTool) InputSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (e *echoTool) Flags() tool.ToolFlags        { return tool.ToolFlags{} }
func (e *echoTool) CheckPerm(_ context.Context, _ json.RawMessage, _ permission.Checker) permission.CheckResult {
	return permission.CheckResult{Decision: permission.DecisionAllow}
}
func (e *echoTool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	return tool.InvokeResult{Content: "echo: " + string(input)}, nil
}

type allowChecker struct{}

func (allowChecker) Check(_ context.Context, _ string, _ string) permission.CheckResult {
	return permission.CheckResult{Decision: permission.DecisionAllow}
}
func (allowChecker) AddSessionRule(_ permission.Rule) {}
func (allowChecker) AddPersistentRule(_ permission.Rule) error {
	return nil
}

func newTestREPL(reg *tool.Registry, bus *observe.EventBus) *Tool {
	return &Tool{
		Registry:     reg,
		Orchestrator: tool.NewOrchestrator(reg, allowChecker{}, &permission.NonInteractivePrompter{}, bus),
		Bus:          bus,
	}
}

func TestPrimitiveToolNames(t *testing.T) {
	expected := []string{"Read", "Write", "Edit", "Glob", "Grep", "Bash", "NotebookEdit", "Agent"}
	for _, name := range expected {
		if !PrimitiveToolNames[name] {
			t.Errorf("PrimitiveToolNames missing %q", name)
		}
	}
	if len(PrimitiveToolNames) != len(expected) {
		t.Errorf("PrimitiveToolNames has %d entries, want %d", len(PrimitiveToolNames), len(expected))
	}
}

func TestInvoke_SingleOperation(t *testing.T) {
	bus := observe.NewEventBus(100)
	reg := tool.NewRegistry(bus)
	reg.Register(&echoTool{name: "Bash"})

	tl := newTestREPL(reg, bus)

	input := `{"operations": [{"tool": "Bash", "input": {"command": "echo hello"}}]}`
	result, err := tl.Invoke(context.Background(), json.RawMessage(input), nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "[0] Bash:") {
		t.Errorf("Content = %q, want '[0] Bash:'", result.Content)
	}
	if !strings.Contains(result.Content, "echo:") {
		t.Errorf("Content = %q, want echo output", result.Content)
	}
}

func TestInvoke_UnauthorizedTool(t *testing.T) {
	bus := observe.NewEventBus(100)
	reg := tool.NewRegistry(bus)

	tl := newTestREPL(reg, bus)

	input := `{"operations": [{"tool": "Config", "input": {}}]}`
	result, err := tl.Invoke(context.Background(), json.RawMessage(input), nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "not a REPL-allowed tool") {
		t.Errorf("Content = %q, want 'not a REPL-allowed tool'", result.Content)
	}
}

func TestInvoke_MissingTool(t *testing.T) {
	bus := observe.NewEventBus(100)
	reg := tool.NewRegistry(bus)
	// Read is in PrimitiveToolNames but not registered
	tl := newTestREPL(reg, bus)

	input := `{"operations": [{"tool": "Read", "input": {}}]}`
	result, err := tl.Invoke(context.Background(), json.RawMessage(input), nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "unknown tool: Read") {
		t.Errorf("Content = %q, want 'unknown tool: Read'", result.Content)
	}
}

func TestInvoke_EmptyOperations(t *testing.T) {
	bus := observe.NewEventBus(100)
	reg := tool.NewRegistry(bus)
	tl := newTestREPL(reg, bus)

	input := `{"operations": []}`
	_, err := tl.Invoke(context.Background(), json.RawMessage(input), nil)
	if err == nil {
		t.Fatal("expected error for empty operations")
	}
	if !strings.Contains(err.Error(), "at least one operation") {
		t.Errorf("error = %q, want 'at least one operation'", err.Error())
	}
}

func TestInvoke_MultipleOperations(t *testing.T) {
	bus := observe.NewEventBus(100)
	reg := tool.NewRegistry(bus)
	reg.Register(&echoTool{name: "Bash"})
	reg.Register(&echoTool{name: "Glob"})

	tl := newTestREPL(reg, bus)

	input := `{"operations": [
		{"tool": "Bash", "input": {"cmd": "first"}},
		{"tool": "Glob", "input": {"pat": "second"}}
	]}`
	result, err := tl.Invoke(context.Background(), json.RawMessage(input), nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "[0] Bash:") {
		t.Error("missing first operation result")
	}
	if !strings.Contains(result.Content, "[1] Glob:") {
		t.Error("missing second operation result")
	}
	if !strings.Contains(result.Content, "---") {
		t.Error("missing separator between results")
	}
}

func TestToolMetadata(t *testing.T) {
	tl := &Tool{}
	if tl.Name() != "REPL" {
		t.Errorf("Name = %q, want 'REPL'", tl.Name())
	}
	flags := tl.Flags()
	if flags.ReadOnly {
		t.Error("expected ReadOnly = false")
	}
	if !flags.Destructive {
		t.Error("expected Destructive = true")
	}
}
