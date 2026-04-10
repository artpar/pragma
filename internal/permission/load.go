package permission

import (
	"strings"

	"github.com/artpar/gogent/internal/config"
)

// RulesFromConfigEntries converts raw config permission entries to structured Rules.
// Entries are returned in the same order (caller is responsible for priority ordering).
func RulesFromConfigEntries(entries []config.PermissionWithSource) []Rule {
	rules := make([]Rule, 0, len(entries))
	for _, e := range entries {
		decision := parseDecision(e.Behavior)
		if decision == "" {
			continue // skip invalid behaviors
		}
		source := parseSource(e.Source)
		rule := ParseRuleString(e.Rule, decision, source)
		rules = append(rules, rule)
	}
	return rules
}

// ParseRuleString parses a rule string like "ToolName" or "ToolName(content)" into a Rule.
// Supports escaped parentheses: \( and \) for literal parens in content.
func ParseRuleString(ruleStr string, decision Decision, source RuleSource) Rule {
	toolName, content := splitRuleString(ruleStr)
	return Rule{
		ToolName: toolName,
		Content:  content,
		Decision: decision,
		Source:   source,
	}
}

// splitRuleString splits "ToolName(content)" into ("ToolName", "content").
// If there's no parenthesized content, returns ("ToolName", "").
func splitRuleString(s string) (string, string) {
	idx := strings.IndexByte(s, '(')
	if idx < 0 {
		return s, ""
	}
	toolName := s[:idx]

	// Find matching close paren, respecting escapes
	rest := s[idx+1:]
	if len(rest) > 0 && rest[len(rest)-1] == ')' {
		content := rest[:len(rest)-1]
		// Unescape: \\ first (to avoid interfering with \( and \))
		// Use a placeholder to prevent double-replacement
		content = strings.ReplaceAll(content, `\\`, "\x00")
		content = strings.ReplaceAll(content, `\(`, "(")
		content = strings.ReplaceAll(content, `\)`, ")")
		content = strings.ReplaceAll(content, "\x00", `\`)
		return toolName, content
	}

	// No closing paren — treat as tool-wide rule using the part before '('
	return toolName, ""
}

func parseDecision(behavior string) Decision {
	switch strings.ToLower(behavior) {
	case "allow":
		return DecisionAllow
	case "deny":
		return DecisionDeny
	case "ask":
		return DecisionAsk
	default:
		return ""
	}
}

func parseSource(source string) RuleSource {
	switch source {
	case "policySettings":
		return SourcePolicy
	case "userSettings":
		return SourceUser
	case "projectSettings":
		return SourceProject
	case "localSettings":
		return SourceLocal
	case "cliArg":
		return SourceCLI
	case "session":
		return SourceSession
	default:
		return SourceUser
	}
}
