package google

import (
	"fmt"
	"sync"
)

// IDMapper maintains a bidirectional mapping between internal UUIDs and
// synthetic Google wire-format tool call IDs.
// Google's API does not use tool call IDs, so we synthesize positional ones.
type IDMapper struct {
	mu             sync.RWMutex
	internalToWire map[string]string
	wireToInternal map[string]string
	counter        int
}

// NewIDMapper creates an empty IDMapper.
func NewIDMapper() *IDMapper {
	return &IDMapper{
		internalToWire: make(map[string]string),
		wireToInternal: make(map[string]string),
	}
}

// RegisterPair records a bidirectional mapping.
// Cleans up stale reverse mappings if an ID is reassigned.
func (m *IDMapper) RegisterPair(internalID, wireID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if oldWire, ok := m.internalToWire[internalID]; ok && oldWire != wireID {
		delete(m.wireToInternal, oldWire)
	}

	if oldInternal, ok := m.wireToInternal[wireID]; ok && oldInternal != internalID {
		delete(m.internalToWire, oldInternal)
	}
	m.internalToWire[internalID] = wireID
	m.wireToInternal[wireID] = internalID
}

// ToWire returns the wire ID for an internal UUID.
// Returns "" if the internal ID is not registered.
func (m *IDMapper) ToWire(internalID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.internalToWire[internalID]
}

// ToInternal returns the internal UUID for a wire ID.
// Returns empty string if no mapping exists.
func (m *IDMapper) ToInternal(wireID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.wireToInternal[wireID]
}

// HasInternal returns true if the internal ID has a registered mapping.
func (m *IDMapper) HasInternal(internalID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.internalToWire[internalID]
	return ok
}

// NextWireID generates the next synthetic wire ID for a function call.
func (m *IDMapper) NextWireID(name string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := fmt.Sprintf("fc_%s_%d", name, m.counter)
	m.counter++
	return id
}

// syntheticWireID generates a deterministic wire ID from an internal UUID and tool name.
func syntheticWireID(internalID, name string) string {
	// Use first 8 chars of UUID + name for uniqueness
	short := internalID
	if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("fc_%s_%s", name, short)
}
