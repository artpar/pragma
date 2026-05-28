package sysprompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/model"
)

func TestBuild_FullPrompt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Create an AGENT.md
	pragmaDir := filepath.Join(dir, "proj", ".pragma")
	if err := os.MkdirAll(pragmaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pragmaDir, "AGENT.md"), []byte("test project rule"), 0o644); err != nil {
		t.Fatal(err)
	}

	builder := New(filepath.Join(dir, "proj"), "test-model", nil)
	prompt := builder.Build()

	if len(prompt.Blocks) != 3 {
		t.Fatalf("got %d blocks, want 3", len(prompt.Blocks))
	}

	if prompt.Blocks[0].Cacheable {
		t.Error("Codex prompt block should not be cacheable")
	}

	// AGENT.md block should not be cacheable
	if prompt.Blocks[1].Cacheable {
		t.Error("AGENT.md block should not be cacheable")
	}
	if !strings.Contains(prompt.Blocks[1].Text, "test project rule") {
		t.Error("AGENT.md block should contain project rule")
	}

	// Env block should not be cacheable
	if prompt.Blocks[2].Cacheable {
		t.Error("env block should not be cacheable")
	}
	if !strings.Contains(prompt.Blocks[2].Text, "Platform:") {
		t.Error("env block should contain platform info")
	}
}

func TestBuild_NoAgentMD(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	builder := New(dir, "test-model", nil)
	prompt := builder.Build()

	if len(prompt.Blocks) != 2 {
		t.Fatalf("got %d blocks, want 2 (no AGENT.md)", len(prompt.Blocks))
	}

	if prompt.Blocks[0].Text != codexDefaultPrompt {
		t.Error("static prompt should match Codex default prompt")
	}
}

func TestBuild_StaticPromptIsCodexDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	builder := New(dir, "test-model", nil)
	prompt := builder.Build()

	if prompt.Blocks[0].Text != codexDefaultPrompt {
		t.Fatal("static prompt does not match embedded Codex default prompt")
	}
}

func TestBuild_StaticPromptDoesNotAdvertiseSpecificToolNames(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	builder := New(dir, "test-model", nil)
	prompt := builder.Build()

	text := systemTextForTest(prompt)
	for _, name := range []string{
		"Bash",
		"Read",
		"Edit",
		"Write",
		"Glob",
		"Grep",
		"Agent",
		"LifecycleRun",
		"TaskCreate",
		"AskUserQuestion",
		"Skill",
	} {
		if strings.Contains(text, name) {
			t.Fatalf("static prompt leaked tool name %q", name)
		}
	}
}

func systemTextForTest(prompt model.SystemPrompt) string {
	var b strings.Builder
	for _, block := range prompt.Blocks {
		b.WriteString(block.Text)
		b.WriteByte('\n')
	}
	return b.String()
}
