package permission

import (
	"strings"

	"github.com/artpar/gogent/internal/config"
	"github.com/artpar/gogent/internal/observe"
)

// RulesFromConfigEntries converts raw config permission entries to structured Rules.
// Entries are returned in the same order (caller is responsible for priority ordering).
func RulesFromConfigEntries(entries []config.PermissionWithSource) []Rule {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	rules := make([]Rule, 0, len(entries))
	for _, e := range entries {
		observe.GlobalTrace("range entries")
		decision := parseDecision(e.Behavior)
		if decision == "" {
			observe.GlobalTrace("if: decision == \"\"")
			continue
		}
		source := parseSource(e.Source)
		rule := ParseRuleString(e.Rule, decision, source)
		rules = append(rules, rule)
	}
	observe.GlobalTrace("return: rules")
	observe.GlobalTrace("return: rules")
	observe.GlobalTrace("return: rules")
	return rules
}

// ParseRuleString parses a rule string like "ToolName" or "ToolName(content)" into a Rule.
// Supports escaped parentheses: \( and \) for literal parens in content.
func ParseRuleString(ruleStr string, decision Decision, source RuleSource) Rule {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	toolName, content := splitRuleString(ruleStr)
	observe.GlobalTrace("return: Rule{\n\tToolName:\ttoolName,\n\tContent:\tcontent,\n\tDecision:\tdecision,\n\tSource:\t\t...")
	observe.GlobalTrace("return: Rule{\n\tToolName:\ttoolName,\n\tContent:\tcontent,\n\tDecision:\tdecision,\n\tSource:\t\t...")
	observe.GlobalTrace("return: Rule{\n\tToolName:\ttoolName,\n\tContent:\tcontent,\n\tDecision:\tdecision,\n\tSource:\t\t...")
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	idx := strings.IndexByte(s, '(')
	if idx < 0 {
		observe.GlobalTrace("if: idx < 0")
		observe.GlobalTrace("return: s, \"\"")
		observe.GlobalTrace("return: s, \"\"")
		observe.GlobalTrace("return: s, \"\"")
		return s, ""
	}
	toolName := s[:idx]

	rest := s[idx+1:]
	if len(rest) > 0 && rest[len(rest)-1] == ')' {
		observe.GlobalTrace("if: len(rest) > 0 && rest[len(rest)-1] == ')'")
		content := rest[:len(rest)-1]

		content = strings.ReplaceAll(content, `\\`, "\x00")
		content = strings.ReplaceAll(content, `\(`, "(")
		content = strings.ReplaceAll(content, `\)`, ")")
		content = strings.ReplaceAll(content, "\x00", `\`)
		observe.GlobalTrace("return: toolName, content")
		observe.GlobalTrace("return: toolName, content")
		observe.GlobalTrace("return: toolName, content")
		return toolName, content
	}
	observe.GlobalTrace("return: toolName, \"\"")
	observe.GlobalTrace("return: toolName, \"\"")
	observe.GlobalTrace("return: toolName, \"\"")

	return toolName, ""
}

func parseDecision(behavior string) Decision {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch strings.ToLower(behavior) {
	case "allow":
		observe.GlobalTrace("case: \"allow\"")
		return DecisionAllow
	case "deny":
		observe.GlobalTrace("case: \"deny\"")
		return DecisionDeny
	case "ask":
		observe.GlobalTrace("case: \"ask\"")
		return DecisionAsk
	default:
		observe.GlobalTrace("default")
		return ""
	}
}

func parseSource(source string) RuleSource {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch source {
	case "policySettings":
		observe.GlobalTrace("case: \"policySettings\"")
		return SourcePolicy
	case "userSettings":
		observe.GlobalTrace("case: \"userSettings\"")
		return SourceUser
	case "projectSettings":
		observe.GlobalTrace("case: \"projectSettings\"")
		return SourceProject
	case "localSettings":
		observe.GlobalTrace("case: \"localSettings\"")
		return SourceLocal
	case "cliArg":
		observe.GlobalTrace("case: \"cliArg\"")
		return SourceCLI
	case "session":
		observe.GlobalTrace("case: \"session\"")
		return SourceSession
	default:
		observe.GlobalTrace("default")
		return SourceUser
	}
}
