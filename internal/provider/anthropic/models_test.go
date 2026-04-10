package anthropic

import "testing"

func TestLookupModelDated(t *testing.T) {
	info, ok := LookupModel("claude-sonnet-4-6-20250514")
	if !ok {
		t.Fatal("expected to find claude-sonnet-4-6-20250514")
	}
	if info.DefaultMaxOutput != 32000 {
		t.Errorf("DefaultMaxOutput: got %d, want 32000", info.DefaultMaxOutput)
	}
	if info.ThinkingType != "adaptive" {
		t.Errorf("ThinkingType: got %q, want %q", info.ThinkingType, "adaptive")
	}
}

func TestLookupModelAlias(t *testing.T) {
	info, ok := LookupModel("claude-opus-4-6")
	if !ok {
		t.Fatal("expected alias to resolve")
	}
	if info.ID != "claude-opus-4-6-20250610" {
		t.Errorf("ID: got %q", info.ID)
	}
	if info.UpperMaxOutput != 128000 {
		t.Errorf("UpperMaxOutput: got %d, want 128000", info.UpperMaxOutput)
	}
}

func TestLookupModelUnknown(t *testing.T) {
	_, ok := LookupModel("gpt-4o")
	if ok {
		t.Error("expected unknown model to return false")
	}
}

func TestLookupModelPricingNonZero(t *testing.T) {
	for id, info := range registry {
		if info.Pricing.InputPerMToken <= 0 {
			t.Errorf("%s: InputPerMToken is zero", id)
		}
		if info.Pricing.OutputPerMToken <= 0 {
			t.Errorf("%s: OutputPerMToken is zero", id)
		}
		if info.Pricing.CacheCreatePerMToken <= 0 {
			t.Errorf("%s: CacheCreatePerMToken is zero", id)
		}
		if info.Pricing.CacheReadPerMToken <= 0 {
			t.Errorf("%s: CacheReadPerMToken is zero", id)
		}
	}
}

func TestLookupModelPrefixDeterministic(t *testing.T) {
	// "claude-opus-4" matches multiple registry keys (4-5 and 4-6).
	// Run 100 times to verify deterministic result from sorted candidates.
	var firstID string
	for i := 0; i < 100; i++ {
		info, ok := LookupModel("claude-opus-4")
		if !ok {
			t.Fatal("expected prefix match")
		}
		if firstID == "" {
			firstID = info.ID
		} else if info.ID != firstID {
			t.Fatalf("non-deterministic: got %q on iteration %d, previously got %q", info.ID, i, firstID)
		}
	}
}

func TestLookupModelThinkingEnabled(t *testing.T) {
	info, ok := LookupModel("claude-sonnet-4-20250514")
	if !ok {
		t.Fatal("expected to find model")
	}
	if info.ThinkingType != "enabled" {
		t.Errorf("ThinkingType: got %q, want %q", info.ThinkingType, "enabled")
	}
}
