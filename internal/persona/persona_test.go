package persona

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPersonaYAMLs(t *testing.T) {
	assertPersonaDir(t, filepath.Join("..", "..", "personas"))
	assertPersonaDir(t, filepath.Join("..", "..", "personas-research-v2"))
}

func TestLoadPersonaWithLLMConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "coder.yaml")
	if err := os.WriteFile(path, []byte(`
id: coder
description: Writes code
llm:
  provider: openai
  model: mlx-community/VibeThinker-3B-4bit
  max_tokens: 4096
  temperature: 0
  thinking: false
  thinking_budget: 1024
  system_prompt: Keep commands fenced.
prompt: |
  Implement the requested change.
`), 0o644); err != nil {
		t.Fatalf("write persona: %v", err)
	}

	def, err := LoadDefinitionFile(path)
	if err != nil {
		t.Fatalf("LoadDefinitionFile: %v", err)
	}
	if def.LLM.Provider != "openai" {
		t.Fatalf("provider = %q, want openai", def.LLM.Provider)
	}
	if def.LLM.Model != "mlx-community/VibeThinker-3B-4bit" {
		t.Fatalf("model = %q", def.LLM.Model)
	}
	if def.LLM.MaxTokens != 4096 {
		t.Fatalf("max tokens = %d, want 4096", def.LLM.MaxTokens)
	}
	if def.LLM.Temperature == nil || *def.LLM.Temperature != 0 {
		t.Fatalf("temperature = %v, want 0", def.LLM.Temperature)
	}
	if def.LLM.Thinking == nil || *def.LLM.Thinking {
		t.Fatalf("thinking = %v, want false", def.LLM.Thinking)
	}
	if def.LLM.ThinkingBudget != 1024 {
		t.Fatalf("thinking budget = %d, want 1024", def.LLM.ThinkingBudget)
	}
	if def.LLM.SystemPrompt != "Keep commands fenced." {
		t.Fatalf("system prompt = %q", def.LLM.SystemPrompt)
	}
}

func assertPersonaDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read persona dir %s: %v", dir, err)
	}
	found := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		found++
		path := filepath.Join(dir, entry.Name())
		id := strings.TrimSuffix(entry.Name(), ".yaml")
		def, err := LoadDefinitionFile(path)
		if err != nil {
			t.Fatalf("load %s persona: %v", path, err)
		}
		if def.ID != id {
			t.Fatalf("persona %s id = %q, want %q", path, def.ID, id)
		}
		if def.Prompt == "" {
			t.Fatalf("persona %s prompt is empty", path)
		}
	}
	if found == 0 {
		t.Fatalf("persona dir %s has no YAML files", dir)
	}
}
