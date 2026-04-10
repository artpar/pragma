package anthropic

import (
	"strings"
	"sync"
)

// IDMapper maintains bidirectional mapping between internal UUIDs and
// Anthropic wire IDs (toolu_*). Thread-safe for concurrent use.
//
// The agentic loop, orchestrator, and permission system all reference tool
// calls by internal UUID. Only the provider adapter knows about wire IDs.
type IDMapper struct {
	mu             sync.RWMutex
	internalToWire map[string]string
	wireToInternal map[string]string
}

// NewIDMapper creates an empty ID mapper.
func NewIDMapper() *IDMapper {
	return &IDMapper{
		internalToWire: make(map[string]string),
		wireToInternal: make(map[string]string),
	}
}

// RegisterPair records a bidirectional mapping between an internal UUID and
// a wire ID. Overwrites any existing mapping for either ID.
func (m *IDMapper) RegisterPair(internalID, wireID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Clean up stale reverse mapping when internal ID is reassigned
	if oldWire, ok := m.internalToWire[internalID]; ok && oldWire != wireID {
		delete(m.wireToInternal, oldWire)
	}
	// Clean up stale forward mapping when wire ID is reassigned
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
// Returns "" if the wire ID is not registered.
func (m *IDMapper) ToInternal(wireID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.wireToInternal[wireID]
}

// syntheticWireID generates a deterministic Anthropic-format wire ID from
// an internal UUID. Used for history messages where the original wire ID
// is not stored — Anthropic only requires that tool_use and tool_result
// IDs match within the same request.
func syntheticWireID(internalID string) string {
	clean := strings.ReplaceAll(internalID, "-", "")
	if len(clean) > 24 {
		clean = clean[:24]
	}
	return "toolu_" + clean
}

