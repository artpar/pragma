package sysprompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuild_FullPrompt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Create an AGENT.md
	gogentDir := filepath.Join(dir, "proj", ".pragma")
	if err := os.MkdirAll(gogentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gogentDir, "AGENT.md"), []byte("test project rule"), 0o644); err != nil {
		t.Fatal(err)
	}

	builder := New(filepath.Join(dir, "proj"), "test-model", nil)
	prompt := builder.Build()

	// 7 static + 1 AGENT.md + 1 env = 9 blocks
	if len(prompt.Blocks) != 9 {
		t.Fatalf("got %d blocks, want 9", len(prompt.Blocks))
	}

	// First 2 blocks NOT cacheable (identity + system rules — avoids Anthropic 4-block cache_control limit)
	// Blocks 2-6 cacheable (using-tools, doing-tasks, actions-with-care, tone/style, output-efficiency)
	for i := 0; i < 2; i++ {
		if prompt.Blocks[i].Cacheable {
			t.Errorf("block %d should NOT be cacheable", i)
		}
	}
	for i := 2; i < 7; i++ {
		if !prompt.Blocks[i].Cacheable {
			t.Errorf("block %d should be cacheable", i)
		}
	}

	// AGENT.md block should not be cacheable
	if prompt.Blocks[7].Cacheable {
		t.Error("AGENT.md block should not be cacheable")
	}
	if !strings.Contains(prompt.Blocks[7].Text, "test project rule") {
		t.Error("AGENT.md block should contain project rule")
	}

	// Env block should not be cacheable
	if prompt.Blocks[8].Cacheable {
		t.Error("env block should not be cacheable")
	}
	if !strings.Contains(prompt.Blocks[8].Text, "Platform:") {
		t.Error("env block should contain platform info")
	}
}

func TestBuild_NoAgentMD(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	builder := New(dir, "test-model", nil)
	prompt := builder.Build()

	// 7 static + 0 AGENT.md + 1 env = 8 blocks
	if len(prompt.Blocks) != 8 {
		t.Fatalf("got %d blocks, want 8 (no AGENT.md)", len(prompt.Blocks))
	}

	// All static blocks present
	if !strings.Contains(prompt.Blocks[0].Text, "pragma") {
		t.Error("identity block should mention gogent")
	}
}

func TestBuild_StaticBlockOrder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	builder := New(dir, "test-model", nil)
	prompt := builder.Build()

	checks := []struct {
		idx      int
		contains string
		name     string
	}{
		{0, "AI coding assistant", "identity"},
		{1, "System", "system rules"},
		{2, "Using your tools", "using tools"},
		{3, "Doing tasks", "doing tasks"},
		{4, "Executing actions with care", "actions with care"},
		{5, "Tone and style", "tone & style"},
		{6, "Output efficiency", "output efficiency"},
	}

	for _, c := range checks {
		if !strings.Contains(prompt.Blocks[c.idx].Text, c.contains) {
			t.Errorf("block %d should be %s (expected %q)", c.idx, c.name, c.contains)
		}
	}
}
