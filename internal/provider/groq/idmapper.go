package groq

import (
	"strings"
	"sync"
)

// IDMapper maintains a bidirectional mapping between internal UUIDs and
// Groq wire-format tool call IDs (call_xxx).
type IDMapper struct {
	mu             sync.RWMutex
	internalToWire map[string]string
	wireToInternal map[string]string
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
	// Clean up stale reverse mapping if internalID was previously mapped to a different wireID
	if oldWire, ok := m.internalToWire[internalID]; ok && oldWire != wireID {
		delete(m.wireToInternal, oldWire)
	}
	// Clean up stale forward mapping if wireID was previously mapped to a different internalID
	if oldInternal, ok := m.wireToInternal[wireID]; ok && oldInternal != internalID {
		delete(m.internalToWire, oldInternal)
	}
	m.internalToWire[internalID] = wireID
	m.wireToInternal[wireID] = internalID
}

// ToWire returns the wire ID for an internal UUID.
// Returns "" if the internal ID is not registered.
// Callers must handle the empty case (e.g., generate and register a synthetic ID).
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

// syntheticWireID generates a deterministic Groq-format wire ID from an internal UUID.
func syntheticWireID(internalID string) string {
	clean := strings.ReplaceAll(internalID, "-", "")
	if len(clean) > 24 {
		clean = clean[:24]
	}
	return "call_" + clean
}
