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
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	// Iterate rules in priority order — first match wins
	for i := range rc.rules {
		rule := &rc.rules[i]
		if rule.ToolName != toolName {
			continue
		}

		// Tool-wide rule (no content pattern)
		if rule.Content == "" {
			rc.emitRuleMatched(toolName, rule.Content, string(rule.Source), string(rule.Decision))
			return CheckResult{
				Decision: rule.Decision,
				Rule:     rule,
				Content:  content,
			}
		}

		// Content-specific rule — match only if we have content to compare
		if content != "" && MatchContent(rule.Content, content, rc.workDir) {
			rc.emitRuleMatched(toolName, rule.Content, string(rule.Source), string(rule.Decision))
			return CheckResult{
				Decision: rule.Decision,
				Rule:     rule,
				Content:  content,
			}
		}
	}

	// Safety gate: dangerous paths force DecisionAsk regardless of mode.
	// This check runs BEFORE acceptEdits (matching TS reference: step 4 before step 6).
	// Only applies to file-path content (starts with / or ~), not commands or domains.
	// Skipped for ModeBypassPermissions — user explicitly opted out of all checks.
	if rc.mode != ModeBypassPermissions && content != "" && isFilePath(content) {
		for _, absPath := range resolvePathsForCheck(content, rc.workDir) {
			if IsDangerousPath(absPath, rc.workDir) {
				rc.emitRuleMatched(toolName, "", "dangerous_path", string(DecisionAsk))
				return CheckResult{
					Decision: DecisionAsk,
					Reason:   "dangerous path: " + filepath.Base(absPath),
					Content:  content,
				}
			}
		}
	}

	// No rule matched — check acceptEdits mode before falling through
	if rc.mode == ModeAcceptEdits {
		if decision, ok := rc.acceptEditsDecision(toolName, content); ok {
			rc.emitRuleMatched(toolName, "", "mode_accept_edits", string(decision))
			return CheckResult{
				Decision: decision,
				Reason:   "acceptEdits mode: auto-allow read/write tools in project directory",
				Content:  content,
			}
		}
	}

	// Fall through to mode default
	decision := rc.modeDefault()
	rc.emitRuleMatched(toolName, "", "mode_default", string(decision))
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
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rule.Source = SourceSession
	rc.rules = append(rc.rules, rule)
}

// modeDefault returns the Decision for unmatched tools based on the permission mode.
func (rc *RuleChecker) modeDefault() Decision {
	switch rc.mode {
	case ModeBypassPermissions:
		return DecisionAllow
	case ModeDontAsk:
		return DecisionDeny
	default: // ModeDefault, ModeAcceptEdits
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
	if !acceptEditsTools[toolName] {
		return "", false
	}
	// Read-only tools with no path content: allow (they're safe)
	if content == "" {
		return DecisionAllow, true
	}
	// File tools: allow only if ALL resolved paths (including symlink targets)
	// are within workDir. A symlink pointing outside workDir must not auto-allow.
	workDir := filepath.Clean(rc.workDir)
	for _, absPath := range resolvePathsForCheck(content, rc.workDir) {
		if !strings.HasPrefix(absPath, workDir+string(filepath.Separator)) && absPath != workDir {
			return "", false
		}
	}
	return DecisionAllow, true
}

func (rc *RuleChecker) emitRuleMatched(toolName, pattern, source, decision string) {
	if rc.bus == nil {
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
