package mcp

import (
	"encoding/json"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/artpar/pragma/internal/observe"
)

// extractTextContent concatenates text from CallToolResult content blocks.
func extractTextContent(result *mcp.CallToolResult) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if result == nil || len(result.Content) == 0 {
		observe.GlobalTrace("if: result == nil || len(result.Content) == 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	var parts []string
	for _, content := range result.Content {
		observe.GlobalTrace("range result.Content")
		switch c := content.(type) {
		case mcp.TextContent:
			observe.GlobalTrace("typecase: mcp.TextContent")
			parts = append(parts, c.Text)
		case *mcp.TextContent:
			observe.GlobalTrace("typecase: *mcp.TextContent")
			parts = append(parts, c.Text)
		default:
			observe.GlobalTrace("typedefault")

			data, err := json.Marshal(c)
			if err == nil {
				parts = append(parts, string(data))
			}
		}
	}
	observe.GlobalTrace("return: strings.Join(parts, \"\\n\")")
	return strings.Join(parts, "\n")
}
