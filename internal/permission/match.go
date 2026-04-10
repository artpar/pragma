package permission

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// MatchContent dispatches to the appropriate matching strategy based on the rule content prefix.
// It matches a rule's content pattern against the actual content extracted from a tool invocation.
func MatchContent(ruleContent, actualContent, workDir string) bool {
	if ruleContent == "" || actualContent == "" {
		return false
	}
	if strings.HasPrefix(ruleContent, "domain:") {
		return MatchDomainContent(ruleContent, actualContent)
	}
	// If actualContent starts with "/" or looks like an absolute path, use path matching.
	// Otherwise use shell matching (for Bash commands).
	if strings.HasPrefix(actualContent, "/") || strings.HasPrefix(actualContent, "~") {
		return MatchPathContent(ruleContent, actualContent, workDir)
	}
	return MatchShellContent(ruleContent, actualContent)
}

// MatchShellContent matches a shell command against a permission rule pattern.
// Supports three pattern types:
//   - exact: "npm install" matches "npm install"
//   - prefix (legacy): "npm:*" matches any command starting with "npm"
//   - wildcard: "npm *" matches "npm" followed by anything (* is glob)
func MatchShellContent(pattern, command string) bool {
	if pattern == command {
		return true
	}

	// Legacy prefix syntax: "cmd:*" matches commands starting with "cmd "
	if strings.HasSuffix(pattern, ":*") {
		prefix := strings.TrimSuffix(pattern, ":*")
		return command == prefix || strings.HasPrefix(command, prefix+" ")
	}

	// Wildcard matching: convert glob pattern to regex.
	return matchWildcard(pattern, command)
}

// MatchPathContent matches a file path against a gitignore-style pattern.
// Uses doublestar for globbing. Patterns are resolved relative to workDir.
func MatchPathContent(pattern, filePath, workDir string) bool {
	// Normalize the file path
	cleanPath := filepath.Clean(filePath)

	// If pattern starts with /, it's relative to workDir
	if strings.HasPrefix(pattern, "/") {
		pattern = filepath.Join(workDir, pattern)
	}
	pattern = filepath.Clean(pattern)

	matched, err := doublestar.Match(pattern, cleanPath)
	if err != nil {
		return false
	}
	return matched
}

// MatchDomainContent matches a domain rule against a domain content string.
// Rule format: "domain:github.com", content format: "domain:github.com".
func MatchDomainContent(ruleContent, actualContent string) bool {
	if !strings.HasPrefix(ruleContent, "domain:") || !strings.HasPrefix(actualContent, "domain:") {
		return false
	}
	ruleDomain := strings.TrimPrefix(ruleContent, "domain:")
	actualDomain := strings.TrimPrefix(actualContent, "domain:")
	return strings.EqualFold(ruleDomain, actualDomain)
}

// matchWildcard converts a glob-style pattern to a regex and matches.
// `*` matches any characters, `\*` matches literal `*`, `\\` matches literal `\`.
// A pattern ending with ` *` also matches the bare command (trailing space is optional).
func matchWildcard(pattern, input string) bool {
	// Check if trailing space should be optional: pattern ends with " *"
	trailingOptional := strings.HasSuffix(pattern, " *")

	// Build regex from pattern
	var re strings.Builder
	re.WriteString("^")

	i := 0
	for i < len(pattern) {
		ch := pattern[i]
		switch {
		case ch == '\\' && i+1 < len(pattern):
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
			re.WriteString(".*")
			i++
		default:
			re.WriteString(regexp.QuoteMeta(string(ch)))
			i++
		}
	}
	re.WriteString("$")

	compiled, err := regexp.Compile(re.String())
	if err != nil {
		return false
	}
	if compiled.MatchString(input) {
		return true
	}

	// If pattern ends with " *", also match the bare command (without the trailing part)
	if trailingOptional {
		barePattern := strings.TrimSuffix(pattern, " *")
		return input == barePattern
	}
	return false
}
