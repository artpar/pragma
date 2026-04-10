package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/gogent/internal/model"
)

const maxToolResultDisplay = 200

// renderMessage renders a single message for the viewport.
func renderMessage(msg model.Message) string {
	var b strings.Builder

	switch msg.Role {
	case model.RoleUser:
		if msg.Flags.IsInternal {
			return ""
		}
		b.WriteString(userLabelStyle.Render("You"))
		b.WriteString("\n")
	case model.RoleAssistant:
		b.WriteString(assistantLabelStyle.Render("Assistant"))
		b.WriteString("\n")
	}

	for _, part := range msg.Content {
		rendered := renderContentPart(part)
		if rendered != "" {
			b.WriteString(rendered)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

// renderContentPart renders a single content part.
func renderContentPart(part model.ContentPart) string {
	switch p := part.(type) {
	case model.TextPart:
		return p.Text
	case model.ThinkingPart:
		return renderThinking(p)
	case model.ToolCallPart:
		return renderToolCall(p)
	case model.ToolResultPart:
		return renderToolResult(p)
	case model.ImagePart:
		return "[image: " + p.MimeType + "]"
	case model.DocumentPart:
		return "[document: " + p.MimeType + "]"
	default:
		return ""
	}
}

// renderToolCall renders a tool call with name and truncated input.
func renderToolCall(tc model.ToolCallPart) string {
	var inputPreview string
	if len(tc.Input) > 0 {
		var parsed map[string]any
		if json.Unmarshal(tc.Input, &parsed) == nil {
			// Show key=value pairs, truncated
			var pairs []string
			for k, v := range parsed {
				s := fmt.Sprintf("%s=%v", k, v)
				if len(s) > 60 {
					s = s[:57] + "..."
				}
				pairs = append(pairs, s)
				if len(pairs) >= 3 {
					break
				}
			}
			inputPreview = strings.Join(pairs, " ")
		}
	}

	label := toolCallStyle.Render("tool: " + tc.Name)
	if inputPreview != "" {
		return label + " " + toolResultStyle.Render(inputPreview)
	}
	return label
}

// renderToolResult renders a tool result, truncated for display.
func renderToolResult(tr model.ToolResultPart) string {
	prefix := "result"
	if tr.IsError {
		prefix = errorStyle.Render("error")
	}

	content := tr.Content
	if len(content) > maxToolResultDisplay {
		content = content[:maxToolResultDisplay] + "..."
	}
	// Collapse to single line for display
	content = strings.ReplaceAll(content, "\n", " ")

	return toolResultStyle.Render(fmt.Sprintf("[%s] %s", prefix, content))
}

// renderThinking renders a thinking block, dimmed.
func renderThinking(tp model.ThinkingPart) string {
	if tp.Redacted {
		return thinkingStyle.Render("[thinking redacted]")
	}
	text := tp.Text
	if len(text) > 500 {
		text = text[:497] + "..."
	}
	return thinkingStyle.Render(text)
}

// renderConversation renders all visible messages for the viewport.
func renderConversation(msgs []model.Message) string {
	var b strings.Builder
	for _, msg := range msgs {
		rendered := renderMessage(msg)
		if rendered != "" {
			b.WriteString(rendered)
		}
	}
	return b.String()
}
