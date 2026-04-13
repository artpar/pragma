package skill

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/observe"
	skillpkg "github.com/artpar/gogent/internal/skill"
)

type staticState struct{ dir string }

func (s staticState) WorkDir() string { return s.dir }

func newTestTool(t *testing.T, skillDir string) *Tool {
	t.Helper()
	store := app.NewStateStore(app.AppState{CWD: "/tmp"})
	bus := observe.NewEventBus(100)
	loader := skillpkg.NewLoader(skillDir)
	return &Tool{
		Store:  store,
		Bus:    bus,
		Loader: loader,
		// EngineFactory is nil — only inline skills can be tested without engine
	}
}

func TestInvoke_EmptySkillName(t *testing.T) {
	tl := newTestTool(t, t.TempDir())
	input, _ := json.Marshal(SkillInput{Skill: ""})
	result, err := tl.Invoke(context.Background(), input, staticState{"/tmp"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "Skill name is required") {
		t.Errorf("Content = %q, want 'Skill name is required'", result.Content)
	}
}

func TestInvoke_UnknownSkill(t *testing.T) {
	tl := newTestTool(t, t.TempDir())
	input, _ := json.Marshal(SkillInput{Skill: "nonexistent"})
	result, err := tl.Invoke(context.Background(), input, staticState{"/tmp"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "Unknown skill") {
		t.Errorf("Content = %q, want 'Unknown skill'", result.Content)
	}
}

func TestSkillNameNormalization(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/Commit", "commit"},
		{"  review  ", "review"},
		{"/FooBar", "foobar"},
		{"simple", "simple"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(tt.input), "/"))
			if name != tt.want {
				t.Errorf("normalized %q = %q, want %q", tt.input, name, tt.want)
			}
		})
	}
}

func TestInvoke_InlineSkill(t *testing.T) {
	// Create a skill directory with a simple inline skill
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Skills are loaded from .gogent/skills/<name>/SKILL.md
	skillDir := filepath.Join(tmp, ".gogent", "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)

	// Write a simple skill file
	skillContent := `---
name: test-skill
description: A test skill
---
This is the skill content for $ARGUMENTS.`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644)

	tl := newTestTool(t, tmp)
	input, _ := json.Marshal(SkillInput{Skill: "test-skill", Args: "world"})
	result, err := tl.Invoke(context.Background(), input, staticState{tmp})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "skill content") {
		t.Errorf("Content = %q, want to contain skill content", result.Content)
	}
}
