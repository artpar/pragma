package permission

import (
	"context"
	"encoding/json"
)

// Decision is the outcome of a permission check.
type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
	DecisionAsk   Decision = "ask"
)

// Rule describes a single permission rule.
type Rule struct {
	Pattern  string   `json:"pattern"`
	Decision Decision `json:"decision"`
	Source   string   `json:"source"`
}

// CheckResult is the outcome of a permission check with context.
type CheckResult struct {
	Decision Decision `json:"decision"`
	Rule     Rule     `json:"rule"`
	Reason   string   `json:"reason,omitempty"`
}

// Checker evaluates permission for a tool invocation.
type Checker interface {
	Check(ctx context.Context, toolName string, input json.RawMessage) CheckResult
}
