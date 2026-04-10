package observe

import "testing"

func TestAuditorPermissionChecked(t *testing.T) {
	a := NewAuditor()
	a.HandleEvent(ToolPermissionChecked{
		EventHeader: NewEventHeader("ToolPermissionChecked", "t1", "s1", ""),
		ToolCallID:  "tc-1",
		ToolName:    "Bash",
		Decision:    "allow",
		Rule:        "allow:Bash(git *)",
		Source:      "settings",
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
}

func TestAuditorViolations(t *testing.T) {
	a := NewAuditor()

	// Normal denial — WasExecuted=false (correct behavior)
	a.HandleEvent(PermissionDenialEnforced{
		EventHeader: NewEventHeader("PermissionDenialEnforced", "t1", "s1", ""),
		ToolCallID:  "tc-1",
		ToolName:    "Bash",
		WasExecuted: false,
	})

	// Violation — WasExecuted=true (bug: denial ignored)
	a.HandleEvent(PermissionDenialEnforced{
		EventHeader: NewEventHeader("PermissionDenialEnforced", "t1", "s2", ""),
		ToolCallID:  "tc-2",
		ToolName:    "FileWrite",
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

func TestAuditorPromptedUpdatesEntry(t *testing.T) {
	a := NewAuditor()

	a.HandleEvent(ToolPermissionChecked{
		EventHeader: NewEventHeader("ToolPermissionChecked", "t1", "s1", ""),
		ToolCallID:  "tc-1",
		ToolName:    "Bash",
		Decision:    "ask",
	})
	a.HandleEvent(ToolPermissionPrompted{
		EventHeader:  NewEventHeader("ToolPermissionPrompted", "t1", "s2", ""),
		ToolCallID:   "tc-1",
		ToolName:     "Bash",
		UserDecision: "allow",
		DurationMs:   2500,
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
	a.HandleEvent(ToolPermissionChecked{
		EventHeader: NewEventHeader("ToolPermissionChecked", "t1", "s1", ""),
		ToolCallID:  "tc-1",
		ToolName:    "Bash",
		Decision:    "allow",
	})

	trail := a.Trail()
	trail[0].Decision = "modified"

	fresh := a.Trail()
	if fresh[0].Decision == "modified" {
		t.Error("Trail returned a reference, not a copy")
	}
}

func TestAuditorOrphanedPromptCreatesEntry(t *testing.T) {
	a := NewAuditor()

	// ToolPermissionPrompted without prior ToolPermissionChecked
	a.HandleEvent(ToolPermissionPrompted{
		EventHeader:  NewEventHeader("ToolPermissionPrompted", "t1", "s1", ""),
		ToolCallID:   "tc-orphan",
		ToolName:     "Bash",
		UserDecision: "allow",
		DurationMs:   1000,
	})

	trail := a.Trail()
	if len(trail) != 1 {
		t.Fatalf("Trail: got %d, want 1", len(trail))
	}
	if trail[0].ToolCallID != "tc-orphan" {
		t.Errorf("ToolCallID: got %q, want tc-orphan", trail[0].ToolCallID)
	}
	if trail[0].Decision != "prompted" {
		t.Errorf("Decision: got %q, want prompted", trail[0].Decision)
	}
	if trail[0].UserResponse != "allow" {
		t.Errorf("UserResponse: got %q, want allow", trail[0].UserResponse)
	}
}
