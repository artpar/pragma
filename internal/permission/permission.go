package permission

import (
	"context"
	"github.com/artpar/pragma/internal/observe"
)

// Decision is the outcome of a permission check.
type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
	DecisionAsk   Decision = "ask"
)

type RememberScope string

const (
	RememberNone       RememberScope = "none"
	RememberSession    RememberScope = "session"
	RememberPersistent RememberScope = "persistent"
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
	AddPersistentRule(rule Rule) error
}

type PathChecker interface {
	CheckPath(ctx context.Context, toolName string, path string) CheckResult
}

func CheckPath(ctx context.Context, checker Checker, toolName string, path string) CheckResult {
	observe.TraceCtx(ctx, "permission", "CheckPath", "enter")
	defer observe.TraceCtx(ctx, "permission", "CheckPath", "exit")
	if pathChecker, ok := checker.(PathChecker); ok {
		observe.TraceCtx(ctx, "permission", "CheckPath", "if: ok")
		observe.TraceCtx(ctx, "permission", "CheckPath", "return: pathChecker.CheckPath(ctx, toolName, path)")
		return pathChecker.CheckPath(ctx, toolName, path)
	}
	observe.TraceCtx(ctx, "permission", "CheckPath", "return: checker.Check(ctx, toolName, path)")
	return checker.Check(ctx, toolName, path)
}

type WorkDirScopedChecker interface {
	Checker
	WithWorkDir(workDir string) Checker
}

func CheckerForWorkDir(checker Checker, workDir string) Checker {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if scoped, ok := checker.(WorkDirScopedChecker); ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: scoped.WithWorkDir(workDir)")
		return scoped.WithWorkDir(workDir)
	}
	observe.GlobalTrace("return: checker")
	return checker
}

type SessionRuleResetter interface {
	ClearSessionRules()
}

func SessionRuleForPrompt(toolName string, result CheckResult, decision Decision) Rule {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: Rule{\n\tToolName:\ttoolName,\n\tContent:\tresult.Content,\n\tDecision:\tdecision,\n\tSo...")
	return Rule{
		ToolName: toolName,
		Content:  result.Content,
		Decision: decision,
		Source:   SourceSession,
	}
}
