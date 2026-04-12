package permission

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/artpar/gogent/internal/observe"
)

// RuleChecker is the production implementation of Checker.
// It evaluates rules in priority order (policy first, session last)
// and falls through to mode defaults when no rule matches.
type RuleChecker struct {
	mu      sync.RWMutex
	rules   []Rule // ordered by source priority
	mode    PermissionMode
	workDir string
	bus     *observe.EventBus
}

// NewRuleChecker creates a RuleChecker with the given rules and mode.
// Rules must be pre-sorted by source priority (policy first).
func NewRuleChecker(rules []Rule, mode PermissionMode, workDir string, bus *observe.EventBus) *RuleChecker {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &RuleChecker{\n\trules:\t\trules,\n\tmode:\t\tmode,\n\tworkDir:\tworkDir,\n\tbus:\t\tbus,\n}")
	observe.GlobalTrace("return: &RuleChecker{\n\trules:\t\trules,\n\tmode:\t\tmode,\n\tworkDir:\tworkDir,\n\tbus:\t\tbus,\n}")
	observe.GlobalTrace("return: &RuleChecker{\n\trules:\t\trules,\n\tmode:\t\tmode,\n\tworkDir:\tworkDir,\n\tbus:\t\tbus,\n}")
	return &RuleChecker{
		rules:   rules,
		mode:    mode,
		workDir: workDir,
		bus:     bus,
	}
}

// Check evaluates permission for a tool invocation.
// content is the tool-specific extracted string (command for Bash, path for file tools, domain:X for WebFetch).
func (rc *RuleChecker) Check(ctx context.Context, toolName string, content string) CheckResult {
	observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "enter")
	defer observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "exit")
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	for i := range rc.rules {
		observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "range rc.rules")
		rule := &rc.rules[i]
		if rule.ToolName != toolName {
			observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "if: rule.ToolName != toolName")
			continue
		}

		if rule.Content == "" {
			observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "if: rule.Content == \"\"")
			rc.emitRuleMatched(toolName, rule.Content, string(rule.Source), string(rule.Decision))
			observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "return: CheckResult{\n\tDecision:\trule.Decision,\n\tRule:\t\trule,\n\tContent:\tcontent,\n}")
			observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "return: CheckResult{\n\tDecision:\trule.Decision,\n\tRule:\t\trule,\n\tContent:\tcontent,\n}")
			observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "return: CheckResult{\n\tDecision:\trule.Decision,\n\tRule:\t\trule,\n\tContent:\tcontent,\n}")
			return CheckResult{
				Decision: rule.Decision,
				Rule:     rule,
				Content:  content,
			}
		}

		if content != "" && MatchContent(rule.Content, content, rc.workDir) {
			observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "if: content != \"\" && MatchContent(rule.Content, content, rc.workDir)")
			rc.emitRuleMatched(toolName, rule.Content, string(rule.Source), string(rule.Decision))
			observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "return: CheckResult{\n\tDecision:\trule.Decision,\n\tRule:\t\trule,\n\tContent:\tcontent,\n}")
			observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "return: CheckResult{\n\tDecision:\trule.Decision,\n\tRule:\t\trule,\n\tContent:\tcontent,\n}")
			observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "return: CheckResult{\n\tDecision:\trule.Decision,\n\tRule:\t\trule,\n\tContent:\tcontent,\n}")
			return CheckResult{
				Decision: rule.Decision,
				Rule:     rule,
				Content:  content,
			}
		}
	}

	if rc.mode != ModeBypassPermissions && content != "" && isFilePath(content) {
		observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "if: rc.mode != ModeBypassPermissions && content != \"\" && isFilePath(content)")
		for _, absPath := range resolvePathsForCheck(content, rc.workDir) {
			observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "range resolvePathsForCheck(content, rc.workDir)")
			if IsDangerousPath(absPath, rc.workDir) {
				observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "if: IsDangerousPath(absPath, rc.workDir)")
				rc.emitRuleMatched(toolName, "", "dangerous_path", string(DecisionAsk))
				observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "return: CheckResult{\n\tDecision:\tDecisionAsk,\n\tReason:\t\t\"dangerous path: \" + filepath....")
				observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "return: CheckResult{\n\tDecision:\tDecisionAsk,\n\tReason:\t\t\"dangerous path: \" + filepath....")
				observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "return: CheckResult{\n\tDecision:\tDecisionAsk,\n\tReason:\t\t\"dangerous path: \" + filepath....")
				return CheckResult{
					Decision: DecisionAsk,
					Reason:   "dangerous path: " + filepath.Base(absPath),
					Content:  content,
				}
			}
		}
	}

	if rc.mode == ModeAcceptEdits {
		observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "if: rc.mode == ModeAcceptEdits")
		if decision, ok := rc.acceptEditsDecision(toolName, content); ok {
			observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "if: ok")
			rc.emitRuleMatched(toolName, "", "mode_accept_edits", string(decision))
			observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "return: CheckResult{\n\tDecision:\tdecision,\n\tReason:\t\t\"acceptEdits mode: auto-allow rea...")
			observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "return: CheckResult{\n\tDecision:\tdecision,\n\tReason:\t\t\"acceptEdits mode: auto-allow rea...")
			observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "return: CheckResult{\n\tDecision:\tdecision,\n\tReason:\t\t\"acceptEdits mode: auto-allow rea...")
			return CheckResult{
				Decision: decision,
				Reason:   "acceptEdits mode: auto-allow read/write tools in project directory",
				Content:  content,
			}
		}
	}

	decision := rc.modeDefault()
	rc.emitRuleMatched(toolName, "", "mode_default", string(decision))
	observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "return: CheckResult{\n\tDecision:\tdecision,\n\tReason:\t\t\"no matching rule, mode default: ...")
	observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "return: CheckResult{\n\tDecision:\tdecision,\n\tReason:\t\t\"no matching rule, mode default: ...")
	observe.TraceCtx(ctx, "permission", "RuleChecker.Check", "return: CheckResult{\n\tDecision:\tdecision,\n\tReason:\t\t\"no matching rule, mode default: ...")
	return CheckResult{
		Decision: decision,
		Reason:   "no matching rule, mode default: " + string(rc.mode),
		Content:  content,
	}
}

// AddSessionRule adds a rule for the current session (e.g., user granted "always allow" in TUI).
// Session rules are appended at the end (lowest priority among existing rules,
// but checked after all other rules have been tried).
func (rc *RuleChecker) AddSessionRule(rule Rule) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rule.Source = SourceSession
	rc.rules = append(rc.rules, rule)
}

// modeDefault returns the Decision for unmatched tools based on the permission mode.
func (rc *RuleChecker) modeDefault() Decision {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch rc.mode {
	case ModeBypassPermissions:
		observe.GlobalTrace("case: ModeBypassPermissions")
		return DecisionAllow
	case ModeDontAsk:
		observe.GlobalTrace("case: ModeDontAsk")
		return DecisionDeny
	default:
		observe.GlobalTrace("default")
		return DecisionAsk
	}
}

// acceptEditsTools are the tools that ModeAcceptEdits auto-allows when the content
// (file path) is within the project working directory. Bash is always "ask".
var acceptEditsTools = map[string]bool{
	"Read":         true,
	"Edit":         true,
	"Write":        true,
	"Glob":         true,
	"Grep":         true,
	"NotebookEdit": true,
}

// acceptEditsDecision checks if acceptEdits mode should auto-allow this tool.
// Returns (decision, true) if a decision was made, or (_, false) to fall through.
func (rc *RuleChecker) acceptEditsDecision(toolName, content string) (Decision, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !acceptEditsTools[toolName] {
		observe.GlobalTrace("if: !acceptEditsTools[toolName]")
		observe.GlobalTrace("return: \"\", false")
		observe.GlobalTrace("return: \"\", false")
		observe.GlobalTrace("return: \"\", false")
		return "", false
	}

	if content == "" {
		observe.GlobalTrace("if: content == \"\"")
		observe.GlobalTrace("return: DecisionAllow, true")
		observe.GlobalTrace("return: DecisionAllow, true")
		observe.GlobalTrace("return: DecisionAllow, true")
		return DecisionAllow, true
	}

	workDir := filepath.Clean(rc.workDir)
	for _, absPath := range resolvePathsForCheck(content, rc.workDir) {
		observe.GlobalTrace("range resolvePathsForCheck(content, rc.workDir)")
		if !strings.HasPrefix(absPath, workDir+string(filepath.Separator)) && absPath != workDir {
			observe.GlobalTrace("if: !strings.HasPrefix(absPath, workDir+string(filepath.Separator)) && absPath !=...")
			observe.GlobalTrace("return: \"\", false")
			observe.GlobalTrace("return: \"\", false")
			observe.GlobalTrace("return: \"\", false")
			return "", false
		}
	}
	observe.GlobalTrace("return: DecisionAllow, true")
	observe.GlobalTrace("return: DecisionAllow, true")
	observe.GlobalTrace("return: DecisionAllow, true")
	return DecisionAllow, true
}

func (rc *RuleChecker) emitRuleMatched(toolName, pattern, source, decision string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if rc.bus == nil {
		observe.GlobalTrace("if: rc.bus == nil")
		return
	}
	rc.bus.Emit(observe.PermissionRuleMatched{
		EventHeader: observe.NewEventHeader("PermissionRuleMatched", "", "", ""),
		ToolName:    toolName,
		Pattern:     pattern,
		Source:      source,
		Decision:    decision,
	})
}
