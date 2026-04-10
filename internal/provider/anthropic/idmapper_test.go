package anthropic

import (
	"sync"
	"testing"
)

func TestIDMapperRoundTrip(t *testing.T) {
	m := NewIDMapper()
	m.RegisterPair("internal-uuid-1", "toolu_abc123")

	if got := m.ToWire("internal-uuid-1"); got != "toolu_abc123" {
		t.Errorf("ToWire: got %q, want %q", got, "toolu_abc123")
	}
	if got := m.ToInternal("toolu_abc123"); got != "internal-uuid-1" {
		t.Errorf("ToInternal: got %q, want %q", got, "internal-uuid-1")
	}
}

func TestIDMapperMissingReturnsEmpty(t *testing.T) {
	m := NewIDMapper()

	if got := m.ToWire("nonexistent"); got != "" {
		t.Errorf("ToWire missing: got %q, want empty", got)
	}
	if got := m.ToInternal("nonexistent"); got != "" {
		t.Errorf("ToInternal missing: got %q, want empty", got)
	}
}

func TestIDMapperMultiplePairs(t *testing.T) {
	m := NewIDMapper()
	m.RegisterPair("id-a", "wire-a")
	m.RegisterPair("id-b", "wire-b")
	m.RegisterPair("id-c", "wire-c")

	tests := []struct {
		internal string
		wire     string
	}{
		{"id-a", "wire-a"},
		{"id-b", "wire-b"},
		{"id-c", "wire-c"},
	}

	for _, tt := range tests {
		if got := m.ToWire(tt.internal); got != tt.wire {
			t.Errorf("ToWire(%q): got %q, want %q", tt.internal, got, tt.wire)
		}
		if got := m.ToInternal(tt.wire); got != tt.internal {
			t.Errorf("ToInternal(%q): got %q, want %q", tt.wire, got, tt.internal)
		}
	}
}

func TestIDMapperOverwrite(t *testing.T) {
	m := NewIDMapper()
	m.RegisterPair("id-1", "wire-old")
	m.RegisterPair("id-1", "wire-new")

	if got := m.ToWire("id-1"); got != "wire-new" {
		t.Errorf("ToWire after overwrite: got %q, want %q", got, "wire-new")
	}
	if got := m.ToInternal("wire-new"); got != "id-1" {
		t.Errorf("ToInternal new: got %q, want %q", got, "id-1")
	}
}

func TestIDMapperConcurrent(t *testing.T) {
	m := NewIDMapper()
	var wg sync.WaitGroup
	const n = 100

	// Concurrent writes
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := string(rune('a'+i%26)) + string(rune('0'+i%10))
			m.RegisterPair("int-"+id, "wire-"+id)
		}(i)
	}

	// Concurrent reads while writing
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := string(rune('a'+i%26)) + string(rune('0'+i%10))
			m.ToWire("int-" + id)
			m.ToInternal("wire-" + id)
		}(i)
	}

	wg.Wait()
}

func TestSyntheticWireID(t *testing.T) {
	tests := []struct {
		name     string
		internal string
		wantPfx  string
	}{
		{"uuid with dashes", "550e8400-e29b-41d4-a716-446655440000", "toolu_"},
		{"short id", "abc", "toolu_abc"},
		{"already clean", "abcdef1234567890abcdef12", "toolu_abcdef1234567890abcdef12"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := syntheticWireID(tt.internal)
			if len(got) <= 6 {
				t.Errorf("syntheticWireID(%q): too short: %q", tt.internal, got)
			}
			if got[:6] != "toolu_" {
				t.Errorf("syntheticWireID(%q): missing toolu_ prefix: %q", tt.internal, got)
			}
			// Max 30 chars: "toolu_" (6) + 24 hex chars
			if len(got) > 30 {
				t.Errorf("syntheticWireID(%q): too long (%d): %q", tt.internal, len(got), got)
			}
		})
	}
}

