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
	index   map[string]int // toolCallID → entries index for O(1) lookup
}

func NewAuditor() *Auditor {
	return &Auditor{
		index: make(map[string]int),
	}
}

func (a *Auditor) HandleEvent(event Event) {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch e := event.(type) {
	case PermissionDecisionFinal:
		a.entries = append(a.entries, AuditEntry{
			Timestamp:    e.EventTimestamp(),
			ToolCallID:   e.ToolCallID,
			ToolName:     e.ToolName,
			Decision:     e.Decision,
			UserResponse: e.UserDecision,
			RuleMatched:  e.Rule,
			RuleSource:   e.Source,
			WasExecuted:  e.WasExecuted,
		})
		a.index[e.ToolCallID] = len(a.entries) - 1
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
