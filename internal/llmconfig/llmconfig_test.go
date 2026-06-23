package llmconfig

import "testing"

func TestMergeOverlaysOnlyDefinedFields(t *testing.T) {
	baseTemp := 0.2
	overrideThinking := false
	baseThinking := true
	got := Merge(Config{
		Provider:       "openai",
		Model:          "base-model",
		MaxTokens:      1000,
		Temperature:    &baseTemp,
		Thinking:       &baseThinking,
		ThinkingBudget: 2000,
		SystemPrompt:   "base prompt",
	}, Config{
		Model:    "override-model",
		Thinking: &overrideThinking,
	})

	if got.Provider != "openai" {
		t.Fatalf("provider = %q, want openai", got.Provider)
	}
	if got.Model != "override-model" {
		t.Fatalf("model = %q, want override-model", got.Model)
	}
	if got.MaxTokens != 1000 {
		t.Fatalf("max tokens = %d, want 1000", got.MaxTokens)
	}
	if got.Temperature == nil || *got.Temperature != 0.2 {
		t.Fatalf("temperature = %v, want 0.2", got.Temperature)
	}
	if got.Thinking == nil || *got.Thinking {
		t.Fatalf("thinking = %v, want false", got.Thinking)
	}
	if got.ThinkingBudget != 2000 {
		t.Fatalf("thinking budget = %d, want 2000", got.ThinkingBudget)
	}
	if got.SystemPrompt != "base prompt" {
		t.Fatalf("system prompt = %q, want base prompt", got.SystemPrompt)
	}
}

func TestIsZero(t *testing.T) {
	if !(Config{}).IsZero() {
		t.Fatal("empty config should be zero")
	}
	thinking := false
	if (Config{Thinking: &thinking}).IsZero() {
		t.Fatal("explicit thinking false should be non-zero")
	}
}
