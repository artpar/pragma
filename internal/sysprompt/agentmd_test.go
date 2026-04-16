package sysprompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripFrontmatter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no frontmatter",
			input: "just content",
			want:  "just content",
		},
		{
			name:  "with frontmatter",
			input: "---\ntitle: test\ntype: user\n---\nactual content",
			want:  "actual content",
		},
		{
			name:  "frontmatter with trailing newline",
			input: "---\nkey: val\n---\n\ncontent after blank line",
			want:  "\ncontent after blank line",
		},
		{
			name:  "unclosed frontmatter",
			input: "---\ntitle: test\nno closing marker",
			want:  "---\ntitle: test\nno closing marker",
		},
		{
			name:  "empty content after frontmatter",
			input: "---\nkey: val\n---\n",
			want:  "",
		},
		{
			name:  "frontmatter only dashes in content",
			input: "---\nmeta: data\n---\nline one\n---\nline two",
			want:  "line one\n---\nline two",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripFrontmatter(tt.input)
			if got != tt.want {
				t.Errorf("stripFrontmatter() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadAgentMD_AllScopes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Create global AGENT.md
	globalDir := filepath.Join(dir, ".pragma")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "AGENT.md"), []byte("global rules"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create project AGENT.md
	projectDir := filepath.Join(dir, "myproject", ".pragma")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "AGENT.md"), []byte("project rules"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create local AGENT.local.md
	if err := os.WriteFile(filepath.Join(projectDir, "AGENT.local.md"), []byte("local rules"), 0o644); err != nil {
		t.Fatal(err)
	}

	sources := LoadAgentMD(filepath.Join(dir, "myproject"), nil)

	if len(sources) != 3 {
		t.Fatalf("got %d sources, want 3", len(sources))
	}

	wantScopes := []string{"global", "project", "local"}
	wantContents := []string{"global rules", "project rules", "local rules"}
	for i, src := range sources {
		if src.Scope != wantScopes[i] {
			t.Errorf("sources[%d].Scope = %q, want %q", i, src.Scope, wantScopes[i])
		}
		if src.Content != wantContents[i] {
			t.Errorf("sources[%d].Content = %q, want %q", i, src.Content, wantContents[i])
		}
	}
}

func TestLoadAgentMD_MissingFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Only project AGENT.md exists
	projectDir := filepath.Join(dir, "proj", ".pragma")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "AGENT.md"), []byte("only project"), 0o644); err != nil {
		t.Fatal(err)
	}

	sources := LoadAgentMD(filepath.Join(dir, "proj"), nil)

	if len(sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(sources))
	}
	if sources[0].Scope != "project" {
		t.Errorf("scope = %q, want project", sources[0].Scope)
	}
}

func TestLoadAgentMD_Empty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	sources := LoadAgentMD(dir, nil)
	if len(sources) != 0 {
		t.Errorf("got %d sources, want 0 for no AGENT.md files", len(sources))
	}
}

func TestLoadAgentMD_FrontmatterStripped(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	projectDir := filepath.Join(dir, "proj", ".pragma")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: test\ntype: project\n---\nactual instructions"
	if err := os.WriteFile(filepath.Join(projectDir, "AGENT.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	sources := LoadAgentMD(filepath.Join(dir, "proj"), nil)
	if len(sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(sources))
	}
	if sources[0].Content != "actual instructions" {
		t.Errorf("Content = %q, want %q", sources[0].Content, "actual instructions")
	}
}

func TestLoadAgentMD_Truncation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	projectDir := filepath.Join(dir, "proj", ".pragma")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a file larger than 25KB
	bigContent := strings.Repeat("x", 30*1024)
	if err := os.WriteFile(filepath.Join(projectDir, "AGENT.md"), []byte(bigContent), 0o644); err != nil {
		t.Fatal(err)
	}

	sources := LoadAgentMD(filepath.Join(dir, "proj"), nil)
	if len(sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(sources))
	}
	if !strings.Contains(sources[0].Content, "[truncated") {
		t.Error("expected truncation marker in oversized content")
	}
	// Content should be maxAgentMDBytes + truncation message, not 30KB
	if len(sources[0].Content) > maxAgentMDBytes+100 {
		t.Errorf("content length = %d, expected ~%d", len(sources[0].Content), maxAgentMDBytes)
	}
}

func TestAgentMDBlock_Format(t *testing.T) {
	sources := []AgentMDSource{
		{Path: "/home/user/.pragma/AGENT.md", Content: "global rule 1", Scope: "global"},
		{Path: "/project/.pragma/AGENT.md", Content: "project rule 1", Scope: "project"},
	}

	block := agentMDBlock(sources)

	if block.Cacheable {
		t.Error("agentMDBlock should not be cacheable")
	}
	if !strings.Contains(block.Text, "IMPORTANT: These instructions OVERRIDE") {
		t.Error("missing override instruction header")
	}
	if !strings.Contains(block.Text, "/home/user/.pragma/AGENT.md (global instructions)") {
		t.Error("missing global path header")
	}
	if !strings.Contains(block.Text, "global rule 1") {
		t.Error("missing global content")
	}
	if !strings.Contains(block.Text, "project rule 1") {
		t.Error("missing project content")
	}
}

func TestAgentMDBlock_Empty(t *testing.T) {
	block := agentMDBlock(nil)
	if block.Text != "" {
		t.Errorf("agentMDBlock(nil) should return empty block, got %q", block.Text)
	}
}
