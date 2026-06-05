package observe

import "testing"

func TestAuditorPermissionDecisionFinal(t *testing.T) {
	a := NewAuditor()
	a.HandleEvent(PermissionDecisionFinal{
		EventHeader: NewEventHeader("PermissionDecisionFinal", "t1", "s1", ""),
		ToolCallID:  "tc-1",
		ToolName:    "Bash",
		Decision:    "allow",
		Rule:        "Bash(git *)",
		Source:      "settings",
		WasExecuted: true,
	})

	trail := a.Trail()
	if len(trail) != 1 {
		t.Fatalf("Trail: got %d, want 1", len(trail))
	}
	if trail[0].Decision != "allow" {
		t.Errorf("Decision: got %q", trail[0].Decision)
	}
	if trail[0].ToolName != "Bash" {
		t.Errorf("ToolName: got %q", trail[0].ToolName)
	}
	if trail[0].RuleMatched != "Bash(git *)" {
		t.Errorf("RuleMatched: got %q", trail[0].RuleMatched)
	}
}

func TestAuditorViolations(t *testing.T) {
	a := NewAuditor()

	// Normal denial — WasExecuted=false (correct behavior)
	a.HandleEvent(PermissionDecisionFinal{
		EventHeader: NewEventHeader("PermissionDecisionFinal", "t1", "s1", ""),
		ToolCallID:  "tc-1",
		ToolName:    "Bash",
		Decision:    "deny",
		WasExecuted: false,
	})

	// Violation — WasExecuted=true (bug: denial ignored)
	a.HandleEvent(PermissionDecisionFinal{
		EventHeader: NewEventHeader("PermissionDecisionFinal", "t1", "s2", ""),
		ToolCallID:  "tc-2",
		ToolName:    "FileWrite",
		Decision:    "deny",
		WasExecuted: true,
	})

	violations := a.Violations()
	if len(violations) != 1 {
		t.Fatalf("Violations: got %d, want 1", len(violations))
	}
	if violations[0].ToolName != "FileWrite" {
		t.Errorf("ToolName: got %q, want FileWrite", violations[0].ToolName)
	}
	if !violations[0].WasExecuted {
		t.Error("expected WasExecuted=true")
	}
}

func TestAuditorPromptedDecision(t *testing.T) {
	a := NewAuditor()

	a.HandleEvent(PermissionDecisionFinal{
		EventHeader:  NewEventHeader("PermissionDecisionFinal", "t1", "s1", ""),
		ToolCallID:   "tc-1",
		ToolName:     "Bash",
		Decision:     "allow",
		UserDecision: "allow",
		WasExecuted:  true,
	})

	trail := a.Trail()
	if len(trail) != 1 {
		t.Fatalf("Trail: got %d, want 1", len(trail))
	}
	if trail[0].UserResponse != "allow" {
		t.Errorf("UserResponse: got %q, want 'allow'", trail[0].UserResponse)
	}
}

func TestAuditorTrailIsCopy(t *testing.T) {
	a := NewAuditor()
	a.HandleEvent(PermissionDecisionFinal{
		EventHeader: NewEventHeader("PermissionDecisionFinal", "t1", "s1", ""),
		ToolCallID:  "tc-1",
		ToolName:    "Bash",
		Decision:    "allow",
		WasExecuted: true,
	})

	trail := a.Trail()
	trail[0].Decision = "modified"

	fresh := a.Trail()
	if fresh[0].Decision == "modified" {
		t.Error("Trail returned a reference, not a copy")
	}
}
