package openai

import (
	"github.com/artpar/gogent/internal/observe"
	"strings"
	"sync"
)

// IDMapper maintains a bidirectional mapping between internal UUIDs and
// OpenAI wire-format tool call IDs (call_xxx).
type IDMapper struct {
	mu             sync.RWMutex
	internalToWire map[string]string
	wireToInternal map[string]string
}

// NewIDMapper creates an empty IDMapper.
func NewIDMapper() *IDMapper {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &IDMapper{\n\tinternalToWire:\tmake(map[string]string),\n\twireToInternal:\tmake(ma...")
	return &IDMapper{
		internalToWire: make(map[string]string),
		wireToInternal: make(map[string]string),
	}
}

// RegisterPair records a bidirectional mapping.
// Cleans up stale reverse mappings if an ID is reassigned.
func (m *IDMapper) RegisterPair(internalID, wireID string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.Lock()
	defer m.mu.Unlock()

	if oldWire, ok := m.internalToWire[internalID]; ok && oldWire != wireID {
		observe.GlobalTrace("if: ok && oldWire != wireID")
		delete(m.wireToInternal, oldWire)
	}

	if oldInternal, ok := m.wireToInternal[wireID]; ok && oldInternal != internalID {
		observe.GlobalTrace("if: ok && oldInternal != internalID")
		delete(m.internalToWire, oldInternal)
	}
	m.internalToWire[internalID] = wireID
	m.wireToInternal[wireID] = internalID
}

// ToWire returns the wire ID for an internal UUID.
// Returns "" if the internal ID is not registered.
// Callers must handle the empty case (e.g., generate and register a synthetic ID).
func (m *IDMapper) ToWire(internalID string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	defer m.mu.RUnlock()
	observe.GlobalTrace("return: m.internalToWire[internalID]")
	return m.internalToWire[internalID]
}

// ToInternal returns the internal UUID for a wire ID.
// Returns empty string if no mapping exists.
func (m *IDMapper) ToInternal(wireID string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	defer m.mu.RUnlock()
	observe.GlobalTrace("return: m.wireToInternal[wireID]")
	return m.wireToInternal[wireID]
}

// HasInternal returns true if the internal ID has a registered mapping.
func (m *IDMapper) HasInternal(internalID string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.internalToWire[internalID]
	observe.GlobalTrace("return: ok")
	return ok
}

// syntheticWireID generates a deterministic OpenAI-format wire ID from an internal UUID.
func syntheticWireID(internalID string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	clean := strings.ReplaceAll(internalID, "-", "")
	if len(clean) > 24 {
		observe.GlobalTrace("if: len(clean) > 24")
		clean = clean[:24]
	}
	observe.GlobalTrace("return: \"call_\" + clean")
	return "call_" + clean
}
