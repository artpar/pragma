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
	gogentDir := filepath.Join(dir, "proj", ".gogent")
	if err := os.MkdirAll(gogentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gogentDir, "AGENT.md"), []byte("test project rule"), 0o644); err != nil {
		t.Fatal(err)
	}

	builder := New(filepath.Join(dir, "proj"), "test-model", nil)
	prompt := builder.Build()

	// 4 static + 1 AGENT.md + 1 env = 6 blocks
	if len(prompt.Blocks) != 6 {
		t.Fatalf("got %d blocks, want 6", len(prompt.Blocks))
	}

	// First 2 blocks NOT cacheable (identity + system rules — avoids Anthropic 4-block cache_control limit)
	// Last 2 static blocks cacheable (task guidance + tone/style)
	for i := 0; i < 2; i++ {
		if prompt.Blocks[i].Cacheable {
			t.Errorf("block %d should NOT be cacheable", i)
		}
	}
	for i := 2; i < 4; i++ {
		if !prompt.Blocks[i].Cacheable {
			t.Errorf("block %d should be cacheable", i)
		}
	}

	// AGENT.md block should not be cacheable
	if prompt.Blocks[4].Cacheable {
		t.Error("AGENT.md block should not be cacheable")
	}
	if !strings.Contains(prompt.Blocks[4].Text, "test project rule") {
		t.Error("AGENT.md block should contain project rule")
	}

	// Env block should not be cacheable
	if prompt.Blocks[5].Cacheable {
		t.Error("env block should not be cacheable")
	}
	if !strings.Contains(prompt.Blocks[5].Text, "Platform:") {
		t.Error("env block should contain platform info")
	}
}

func TestBuild_NoAgentMD(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	builder := New(dir, "test-model", nil)
	prompt := builder.Build()

	// 4 static + 0 AGENT.md + 1 env = 5 blocks
	if len(prompt.Blocks) != 5 {
		t.Fatalf("got %d blocks, want 5 (no AGENT.md)", len(prompt.Blocks))
	}

	// All static blocks present
	if !strings.Contains(prompt.Blocks[0].Text, "gogent") {
		t.Error("identity block should mention gogent")
	}
}

func TestBuild_StaticBlockOrder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	builder := New(dir, "test-model", nil)
	prompt := builder.Build()

	if !strings.Contains(prompt.Blocks[0].Text, "AI coding assistant") {
		t.Error("block 0 should be identity")
	}
	if !strings.Contains(prompt.Blocks[1].Text, "System Rules") {
		t.Error("block 1 should be system rules")
	}
	if !strings.Contains(prompt.Blocks[2].Text, "Task Guidance") {
		t.Error("block 2 should be task guidance")
	}
	if !strings.Contains(prompt.Blocks[3].Text, "Tone & Style") {
		t.Error("block 3 should be tone & style")
	}
}
