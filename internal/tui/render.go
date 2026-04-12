package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
)

const maxToolResultDisplay = 200

// renderMessage renders a single message for the viewport.
func renderMessage(msg model.Message) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder

	switch msg.Role {
	case model.RoleUser:
		observe.GlobalTrace("case: model.RoleUser")
		if msg.Flags.IsInternal {
			observe.GlobalTrace("return: \"\"")
			observe.GlobalTrace("return: \"\"")
			observe.GlobalTrace("return: \"\"")
			return ""
		}
		b.WriteString(userLabelStyle.Render("You"))
		b.WriteString("\n")
	case model.RoleAssistant:
		observe.GlobalTrace("case: model.RoleAssistant")
		b.WriteString(assistantLabelStyle.Render("Assistant"))
		b.WriteString("\n")
	}

	for _, part := range msg.Content {
		observe.GlobalTrace("range msg.Content")
		rendered := renderContentPart(part)
		if rendered != "" {
			observe.GlobalTrace("if: rendered != \"\"")
			b.WriteString(rendered)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	observe.GlobalTrace("return: b.String()")
	observe.GlobalTrace("return: b.String()")
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

// renderContentPart renders a single content part.
func renderContentPart(part model.ContentPart) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch p := part.(type) {
	case model.TextPart:
		observe.GlobalTrace("typecase: model.TextPart")
		return p.Text
	case model.ThinkingPart:
		observe.GlobalTrace("typecase: model.ThinkingPart")
		return renderThinking(p)
	case model.ToolCallPart:
		observe.GlobalTrace("typecase: model.ToolCallPart")
		return renderToolCall(p)
	case model.ToolResultPart:
		observe.GlobalTrace("typecase: model.ToolResultPart")
		return renderToolResult(p)
	case model.ImagePart:
		observe.GlobalTrace("typecase: model.ImagePart")
		return "[image: " + p.MimeType + "]"
	case model.DocumentPart:
		observe.GlobalTrace("typecase: model.DocumentPart")
		return "[document: " + p.MimeType + "]"
	default:
		observe.GlobalTrace("typedefault")
		return ""
	}
}

// renderToolCall renders a tool call with name and truncated input.
func renderToolCall(tc model.ToolCallPart) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var inputPreview string
	if len(tc.Input) > 0 {
		observe.GlobalTrace("if: len(tc.Input) > 0")
		var parsed map[string]any
		if json.Unmarshal(tc.Input, &parsed) == nil {
			observe.GlobalTrace("if: json.Unmarshal(tc.Input, &parsed) == nil")
			// Show key=value pairs, truncated
			var pairs []string
			for k, v := range parsed {
				observe.GlobalTrace("range parsed")
				s := fmt.Sprintf("%s=%v", k, v)
				if len(s) > 60 {
					observe.GlobalTrace("if: len(s) > 60")
					s = s[:57] + "..."
				}
				pairs = append(pairs, s)
				if len(pairs) >= 3 {
					observe.GlobalTrace("if: len(pairs) >= 3")
					break
				}
			}
			inputPreview = strings.Join(pairs, " ")
		}
	}

	label := toolCallStyle.Render("tool: " + tc.Name)
	if inputPreview != "" {
		observe.GlobalTrace("if: inputPreview != \"\"")
		observe.GlobalTrace("return: label + \" \" + toolResultStyle.Render(inputPreview)")
		observe.GlobalTrace("return: label + \" \" + toolResultStyle.Render(inputPreview)")
		observe.GlobalTrace("return: label + \" \" + toolResultStyle.Render(inputPreview)")
		return label + " " + toolResultStyle.Render(inputPreview)
	}
	observe.GlobalTrace("return: label")
	observe.GlobalTrace("return: label")
	observe.GlobalTrace("return: label")
	return label
}

// renderToolResult renders a tool result, truncated for display.
func renderToolResult(tr model.ToolResultPart) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	prefix := "result"
	if tr.IsError {
		observe.GlobalTrace("if: tr.IsError")
		prefix = errorStyle.Render("error")
	}

	content := tr.Content
	if len(content) > maxToolResultDisplay {
		observe.GlobalTrace("if: len(content) > maxToolResultDisplay")
		content = content[:maxToolResultDisplay] + "..."
	}

	content = strings.ReplaceAll(content, "\n", " ")
	observe.GlobalTrace("return: toolResultStyle.Render(fmt.Sprintf(\"[%s] %s\", prefix, content))")
	observe.GlobalTrace("return: toolResultStyle.Render(fmt.Sprintf(\"[%s] %s\", prefix, content))")
	observe.GlobalTrace("return: toolResultStyle.Render(fmt.Sprintf(\"[%s] %s\", prefix, content))")

	return toolResultStyle.Render(fmt.Sprintf("[%s] %s", prefix, content))
}

// renderThinking renders a thinking block, dimmed.
func renderThinking(tp model.ThinkingPart) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if tp.Redacted {
		observe.GlobalTrace("if: tp.Redacted")
		observe.GlobalTrace("return: thinkingStyle.Render(\"[thinking redacted]\")")
		observe.GlobalTrace("return: thinkingStyle.Render(\"[thinking redacted]\")")
		observe.GlobalTrace("return: thinkingStyle.Render(\"[thinking redacted]\")")
		return thinkingStyle.Render("[thinking redacted]")
	}
	text := tp.Text
	if len(text) > 500 {
		observe.GlobalTrace("if: len(text) > 500")
		text = text[:497] + "..."
	}
	observe.GlobalTrace("return: thinkingStyle.Render(text)")
	observe.GlobalTrace("return: thinkingStyle.Render(text)")
	observe.GlobalTrace("return: thinkingStyle.Render(text)")
	return thinkingStyle.Render(text)
}

// renderConversation renders all visible messages for the viewport.
func renderConversation(msgs []model.Message) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder
	for _, msg := range msgs {
		observe.GlobalTrace("range msgs")
		rendered := renderMessage(msg)
		if rendered != "" {
			observe.GlobalTrace("if: rendered != \"\"")
			b.WriteString(rendered)
		}
	}
	observe.GlobalTrace("return: b.String()")
	observe.GlobalTrace("return: b.String()")
	observe.GlobalTrace("return: b.String()")
	return b.String()
}
