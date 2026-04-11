package permission

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/artpar/gogent/internal/config"
)

func TestPersistRuleNewFile(t *testing.T) {
	dir := t.TempDir()
	gogentDir := filepath.Join(dir, ".gogent")
	if err := os.MkdirAll(gogentDir, 0o755); err != nil {
		t.Fatal(err)
	}

	rule := Rule{
		ToolName: "Bash",
		Content:  "git *",
		Decision: DecisionAllow,
		Source:   SourceSession,
	}

	if err := PersistRule(dir, rule); err != nil {
		t.Fatalf("PersistRule: %v", err)
	}

	// Read back
	rules, err := LoadPersistedRules(dir)
	if err != nil {
		t.Fatalf("LoadPersistedRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	if rules[0].ToolName != "Bash" {
		t.Errorf("ToolName: got %q", rules[0].ToolName)
	}
	if rules[0].Content != "git *" {
		t.Errorf("Content: got %q", rules[0].Content)
	}
	if rules[0].Decision != DecisionAllow {
		t.Errorf("Decision: got %s", rules[0].Decision)
	}
}

func TestPersistRuleDedup(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".gogent"), 0o755)

	rule := Rule{ToolName: "Bash", Content: "git *", Decision: DecisionAllow}

	// Persist twice
	PersistRule(dir, rule)
	PersistRule(dir, rule)

	rules, _ := LoadPersistedRules(dir)
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1 (dedup failed)", len(rules))
	}
}

func TestPersistRulePreservesExisting(t *testing.T) {
	dir := t.TempDir()
	gogentDir := filepath.Join(dir, ".gogent")
	os.MkdirAll(gogentDir, 0o755)

	// Write initial settings with an existing permission
	initial := config.Config{
		Permissions: []config.RawPermission{
			{Behavior: "deny", Rule: "Bash(rm -rf *)"},
		},
		Model: "gpt-4o",
	}
	data, _ := json.MarshalIndent(initial, "", "  ")
	os.WriteFile(filepath.Join(gogentDir, "settings.local.json"), data, 0o644)

	// Persist a new rule
	rule := Rule{ToolName: "Bash", Content: "git *", Decision: DecisionAllow}
	if err := PersistRule(dir, rule); err != nil {
		t.Fatalf("PersistRule: %v", err)
	}

	// Verify both rules exist
	rules, _ := LoadPersistedRules(dir)
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(rules))
	}

	// Verify the existing deny rule is preserved
	found := false
	for _, r := range rules {
		if r.ToolName == "Bash" && r.Content == "rm -rf *" && r.Decision == DecisionDeny {
			found = true
		}
	}
	if !found {
		t.Error("existing deny rule was lost (TS bug #9814)")
	}

	// Verify other settings fields are preserved
	var cfg config.Config
	data, _ = os.ReadFile(filepath.Join(gogentDir, "settings.local.json"))
	json.Unmarshal(data, &cfg)
	if cfg.Model != "gpt-4o" {
		t.Errorf("Model field lost: got %q", cfg.Model)
	}
}

func TestPersistRuleNoFile(t *testing.T) {
	dir := t.TempDir()
	// No .gogent dir exists yet

	rule := Rule{ToolName: "Read", Decision: DecisionAllow}
	if err := PersistRule(dir, rule); err != nil {
		t.Fatalf("PersistRule with no existing dir: %v", err)
	}

	rules, _ := LoadPersistedRules(dir)
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
}

func TestRuleToString(t *testing.T) {
	tests := []struct {
		rule Rule
		want string
	}{
		{Rule{ToolName: "Bash"}, "Bash"},
		{Rule{ToolName: "Bash", Content: "git *"}, "Bash(git *)"},
		{Rule{ToolName: "Read", Content: "/home/**"}, "Read(/home/**)"},
	}
	for _, tt := range tests {
		got := RuleToString(tt.rule)
		if got != tt.want {
			t.Errorf("RuleToString(%v) = %q, want %q", tt.rule, got, tt.want)
		}
	}
}
