package groq

import "testing"

func TestIDMapper_RegisterAndLookup(t *testing.T) {
	m := NewIDMapper()
	m.RegisterPair("int-123", "call_abc")

	if got := m.ToWire("int-123"); got != "call_abc" {
		t.Errorf("ToWire got %q, want call_abc", got)
	}
	if got := m.ToInternal("call_abc"); got != "int-123" {
		t.Errorf("ToInternal got %q, want int-123", got)
	}
}

func TestIDMapper_SyntheticFallback(t *testing.T) {
	m := NewIDMapper()
	wireID := m.ToWire("550e8400-e29b-41d4-a716-446655440000")
	if wireID[:5] != "call_" {
		t.Errorf("synthetic wire ID should start with call_, got %q", wireID)
	}
	if len(wireID) != 29 { // "call_" + 24 chars
		t.Errorf("synthetic wire ID length %d, want 29", len(wireID))
	}
}

func TestIDMapper_UnknownInternal(t *testing.T) {
	m := NewIDMapper()
	if got := m.ToInternal("call_unknown"); got != "" {
		t.Errorf("expected empty string for unknown wire ID, got %q", got)
	}
}

func TestIDMapper_HasInternal(t *testing.T) {
	m := NewIDMapper()
	if m.HasInternal("nope") {
		t.Error("expected false for unregistered ID")
	}
	m.RegisterPair("int-1", "call_1")
	if !m.HasInternal("int-1") {
		t.Error("expected true for registered ID")
	}
}

func TestIDMapper_StaleCleanup(t *testing.T) {
	m := NewIDMapper()
	m.RegisterPair("int-1", "call_old")
	m.RegisterPair("int-1", "call_new")
	// Old wire ID should be cleaned up
	if m.ToInternal("call_old") != "" {
		t.Error("stale wire ID should be removed")
	}
	if m.ToInternal("call_new") != "int-1" {
		t.Error("new wire ID should map to int-1")
	}
}

func TestSyntheticWireID_Format(t *testing.T) {
	id := syntheticWireID("550e8400-e29b-41d4-a716-446655440000")
	// UUID without dashes = "550e8400e29b41d4a716446655440000" (32 chars), truncated to 24
	if id != "call_550e8400e29b41d4a7164466" {
		t.Errorf("got %q", id)
	}
}
