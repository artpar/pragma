package mcp

import (
	"github.com/artpar/gogent/internal/observe"
	"regexp"
	"strings"
)

const mcpPrefix = "mcp__"

var nonAlphanumericRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
var multiUnderscoreRe = regexp.MustCompile(`_{2,}`)

// NormalizeName replaces non-alphanumeric chars (except _ and -)
// with underscores, then collapses consecutive underscores.
// Matches the TypeScript normalizeNameForMCP exactly.
func NormalizeName(name string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	n := nonAlphanumericRe.ReplaceAllString(name, "_")
	n = multiUnderscoreRe.ReplaceAllString(n, "_")
	observe.GlobalTrace("return: n")
	return n
}

// BuildToolName returns the fully-qualified MCP tool name: "mcp__<server>__<tool>".
func BuildToolName(serverName, toolName string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: mcpPrefix + NormalizeName(serverName) + \"__\" + NormalizeName(toolName)")
	return mcpPrefix + NormalizeName(serverName) + "__" + NormalizeName(toolName)
}

// ParseToolName extracts serverName and toolName from "mcp__<server>__<tool>".
// Returns empty strings and false if fullName is not a valid MCP tool name.
func ParseToolName(fullName string) (serverName, toolName string, ok bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !strings.HasPrefix(fullName, mcpPrefix) {
		observe.GlobalTrace("if: !strings.HasPrefix(fullName, mcpPrefix)")
		observe.GlobalTrace("return: \"\", \"\", false")
		return "", "", false
	}
	rest := fullName[len(mcpPrefix):]
	idx := strings.Index(rest, "__")
	if idx < 0 || idx == 0 {
		observe.GlobalTrace("if: idx < 0 || idx == 0")
		observe.GlobalTrace("return: \"\", \"\", false")
		return "", "", false
	}
	observe.GlobalTrace("return: rest[:idx], rest[idx+2:], true")
	return rest[:idx], rest[idx+2:], true
}

// IsMCPTool returns true if name has the mcp__ prefix.
func IsMCPTool(name string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: strings.HasPrefix(name, mcpPrefix)")
	return strings.HasPrefix(name, mcpPrefix)
}
