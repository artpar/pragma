package permission

import (
	"context"
	"testing"
)

func TestRuleCheckerToolWideRule(t *testing.T) {
	rules := []Rule{
		{ToolName: "Bash", Decision: DecisionDeny, Source: SourceUser},
	}
	rc := NewRuleChecker(rules, ModeDefault, "/tmp", nil)

	result := rc.Check(context.Background(), "Bash", "any command")
	if result.Decision != DecisionDeny {
		t.Errorf("expected deny, got %s", result.Decision)
	}
}

func TestRuleCheckerContentSpecificRule(t *testing.T) {
	rules := []Rule{
		{ToolName: "Bash", Content: "git *", Decision: DecisionAllow, Source: SourceUser},
		{ToolName: "Bash", Decision: DecisionDeny, Source: SourceUser},
	}
	rc := NewRuleChecker(rules, ModeDefault, "/tmp", nil)

	// "git push" matches the content-specific allow rule
	result := rc.Check(context.Background(), "Bash", "git push")
	if result.Decision != DecisionAllow {
		t.Errorf("expected allow for 'git push', got %s", result.Decision)
	}

	// "rm -rf" doesn't match the content rule, falls through to the tool-wide deny
	result = rc.Check(context.Background(), "Bash", "rm -rf /")
	if result.Decision != DecisionDeny {
		t.Errorf("expected deny for 'rm -rf', got %s", result.Decision)
	}
}

func TestRuleCheckerPriorityOrder(t *testing.T) {
	rules := []Rule{
		{ToolName: "Bash", Decision: DecisionAllow, Source: SourcePolicy},
		{ToolName: "Bash", Decision: DecisionDeny, Source: SourceUser},
	}
	rc := NewRuleChecker(rules, ModeDefault, "/tmp", nil)

	// Policy (first) wins over user (second)
	result := rc.Check(context.Background(), "Bash", "anything")
	if result.Decision != DecisionAllow {
		t.Errorf("expected allow (policy), got %s", result.Decision)
	}
}

func TestRuleCheckerModeDefault(t *testing.T) {
	// No rules, mode default → ask
	rc := NewRuleChecker(nil, ModeDefault, "/tmp", nil)
	result := rc.Check(context.Background(), "Bash", "ls")
	if result.Decision != DecisionAsk {
		t.Errorf("expected ask, got %s", result.Decision)
	}
}

func TestRuleCheckerBypassMode(t *testing.T) {
	// No rules, bypass mode → allow
	rc := NewRuleChecker(nil, ModeBypassPermissions, "/tmp", nil)
	result := rc.Check(context.Background(), "Bash", "rm -rf /")
	if result.Decision != DecisionAllow {
		t.Errorf("expected allow (bypass), got %s", result.Decision)
	}
}

func TestRuleCheckerDontAskMode(t *testing.T) {
	rc := NewRuleChecker(nil, ModeDontAsk, "/tmp", nil)
	result := rc.Check(context.Background(), "Bash", "ls")
	if result.Decision != DecisionDeny {
		t.Errorf("expected deny (dontAsk), got %s", result.Decision)
	}
}

func TestRuleCheckerAddSessionRule(t *testing.T) {
	rc := NewRuleChecker(nil, ModeDontAsk, "/tmp", nil)

	// Without session rule: denied
	result := rc.Check(context.Background(), "Bash", "ls")
	if result.Decision != DecisionDeny {
		t.Fatalf("expected deny before session rule")
	}

	// Add session allow rule
	rc.AddSessionRule(Rule{ToolName: "Bash", Decision: DecisionAllow})

	// Now allowed
	result = rc.Check(context.Background(), "Bash", "ls")
	if result.Decision != DecisionAllow {
		t.Errorf("expected allow after session rule, got %s", result.Decision)
	}

	rc.ClearSessionRules()
	result = rc.Check(context.Background(), "Bash", "ls")
	if result.Decision != DecisionDeny {
		t.Errorf("expected deny after clearing session rules, got %s", result.Decision)
	}
}

func TestRuleCheckerDomainRule(t *testing.T) {
	rules := []Rule{
		{ToolName: "WebFetch", Content: "domain:github.com", Decision: DecisionAllow, Source: SourceUser},
		{ToolName: "WebFetch", Decision: DecisionDeny, Source: SourceUser},
	}
	rc := NewRuleChecker(rules, ModeDefault, "/tmp", nil)

	result := rc.Check(context.Background(), "WebFetch", "domain:github.com")
	if result.Decision != DecisionAllow {
		t.Errorf("expected allow for github.com, got %s", result.Decision)
	}

	result = rc.Check(context.Background(), "WebFetch", "domain:evil.com")
	if result.Decision != DecisionDeny {
		t.Errorf("expected deny for evil.com, got %s", result.Decision)
	}
}
