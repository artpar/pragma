package permission

import (
	"encoding/json"
	"testing"
)

func TestRuleRoundTrip(t *testing.T) {
	rule := Rule{
		ToolName: "Bash",
		Content:  "git *",
		Decision: DecisionAllow,
		Source:   SourceUser,
	}

	data, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Rule
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ToolName != rule.ToolName {
		t.Errorf("ToolName: got %q, want %q", got.ToolName, rule.ToolName)
	}
	if got.Content != rule.Content {
		t.Errorf("Content: got %q, want %q", got.Content, rule.Content)
	}
	if got.Decision != rule.Decision {
		t.Errorf("Decision: got %q, want %q", got.Decision, rule.Decision)
	}
	if got.Source != rule.Source {
		t.Errorf("Source: got %q, want %q", got.Source, rule.Source)
	}
}

func TestDecisionValues(t *testing.T) {
	tests := []struct {
		d    Decision
		want string
	}{
		{DecisionAllow, "allow"},
		{DecisionDeny, "deny"},
		{DecisionAsk, "ask"},
	}
	for _, tt := range tests {
		if string(tt.d) != tt.want {
			t.Errorf("Decision %v: got %q, want %q", tt.d, string(tt.d), tt.want)
		}
	}
}

func TestCheckResultRoundTrip(t *testing.T) {
	cr := CheckResult{
		Decision: DecisionDeny,
		Rule: &Rule{
			ToolName: "Bash",
			Content:  "rm *",
			Decision: DecisionDeny,
			Source:   SourceUser,
		},
		Reason: "destructive command",
	}

	data, err := json.Marshal(cr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got CheckResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Decision != cr.Decision {
		t.Errorf("Decision: got %q", got.Decision)
	}
	if got.Rule == nil {
		t.Fatal("Rule is nil")
	}
	if got.Rule.ToolName != cr.Rule.ToolName {
		t.Errorf("Rule.ToolName: got %q", got.Rule.ToolName)
	}
	if got.Rule.Content != cr.Rule.Content {
		t.Errorf("Rule.Content: got %q", got.Rule.Content)
	}
	if got.Reason != cr.Reason {
		t.Errorf("Reason: got %q", got.Reason)
	}
}
