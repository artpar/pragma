package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
)

// echoTool is a real tool implementation that echoes its input.
type echoTool struct {
	name        string
	description string
	schema      json.RawMessage
	flags       ToolFlags
}

func (t *echoTool) Name() string                  { return t.name }
func (t *echoTool) Description() string            { return t.description }
func (t *echoTool) InputSchema() json.RawMessage   { return t.schema }
func (t *echoTool) Flags() ToolFlags               { return t.flags }

func (t *echoTool) Invoke(_ context.Context, input json.RawMessage, _ StateSnapshot) (InvokeResult, error) {
	return InvokeResult{Content: string(input)}, nil
}

func (t *echoTool) CheckPerm(_ context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(context.Background(), t.name, "")
}

func newEchoTool(name string, concurrent bool) *echoTool {
	return &echoTool{
		name:        name,
		description: "echoes input",
		schema:      json.RawMessage(`{"type":"object"}`),
		flags:       ToolFlags{Concurrent: concurrent},
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	bus := observe.NewEventBus(100)
	defer bus.Drain()

	reg := NewRegistry(bus)
	tool := newEchoTool("Bash", false)

	if err := reg.Register(tool); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, ok := reg.Get("Bash")
	if !ok {
		t.Fatal("Get: not found")
	}
	if got.Name() != "Bash" {
		t.Errorf("Name: got %q", got.Name())
	}
}

func TestRegistryDuplicateError(t *testing.T) {
	bus := observe.NewEventBus(100)
	defer bus.Drain()

	reg := NewRegistry(bus)
	tool := newEchoTool("Bash", false)

	_ = reg.Register(tool)
	err := reg.Register(tool)
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}
}

func TestRegistryToolDefs(t *testing.T) {
	bus := observe.NewEventBus(100)
	defer bus.Drain()

	reg := NewRegistry(bus)
	_ = reg.Register(newEchoTool("Bash", false))
	_ = reg.Register(newEchoTool("FileRead", true))

	defs := reg.ToolDefs()
	if len(defs) != 2 {
		t.Fatalf("ToolDefs: got %d, want 2", len(defs))
	}

	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names["Bash"] || !names["FileRead"] {
		t.Errorf("missing tools in defs: %v", names)
	}
}

func TestRegistryScoped(t *testing.T) {
	bus := observe.NewEventBus(100)
	defer bus.Drain()

	reg := NewRegistry(bus)
	_ = reg.Register(newEchoTool("Bash", false))
	_ = reg.Register(newEchoTool("FileRead", true))
	_ = reg.Register(newEchoTool("FileWrite", false))

	scoped := reg.Scoped([]string{"Bash", "FileRead"})

	if len(scoped.List()) != 2 {
		t.Errorf("Scoped: got %d tools, want 2", len(scoped.List()))
	}
	if _, ok := scoped.Get("FileWrite"); ok {
		t.Error("Scoped should not contain FileWrite")
	}
}

func TestRegistryGetNotFound(t *testing.T) {
	bus := observe.NewEventBus(100)
	defer bus.Drain()

	reg := NewRegistry(bus)
	_, ok := reg.Get("nonexistent")
	if ok {
		t.Error("expected not found")
	}
}
