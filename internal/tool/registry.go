package tool

import (
	"fmt"
	"sync"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
)

// Registry holds all registered tools and provides lookup.
type Registry struct {
	tools map[string]Descriptor
	mu    sync.RWMutex
	bus   *observe.EventBus
}

// NewRegistry creates a Registry.
func NewRegistry(bus *observe.EventBus) *Registry {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Registry{\n\ttools:\tmake(map[string]Descriptor),\n\tbus:\tbus,\n}")
	return &Registry{
		tools: make(map[string]Descriptor),
		bus:   bus,
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

// List returns all registered tools.
func (r *Registry) List() []Descriptor {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Descriptor, 0, len(r.tools))
	for _, desc := range r.tools {
		observe.GlobalTrace("range r.tools")
		out = append(out, desc)
	}
	observe.GlobalTrace("return: out")
	return out
}

// ToolDefs returns model.ToolDef for each registered tool (for LLM requests).
func (r *Registry) ToolDefs() []model.ToolDef {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.ToolDef, 0, len(r.tools))
	for _, desc := range r.tools {
		observe.GlobalTrace("range r.tools")
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
