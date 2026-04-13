package permission

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/artpar/gogent/internal/observe"
	"github.com/bmatcuk/doublestar/v4"
)

// MatchContent dispatches to the appropriate matching strategy based on the rule content prefix.
// It matches a rule's content pattern against the actual content extracted from a tool invocation.
func MatchContent(ruleContent, actualContent, workDir string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if ruleContent == "" || actualContent == "" {
		observe.GlobalTrace("if: ruleContent == \"\" || actualContent == \"\"")
		observe.GlobalTrace("return: false")
		return false
	}
	if strings.HasPrefix(ruleContent, "domain:") {
		observe.GlobalTrace("if: strings.HasPrefix(ruleContent, \"domain:\")")
		observe.GlobalTrace("return: MatchDomainContent(ruleContent, actualContent)")
		return MatchDomainContent(ruleContent, actualContent)
	}

	if strings.HasPrefix(actualContent, "/") || strings.HasPrefix(actualContent, "~") {
		observe.GlobalTrace("if: strings.HasPrefix(actualContent, \"/\") || strings.HasPrefix(actualContent, \"~\")")
		observe.GlobalTrace("return: MatchPathContent(ruleContent, actualContent, workDir)")
		return MatchPathContent(ruleContent, actualContent, workDir)
	}
	observe.GlobalTrace("return: MatchShellContent(ruleContent, actualContent)")
	return MatchShellContent(ruleContent, actualContent)
}

// MatchShellContent matches a shell command against a permission rule pattern.
// Supports three pattern types:
//   - exact: "npm install" matches "npm install"
//   - prefix (legacy): "npm:*" matches any command starting with "npm"
//   - wildcard: "npm *" matches "npm" followed by anything (* is glob)
func MatchShellContent(pattern, command string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if pattern == command {
		observe.GlobalTrace("if: pattern == command")
		observe.GlobalTrace("return: true")
		return true
	}

	if strings.HasSuffix(pattern, ":*") {
		observe.GlobalTrace("if: strings.HasSuffix(pattern, \":*\")")
		prefix := strings.TrimSuffix(pattern, ":*")
		observe.GlobalTrace("return: command == prefix || strings.HasPrefix(command, prefix+\" \")")
		return command == prefix || strings.HasPrefix(command, prefix+" ")
	}
	observe.GlobalTrace("return: matchWildcard(pattern, command)")

	return matchWildcard(pattern, command)
}

// MatchPathContent matches a file path against a gitignore-style pattern.
// Uses doublestar for globbing. Patterns are resolved relative to workDir.
func MatchPathContent(pattern, filePath, workDir string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	cleanPath := filepath.Clean(filePath)

	if strings.HasPrefix(pattern, "/") {
		observe.GlobalTrace("if: strings.HasPrefix(pattern, \"/\")")
		pattern = filepath.Join(workDir, pattern)
	}
	pattern = filepath.Clean(pattern)

	matched, err := doublestar.Match(pattern, cleanPath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: false")
		return false
	}
	observe.GlobalTrace("return: matched")
	return matched
}

// MatchDomainContent matches a domain rule against a domain content string.
// Rule format: "domain:github.com", content format: "domain:github.com".
func MatchDomainContent(ruleContent, actualContent string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !strings.HasPrefix(ruleContent, "domain:") || !strings.HasPrefix(actualContent, "domain:") {
		observe.GlobalTrace("if: !strings.HasPrefix(ruleContent, \"domain:\") || !strings.HasPrefix(actualConten...")
		observe.GlobalTrace("return: false")
		return false
	}
	ruleDomain := strings.TrimPrefix(ruleContent, "domain:")
	actualDomain := strings.TrimPrefix(actualContent, "domain:")
	observe.GlobalTrace("return: strings.EqualFold(ruleDomain, actualDomain)")
	return strings.EqualFold(ruleDomain, actualDomain)
}

// matchWildcard converts a glob-style pattern to a regex and matches.
// `*` matches any characters, `\*` matches literal `*`, `\\` matches literal `\`.
// A pattern ending with ` *` also matches the bare command (trailing space is optional).
func matchWildcard(pattern, input string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	trailingOptional := strings.HasSuffix(pattern, " *")

	// Build regex from pattern
	var re strings.Builder
	re.WriteString("^")

	i := 0
	for i < len(pattern) {
		observe.GlobalTrace("for: i < len(pattern)")
		ch := pattern[i]
		switch {
		case ch == '\\' && i+1 < len(pattern):
			observe.GlobalTrace("case: ch == '\\\\' && i+1 < len(pattern)")
			next := pattern[i+1]
			if next == '*' {
				re.WriteString(`\*`)
			} else if next == '\\' {
				re.WriteString(`\\`)
			} else {
				re.WriteString(regexp.QuoteMeta(string(next)))
			}
			i += 2
		case ch == '*':
			observe.GlobalTrace("case: ch == '*'")
			re.WriteString(".*")
			i++
		default:
			observe.GlobalTrace("default")
			re.WriteString(regexp.QuoteMeta(string(ch)))
			i++
		}
	}
	re.WriteString("$")

	compiled, err := regexp.Compile(re.String())
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: false")
		return false
	}
	if compiled.MatchString(input) {
		observe.GlobalTrace("if: compiled.MatchString(input)")
		observe.GlobalTrace("return: true")
		return true
	}

	if trailingOptional {
		observe.GlobalTrace("if: trailingOptional")
		barePattern := strings.TrimSuffix(pattern, " *")
		observe.GlobalTrace("return: input == barePattern")
		return input == barePattern
	}
	observe.GlobalTrace("return: false")
	return false
}
