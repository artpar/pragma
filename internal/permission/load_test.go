package permission

import (
	"testing"

	"github.com/artpar/pragma/internal/config"
)

func TestParseRuleString(t *testing.T) {
	tests := []struct {
		input    string
		wantTool string
		wantContent string
	}{
		{"Bash", "Bash", ""},
		{"Bash(git push)", "Bash", "git push"},
		{"FileEdit(/.pragma/**)", "FileEdit", "/.pragma/**"},
		{"WebFetch(domain:github.com)", "WebFetch", "domain:github.com"},
		{"Bash(echo \\(hello\\))", "Bash", "echo (hello)"},
		// Edge cases
		{"Bash(", "Bash", ""},                    // unclosed paren
		{"Bash()", "Bash", ""},                    // empty parens
		{"Bash(a\\\\b)", "Bash", `a\b`},            // escaped backslash
		{`Bash(echo \\\(test\\\))`, "Bash", `echo \(test\)`}, // escaped backslash + paren
	}

	for _, tt := range tests {
		rule := ParseRuleString(tt.input, DecisionAllow, SourceUser)
		if rule.ToolName != tt.wantTool {
			t.Errorf("ParseRuleString(%q): ToolName = %q, want %q", tt.input, rule.ToolName, tt.wantTool)
		}
		if rule.Content != tt.wantContent {
			t.Errorf("ParseRuleString(%q): Content = %q, want %q", tt.input, rule.Content, tt.wantContent)
		}
	}
}

func TestRulesFromConfigEntries(t *testing.T) {
	entries := []config.PermissionWithSource{
		{Behavior: "allow", Rule: "FileRead", Source: "userSettings"},
		{Behavior: "deny", Rule: "Bash(rm *)", Source: "projectSettings"},
		{Behavior: "invalid", Rule: "Foo", Source: "userSettings"}, // should be skipped
	}

	rules := RulesFromConfigEntries(entries)
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(rules))
	}
	if rules[0].ToolName != "FileRead" || rules[0].Decision != DecisionAllow {
		t.Errorf("rule[0]: %+v", rules[0])
	}
	if rules[1].ToolName != "Bash" || rules[1].Content != "rm *" || rules[1].Decision != DecisionDeny {
		t.Errorf("rule[1]: %+v", rules[1])
	}
}
