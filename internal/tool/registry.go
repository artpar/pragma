package tool

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// Registry holds all registered tools and provides lookup.
type Registry struct {
	tools   map[string]Descriptor
	schemas map[string]*jsonschema.Schema
	hidden  map[string]bool
	mu      sync.RWMutex
	bus     *observe.EventBus
}

// NewRegistry creates a Registry.
func NewRegistry(bus *observe.EventBus) *Registry {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Registry{\n\ttools:\tmake(map[string]Descriptor),\n\tbus:\tbus,\n}")
	observe.GlobalTrace("return: &Registry{\n\ttools:\t\tmake(map[string]Descriptor),\n\tschemas:\tmake(map[string]*j...")
	return &Registry{
		tools:   make(map[string]Descriptor),
		schemas: make(map[string]*jsonschema.Schema),
		bus:     bus,
	}
}

// Register adds a tool. Returns error if name is already registered.
func (r *Registry) Register(desc Descriptor) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.Lock()
	defer r.mu.Unlock()
	name := desc.Name()
	if _, exists := r.tools[name]; exists {
		observe.GlobalTrace("if: exists")
		observe.GlobalTrace("return: fmt.Errorf(\"%w: %q\", model.ErrToolAlreadyRegistered, name)")
		return fmt.Errorf("%w: %q", model.ErrToolAlreadyRegistered, name)
	}
	compiler := jsonschema.NewCompiler()

	if err := compiler.AddResource("schema.json", bytes.NewReader(desc.InputSchema())); err == nil {
		observe.GlobalTrace("if: err == nil")
		if compiled, err := compiler.Compile("schema.json"); err == nil {
			observe.GlobalTrace("if: err == nil")
			r.schemas[name] = compiled
		}
	}
	r.tools[name] = desc
	observe.GlobalTrace("return: nil")
	return nil
}

// Unregister removes a tool by name. Used for MCP tool refresh on reconnect.
func (r *Registry) Unregister(name string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
	delete(r.schemas, name)
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Descriptor, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.RLock()
	defer r.mu.RUnlock()
	desc, ok := r.tools[name]
	observe.GlobalTrace("return: desc, ok")
	return desc, ok
}

func (r *Registry) GetSchema(name string) *jsonschema.Schema {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.RLock()
	defer r.mu.RUnlock()
	observe.GlobalTrace("return: r.schemas[name]")
	return r.schemas[name]
}

// SetHidden marks tool names as hidden from ToolDefs() and List() but still
// available via Get(). Used by REPL mode to hide primitive tools from the LLM
// while keeping them callable by the REPL tool.
func (r *Registry) SetHidden(names map[string]bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hidden = names
}

// List returns all registered tools (excluding hidden ones).
func (r *Registry) List() []Descriptor {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Descriptor, 0, len(r.tools))
	for _, desc := range r.tools {
		observe.GlobalTrace("range r.tools")
		if r.hidden[desc.Name()] {
			observe.GlobalTrace("if: r.hidden[desc.Name()]")
			continue
		}
		out = append(out, desc)
	}
	observe.GlobalTrace("return: out")
	return out
}

// ToolDefs returns model.ToolDef for each registered tool (for LLM requests).
// Hidden tools (set via SetHidden) are excluded from the list.
func (r *Registry) ToolDefs() []model.ToolDef {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.ToolDef, 0, len(r.tools))
	for _, desc := range r.tools {
		observe.GlobalTrace("range r.tools")
		if r.hidden[desc.Name()] {
			observe.GlobalTrace("if: r.hidden[desc.Name()]")
			continue
		}
		out = append(out, model.ToolDef{
			Name:        desc.Name(),
			Description: desc.Description(),
			InputSchema: desc.InputSchema(),
		})
	}
	observe.GlobalTrace("return: out")
	return out
}

// Scoped returns a new Registry containing only the named tools.
// Used for sub-agent tool scoping.
func (r *Registry) Scoped(names []string) *Registry {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.RLock()
	defer r.mu.RUnlock()

	scoped := NewRegistry(r.bus)
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		observe.GlobalTrace("range names")
		nameSet[n] = true
	}
	for name, desc := range r.tools {
		observe.GlobalTrace("range r.tools")
		if nameSet[name] {
			observe.GlobalTrace("if: nameSet[name]")
			scoped.tools[name] = desc
		}
	}
	observe.GlobalTrace("return: scoped")
	return scoped
}
