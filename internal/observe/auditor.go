package observe

import (
	"sync"
	"time"
)

// AuditEntry records one permission decision.
type AuditEntry struct {
	Timestamp    time.Time `json:"timestamp"`
	ToolCallID   string    `json:"tool_call_id"`
	ToolName     string    `json:"tool_name"`
	Decision     string    `json:"decision"`
	UserResponse string    `json:"user_response,omitempty"`
	RuleMatched  string    `json:"rule_matched,omitempty"`
	RuleSource   string    `json:"rule_source,omitempty"`
	WasExecuted  bool      `json:"was_executed"`
}

// Auditor is a Subscriber that maintains a permission audit trail.
type Auditor struct {
	mu      sync.Mutex
	entries []AuditEntry
}

func NewAuditor() *Auditor {
	return &Auditor{}
}

func (a *Auditor) HandleEvent(event Event) {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch e := event.(type) {
	case ToolPermissionChecked:
		a.entries = append(a.entries, AuditEntry{
			Timestamp:  e.EventTimestamp(),
			ToolCallID: e.ToolCallID,
			ToolName:   e.ToolName,
			Decision:   e.Decision,
			RuleMatched: e.Rule,
			RuleSource: e.Source,
		})
	case ToolPermissionPrompted:
		// Find the existing entry and update user response
		for i := len(a.entries) - 1; i >= 0; i-- {
			if a.entries[i].ToolCallID == e.ToolCallID {
				a.entries[i].UserResponse = e.UserDecision
				break
			}
		}
	case PermissionDenialEnforced:
		a.entries = append(a.entries, AuditEntry{
			Timestamp:   e.EventTimestamp(),
			ToolCallID:  e.ToolCallID,
			ToolName:    e.ToolName,
			Decision:    "deny",
			WasExecuted: e.WasExecuted,
		})
	}
}

// Trail returns a copy of the full audit trail.
func (a *Auditor) Trail() []AuditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]AuditEntry, len(a.entries))
	copy(out, a.entries)
	return out
}

// Violations returns entries where a denial was ignored (tool executed despite deny).
func (a *Auditor) Violations() []AuditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	var violations []AuditEntry
	for _, e := range a.entries {
		if e.Decision == "deny" && e.WasExecuted {
			violations = append(violations, e)
		}
	}
	return violations
}
