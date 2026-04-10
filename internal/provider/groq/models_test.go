package groq

import "testing"

func TestLookupModel_Exact(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"llama-3.3-70b-versatile", true},
		{"llama-3.1-8b-instant", true},
		{"meta-llama/llama-4-scout-17b-16e-instruct", true},
		{"openai/gpt-oss-20b", true},
		{"openai/gpt-oss-120b", true},
		{"qwen/qwen3-32b", true},
		{"nonexistent-model", false},
	}
	for _, tt := range tests {
		info, ok := LookupModel(tt.id)
		if ok != tt.want {
			t.Errorf("LookupModel(%q) ok=%v, want %v", tt.id, ok, tt.want)
		}
		if ok && info.ID != tt.id {
			t.Errorf("LookupModel(%q) ID=%q, want %q", tt.id, info.ID, tt.id)
		}
	}
}

func TestLookupModel_PrefixMatch(t *testing.T) {
	info, ok := LookupModel("llama-3.3")
	if !ok {
		t.Fatal("expected prefix match for llama-3.3")
	}
	if info.ID != "llama-3.3-70b-versatile" {
		t.Errorf("got %q, want llama-3.3-70b-versatile", info.ID)
	}
}

func TestLookupModel_Capabilities(t *testing.T) {
	info, ok := LookupModel("meta-llama/llama-4-scout-17b-16e-instruct")
	if !ok {
		t.Fatal("model not found")
	}
	if !info.SupportsVision {
		t.Error("expected vision support for llama-4-scout")
	}
	if !info.SupportsToolUse {
		t.Error("expected tool use support")
	}

	info, ok = LookupModel("openai/gpt-oss-20b")
	if !ok {
		t.Fatal("model not found")
	}
	if !info.SupportsReasoning {
		t.Error("expected reasoning support for gpt-oss-20b")
	}
	if info.ParallelTools {
		t.Error("gpt-oss-20b should not support parallel tools")
	}
}

func TestLookupModel_Pricing(t *testing.T) {
	info, ok := LookupModel("openai/gpt-oss-120b")
	if !ok {
		t.Fatal("model not found")
	}
	if info.Pricing.InputPerMToken != 0.15 {
		t.Errorf("input pricing %v, want 0.15", info.Pricing.InputPerMToken)
	}
	if info.Pricing.CacheReadPerMToken != 0.075 {
		t.Errorf("cache read pricing %v, want 0.075", info.Pricing.CacheReadPerMToken)
	}
}
