package permission

import (
	"context"
)

// Decision is the outcome of a permission check.
type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
	DecisionAsk   Decision = "ask"
)

// PermissionMode controls the default behavior for unmatched tools.
type PermissionMode string

const (
	ModeDefault           PermissionMode = "default"           // ask for unmatched
	ModeAcceptEdits       PermissionMode = "acceptEdits"       // allow file edits in CWD, ask for bash
	ModeBypassPermissions PermissionMode = "bypassPermissions" // allow all
	ModeDontAsk           PermissionMode = "dontAsk"           // deny unmatched silently
)

// RuleSource indicates where a rule came from (priority order: policy > user > project > local > cli > session).
type RuleSource string

const (
	SourcePolicy  RuleSource = "policySettings"
	SourceUser    RuleSource = "userSettings"
	SourceProject RuleSource = "projectSettings"
	SourceLocal   RuleSource = "localSettings"
	SourceCLI     RuleSource = "cliArg"
	SourceSession RuleSource = "session"
)

// Rule describes a single permission rule.
type Rule struct {
	ToolName string     `json:"tool_name"`
	Content  string     `json:"content,omitempty"` // command for Bash, path for file tools, domain:X for WebFetch
	Decision Decision   `json:"decision"`
	Source   RuleSource `json:"source"`
}

// CheckResult is the outcome of a permission check with context.
type CheckResult struct {
	Decision Decision `json:"decision"`
	Rule     *Rule    `json:"rule,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Content  string   `json:"content,omitempty"` // the content string that was checked
}

// Checker evaluates permission for a tool invocation.
// content is the tool-specific extracted string (command for Bash, path for file tools, domain:X for WebFetch).
// Empty content means tool-wide check only.
type Checker interface {
	Check(ctx context.Context, toolName string, content string) CheckResult
	AddSessionRule(rule Rule)
}

func SessionRuleForPrompt(toolName string, result CheckResult, decision Decision) Rule {
	return Rule{
		ToolName: toolName,
		Content:  result.Content,
		Decision: decision,
		Source:   SourceSession,
	}
}
