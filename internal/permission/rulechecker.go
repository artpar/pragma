package permission

import (
	"context"
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

	// No rule matched — fall through to mode default
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
