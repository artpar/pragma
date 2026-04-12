package google

import "testing"

func TestLookupModel_Exact(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"gemini-3.1-pro-preview", true},
		{"gemini-3-flash-preview", true},
		{"gemini-3.1-flash-lite-preview", true},
		{"gemini-2.5-pro", true},
		{"gemini-2.5-flash", true},
		{"gemini-2.0-flash", true},
		{"nonexistent-model", false},
		{"gemini-1.5-pro", false},  // removed — no longer in pricing
		{"gemini-1.5-flash", false}, // removed — no longer in pricing
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

func TestLookupModel_Prefix(t *testing.T) {
	info, ok := LookupModel("gemini-2.5")
	if !ok {
		t.Fatal("LookupModel(gemini-2.5) should match by prefix")
	}
	// Alphabetically "gemini-2.5-flash" < "gemini-2.5-pro"
	if info.ID != "gemini-2.5-flash" {
		t.Errorf("prefix match got %q, want gemini-2.5-flash", info.ID)
	}
}

func TestLookupModel_ThinkingSupport(t *testing.T) {
	thinkingModels := []string{
		"gemini-3.1-pro-preview", "gemini-3-flash-preview",
		"gemini-3.1-flash-lite-preview",
		"gemini-2.5-pro", "gemini-2.5-flash",
	}
	for _, id := range thinkingModels {
		info, ok := LookupModel(id)
		if !ok {
			t.Fatalf("model %q not found", id)
		}
		if !info.SupportsThinking {
			t.Errorf("%q should support thinking", id)
		}
	}

	// gemini-2.0-flash does not support thinking
	info, ok := LookupModel("gemini-2.0-flash")
	if !ok {
		t.Fatal("gemini-2.0-flash not found")
	}
	if info.SupportsThinking {
		t.Error("gemini-2.0-flash should not support thinking")
	}
}

func TestLookupModel_Pricing(t *testing.T) {
	// Verify pricing matches docs (https://ai.google.dev/gemini-api/docs/pricing.md.txt)
	tests := []struct {
		id     string
		input  float64
		output float64
	}{
		{"gemini-2.5-pro", 1.25, 10.00},
		{"gemini-2.5-flash", 0.30, 2.50},
		{"gemini-2.0-flash", 0.10, 0.40},
		{"gemini-3.1-pro-preview", 2.00, 12.00},
		{"gemini-3-flash-preview", 0.50, 3.00},
	}
	for _, tt := range tests {
		info, ok := LookupModel(tt.id)
		if !ok {
			t.Fatalf("model %q not found", tt.id)
		}
		if info.Pricing.InputPerMToken != tt.input {
			t.Errorf("%s input price=%.2f, want %.2f", tt.id, info.Pricing.InputPerMToken, tt.input)
		}
		if info.Pricing.OutputPerMToken != tt.output {
			t.Errorf("%s output price=%.2f, want %.2f", tt.id, info.Pricing.OutputPerMToken, tt.output)
		}
	}
}
