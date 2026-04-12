package google

import "testing"

func TestIDMapper_RegisterAndLookup(t *testing.T) {
	m := NewIDMapper()
	m.RegisterPair("internal-1", "fc_tool_0")

	if got := m.ToWire("internal-1"); got != "fc_tool_0" {
		t.Errorf("ToWire got %q, want fc_tool_0", got)
	}
	if got := m.ToInternal("fc_tool_0"); got != "internal-1" {
		t.Errorf("ToInternal got %q, want internal-1", got)
	}
	if !m.HasInternal("internal-1") {
		t.Error("HasInternal should be true")
	}
	if m.HasInternal("nonexistent") {
		t.Error("HasInternal should be false for unknown")
	}
}

func TestIDMapper_Reassignment(t *testing.T) {
	m := NewIDMapper()
	m.RegisterPair("internal-1", "fc_a_0")
	m.RegisterPair("internal-1", "fc_b_0")

	if got := m.ToWire("internal-1"); got != "fc_b_0" {
		t.Errorf("ToWire got %q, want fc_b_0", got)
	}
	// Old wire ID should be cleaned up
	if got := m.ToInternal("fc_a_0"); got != "" {
		t.Errorf("old wire ID should be cleaned up, got %q", got)
	}
}

func TestIDMapper_NextWireID(t *testing.T) {
	m := NewIDMapper()
	id0 := m.NextWireID("search")
	id1 := m.NextWireID("search")

	if id0 == id1 {
		t.Errorf("NextWireID should produce unique IDs, got %q twice", id0)
	}
	if id0 != "fc_search_0" {
		t.Errorf("first ID got %q, want fc_search_0", id0)
	}
	if id1 != "fc_search_1" {
		t.Errorf("second ID got %q, want fc_search_1", id1)
	}
}

func TestSyntheticWireID(t *testing.T) {
	got := syntheticWireID("abcdefgh-1234-5678-9abc-def012345678", "tool")
	if got != "fc_tool_abcdefgh" {
		t.Errorf("syntheticWireID got %q, want fc_tool_abcdefgh", got)
	}

	// Short UUID
	got = syntheticWireID("short", "fn")
	if got != "fc_fn_short" {
		t.Errorf("syntheticWireID got %q, want fc_fn_short", got)
	}
}
