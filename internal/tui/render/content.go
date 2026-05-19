package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// Styles used by content rendering. These use AdaptiveColor
// to prevent TS issues #1302, #16514, #34905, #41098, #45084.
var (
	userLabel = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "30", Dark: "86"})

	thinkStyle = lipgloss.NewStyle().
			Faint(true).
			Italic(true)

	toolCall = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "24", Dark: "75"})

	toolParam = lipgloss.NewStyle().Faint(true)

	bracketDim = lipgloss.NewStyle().Faint(true)

	bracketErr = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "124", Dark: "196"})

	errBold = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.AdaptiveColor{Light: "124", Dark: "196"})
)

// primaryParams maps tool names to their primary input parameter.
var primaryParams = map[string]string{
	"Bash":                        "command",
	"Read":                        "file_path",
	"Edit":                        "file_path",
	"Write":                       "file_path",
	"Grep":                        "pattern",
	"Glob":                        "pattern",
	"Agent":                       "prompt",
	"ide.file.open":               "filePath",
	"ide.file.resolve":            "filePath",
	"ide.search.text":             "query",
	"ide.object.describe":         "object",
	"ide.object.call":             "method",
	"ide.object.release":          "object",
	"ide.plugin.resolve":          "pluginId",
	"ide.plugin.enable":           "pluginId",
	"ide.plugin.disable":          "pluginId",
	"ide.plugin.load":             "pluginId",
	"ide.plugin.unload":           "pluginId",
	"ide.plugin.install":          "pluginId",
	"ide.plugin.uninstall":        "pluginId",
	"ide.plugin.self.update":      "filePath",
	"ide.debug.breakpoint.set":    "filePath",
	"ide.debug.breakpoint.remove": "filePath",
	"tool_result.read":            "tool_call_id",
}

// RenderMessage renders a single message for the viewport.
func RenderMessage(msg model.Message, md *MarkdownRenderer) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder

	switch msg.Role {
	case model.RoleUser:
		observe.GlobalTrace("case: model.RoleUser")
		if msg.Flags.IsInternal {
			observe.GlobalTrace("return: \"\"")
			return ""
		}
		// Render user message inline: ❯ message text (no separate "You" label)
		var parts []string
		for _, part := range msg.Content {
			observe.GlobalTrace("range msg.Content")
			rendered := RenderContentPart(part, md, 80)
			if rendered != "" {
				observe.GlobalTrace("if: rendered != \"\"")
				parts = append(parts, rendered)
			}
		}
		if len(parts) == 0 {
			observe.GlobalTrace("return: \"\"")
			return ""
		}
		b.WriteString(userLabel.Render("❯") + " " + strings.Join(parts, "\n"))
		b.WriteString("\n\n")
		observe.GlobalTrace("return: b.String()")
		return b.String()

	case model.RoleAssistant:
		observe.GlobalTrace("case: model.RoleAssistant")

	}

	for _, part := range msg.Content {
		observe.GlobalTrace("range msg.Content")
		rendered := RenderContentPart(part, md, 80)
		if rendered != "" {
			observe.GlobalTrace("if: rendered != \"\"")
			b.WriteString(rendered)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

// RenderConversation renders all visible messages.
func RenderConversation(msgs []model.Message, md *MarkdownRenderer) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder
	for _, msg := range msgs {
		observe.GlobalTrace("range msgs")
		rendered := RenderMessage(msg, md)
		if rendered == "" {
			observe.GlobalTrace("if: rendered == \"\"")
			continue
		}
		b.WriteString(rendered)
	}
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

// RenderContentPart renders a single content part.
func RenderContentPart(part model.ContentPart, md *MarkdownRenderer, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch p := part.(type) {
	case model.TextPart:
		observe.GlobalTrace("typecase: model.TextPart")
		if md != nil {
			observe.GlobalTrace("return: md.Render(p.Text)")
			return md.Render(p.Text)
		}
		return p.Text
	case model.ThinkingPart:
		observe.GlobalTrace("typecase: model.ThinkingPart")
		return RenderThinking(p, true)
	case model.ToolCallPart:
		observe.GlobalTrace("typecase: model.ToolCallPart")
		return RenderToolCall(p, width)
	case model.ToolResultPart:
		observe.GlobalTrace("typecase: model.ToolResultPart")
		return RenderToolResultGeneric(p, width)
	case model.ImagePart:
		observe.GlobalTrace("typecase: model.ImagePart")
		return bracketDim.Render(BracketPrefix+"[image: "+p.MimeType+"]") + "\n"
	case model.DocumentPart:
		observe.GlobalTrace("typecase: model.DocumentPart")
		return bracketDim.Render(BracketPrefix+"[document: "+p.MimeType+"]") + "\n"
	default:
		observe.GlobalTrace("typedefault")
		return ""
	}
}

// RenderThinking renders a thinking block with glyph prefix.
// Collapsed (default): single-line "∴ Thinking [Ctrl+O to expand]".
// Expanded: "∴ Thinking…" header + full text indented 2 spaces.
// Matches TS AssistantThinkingMessage.tsx collapsed/expanded behavior.
func RenderThinking(tp model.ThinkingPart, expanded bool) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if tp.Redacted {
		observe.GlobalTrace("if: tp.Redacted")
		observe.GlobalTrace("return: thinkStyle.Render(ThinkGlyph + \" [thinking redacted]\")")
		return thinkStyle.Render(ThinkGlyph + " [thinking redacted]")
	}
	if !expanded {
		observe.GlobalTrace("if: !expanded")
		observe.GlobalTrace("return: thinkStyle.Render(ThinkGlyph+\" Thinking\") +\n\t\" \" + lipgloss.NewStyle().Faint(...")
		return thinkStyle.Render(ThinkGlyph+" Thinking") +
			" " + lipgloss.NewStyle().Faint(true).Render("(ctrl+o to expand)")
	}

	header := thinkStyle.Render(ThinkGlyph + " Thinking\u2026")
	lines := strings.Split(tp.Text, "\n")
	for i, line := range lines {
		observe.GlobalTrace("range lines")
		lines[i] = "  " + line
	}
	body := thinkStyle.Render(strings.Join(lines, "\n"))
	observe.GlobalTrace("return: header + \"\\n\" + body")
	return header + "\n" + body
}

// RenderToolCall renders a tool call as ⏺ ToolName(primaryArg) inline.
func RenderToolCall(tc model.ToolCallPart, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	label := toolCall.Render(tc.Name)
	arg := ""

	if len(tc.Input) > 0 {
		observe.GlobalTrace("if: len(tc.Input) > 0")
		var parsed map[string]any
		if json.Unmarshal(tc.Input, &parsed) == nil {
			observe.GlobalTrace("if: json.Unmarshal(tc.Input, &parsed) == nil")
			primaryKey := primaryParams[tc.Name]
			if primaryKey == "" {
				observe.GlobalTrace("if: primaryKey == \"\"")
				for k := range parsed {
					observe.GlobalTrace("range parsed")
					primaryKey = k
					break
				}
			}
			if val, ok := parsed[primaryKey]; ok {
				observe.GlobalTrace("if: ok")
				valStr := fmt.Sprintf("%v", val)

				maxLen := width - len(tc.Name) - 4
				if maxLen > 10 && len(valStr) > maxLen {
					observe.GlobalTrace("if: maxLen > 10 && len(valStr) > maxLen")
					valStr = valStr[:maxLen-3] + "..."
				}
				arg = valStr
			}
		}
	}

	line := BlackCircle + " " + label
	if arg != "" {
		observe.GlobalTrace("if: arg != \"\"")
		line += toolParam.Render("(" + arg + ")")
	}
	observe.GlobalTrace("return: line + \"\\n\"")
	return line + "\n"
}

// RenderToolResultGeneric renders a tool result with bracket wrapper.
// Used as fallback when tool name is unknown (no correlation).
func RenderToolResultGeneric(tr model.ToolResultPart, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: WrapWithBracket(tr.Content, tr.IsError, width, false)")
	return WrapWithBracket(tr.Content, tr.IsError, width, false)
}

// WrapWithBracket wraps content with the ⎿ bracket glyph.
// Matches TS MessageResponse component layout.
// verbose controls whether output is truncated (non-verbose: 15 lines) or shown in full.
func WrapWithBracket(content string, isError bool, width int, verbose bool) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if content == "" {
		observe.GlobalTrace("if: content == \"\"")
		if isError {
			observe.GlobalTrace("if: isError")
			observe.GlobalTrace("return: bracketErr.Render(BracketPrefix + \"(error)\")")
			return bracketErr.Render(BracketPrefix + "(error)")
		}
		observe.GlobalTrace("return: bracketDim.Render(BracketPrefix + \"(no output)\")")
		return bracketDim.Render(BracketPrefix + "(no output)")
	}

	lines := strings.Split(content, "\n")

	var b strings.Builder
	bracket := bracketDim
	if isError {
		observe.GlobalTrace("if: isError")
		bracket = bracketErr
	}

	maxLines := 15
	if verbose {
		observe.GlobalTrace("if: verbose")
		maxLines = 200
	}
	truncated := false
	if len(lines) > maxLines {
		observe.GlobalTrace("if: len(lines) > maxLines")
		truncated = true
		lines = lines[:maxLines]
	}

	for i, line := range lines {
		observe.GlobalTrace("range lines")
		if i == 0 {
			observe.GlobalTrace("if: i == 0")
			b.WriteString(bracket.Render(BracketPrefix))
			b.WriteString(truncateLine(line, width-len(BracketPrefix)))
		} else {
			observe.GlobalTrace("else: i == 0")
			b.WriteString(ContentIndent)
			b.WriteString(truncateLine(line, width-len(ContentIndent)))
		}
		b.WriteString("\n")
	}

	if truncated {
		observe.GlobalTrace("if: truncated")
		remaining := len(strings.Split(content, "\n")) - maxLines
		b.WriteString(ContentIndent)
		b.WriteString(bracketDim.Render(fmt.Sprintf("(+%d more lines)", remaining)))
		b.WriteString("\n")
		if !verbose {
			observe.GlobalTrace("if: !verbose")
			appendExpandHint(&b)
		}
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

// truncateLine truncates a line to fit within maxWidth using CJK-aware
// string width measurement. Prevents GitHub issues #39593, #46217, #30716
// where full-width characters broke layout calculations.
func truncateLine(line string, maxWidth int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if maxWidth <= 0 {
		observe.GlobalTrace("if: maxWidth <= 0")
		observe.GlobalTrace("return: line")
		return line
	}
	w := runewidth.StringWidth(line)
	if w <= maxWidth {
		observe.GlobalTrace("if: w <= maxWidth")
		observe.GlobalTrace("return: line")
		return line
	}
	if maxWidth < 4 {
		observe.GlobalTrace("if: maxWidth < 4")
		observe.GlobalTrace("return: runewidth.Truncate(line, maxWidth, \"\")")
		return runewidth.Truncate(line, maxWidth, "")
	}
	observe.GlobalTrace("return: runewidth.Truncate(line, maxWidth, \"...\")")
	return runewidth.Truncate(line, maxWidth, "...")
}
