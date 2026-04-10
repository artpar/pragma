package permission

import (
	"encoding/json"
	"testing"
)

func TestRuleRoundTrip(t *testing.T) {
	rule := Rule{
		Pattern:  "Bash(git *)",
		Decision: DecisionAllow,
		Source:   "settings.json",
	}

	data, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Rule
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Pattern != rule.Pattern {
		t.Errorf("Pattern: got %q, want %q", got.Pattern, rule.Pattern)
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
		Rule: Rule{
			Pattern:  "Bash(rm *)",
			Decision: DecisionDeny,
			Source:   "global",
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
	if got.Rule.Pattern != cr.Rule.Pattern {
		t.Errorf("Rule.Pattern: got %q", got.Rule.Pattern)
	}
	if got.Reason != cr.Reason {
		t.Errorf("Reason: got %q", got.Reason)
	}
}
