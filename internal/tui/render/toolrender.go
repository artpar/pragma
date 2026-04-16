package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/observe"
	"github.com/charmbracelet/lipgloss"
)

var (
	diffAdd = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "114"})

	diffRemove = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "124", Dark: "210"})

	diffGutter = lipgloss.NewStyle().Faint(true)

	fileHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "30", Dark: "86"})

	dimText = lipgloss.NewStyle().Faint(true)
)

// ToolRenderer renders a tool result given context.
type ToolRenderer func(input json.RawMessage, content string, isError bool, width int) string

// toolRenderers maps tool names to their specific renderer.
var toolRenderers = map[string]ToolRenderer{
	"Bash":  renderBash,
	"Read":  renderRead,
	"Edit":  renderEdit,
	"Write": renderWrite,
	"Grep":  renderGrep,
	"Glob":  renderGlob,
	"Agent": renderAgent,
}

// RenderToolOutput dispatches to a tool-specific renderer or generic fallback.
func RenderToolOutput(name string, input json.RawMessage, content string, isError bool, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if renderer, ok := toolRenderers[name]; ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: renderer(input, content, isError, width)")
		return renderer(input, content, isError, width)
	}
	observe.GlobalTrace("return: WrapWithBracket(content, isError, width)")
	return WrapWithBracket(content, isError, width)
}

// renderBash renders Bash tool output.
// Shows stdout content with tail truncation for long output.
// The command itself is already shown in the tool call line (⏺ Bash(cmd)).
func renderBash(input json.RawMessage, content string, isError bool, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder

	if isError {
		observe.GlobalTrace("if: isError")
		b.WriteString(bracketErr.Render(BracketPrefix))
		b.WriteString(errBold.Render(truncateContent(content, width-len(ContentIndent), 10)))
		b.WriteString("\n")
		observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")
		return strings.TrimRight(b.String(), "\n")
	}

	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		observe.GlobalTrace("if: trimmed == \"\"")
		b.WriteString(ContentIndent)
		b.WriteString(dimText.Render("(no output)"))
		b.WriteString("\n")
		observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")
		return strings.TrimRight(b.String(), "\n")
	}

	lines := strings.Split(trimmed, "\n")
	maxLines := 15
	if len(lines) > maxLines {
		observe.GlobalTrace("if: len(lines) > maxLines")
		skipped := len(lines) - (maxLines - 1)

		b.WriteString(ContentIndent)
		b.WriteString(dimText.Render(fmt.Sprintf("(%d lines hidden)", skipped)))
		b.WriteString("\n")
		lines = lines[skipped:]
	}

	for _, line := range lines {
		observe.GlobalTrace("range lines")
		b.WriteString(ContentIndent)
		b.WriteString(truncateLine(line, width-len(ContentIndent)))
		b.WriteString("\n")
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

// renderRead renders FileRead tool output as a compact "Read N lines" summary.
// The file path is already shown in the tool call line (⏺ Read(path)).
func renderRead(input json.RawMessage, content string, isError bool, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if isError {
		observe.GlobalTrace("if: isError")
		observe.GlobalTrace("return: WrapWithBracket(content, true, width)")
		return WrapWithBracket(content, true, width)
	}

	var params struct {
		Offset int `json:"offset"`
		Limit  int `json:"limit"`
	}
	json.Unmarshal(input, &params)

	lineCount := strings.Count(strings.TrimRight(content, "\n"), "\n") + 1
	if strings.TrimSpace(content) == "" {
		lineCount = 0
	}

	noun := "lines"
	if lineCount == 1 {
		noun = "line"
	}

	summary := fmt.Sprintf("Read %d %s", lineCount, noun)
	if params.Offset > 0 {
		observe.GlobalTrace("if: params.Offset > 0")
		summary += fmt.Sprintf(" (from line %d)", params.Offset+1)
	}

	observe.GlobalTrace("return: bracketDim.Render(BracketPrefix) + dimText.Render(summary)")
	return bracketDim.Render(BracketPrefix) + dimText.Render(summary)
}

// renderEdit renders FileEdit tool output as a unified diff.
// Reconstructs diff from old_string/new_string inputs.
func renderEdit(input json.RawMessage, content string, isError bool, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if isError {
		observe.GlobalTrace("if: isError")
		observe.GlobalTrace("return: WrapWithBracket(content, true, width)")
		return WrapWithBracket(content, true, width)
	}

	var b strings.Builder

	var params struct {
		FilePath  string `json:"file_path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	json.Unmarshal(input, &params)

	if params.FilePath != "" {
		observe.GlobalTrace("if: params.FilePath != \"\"")
		b.WriteString(bracketDim.Render(BracketPrefix))
		b.WriteString(fileHeader.Render(params.FilePath))
		b.WriteString("\n")
	}

	if params.OldString != "" || params.NewString != "" {
		observe.GlobalTrace("if: params.OldString != \"\" || params.NewString != \"\"")
		oldLines := strings.Split(params.OldString, "\n")
		newLines := strings.Split(params.NewString, "\n")

		for _, line := range oldLines {
			observe.GlobalTrace("range oldLines")
			b.WriteString(ContentIndent)
			b.WriteString(diffRemove.Render("- " + truncateLine(line, width-len(ContentIndent)-2)))
			b.WriteString("\n")
		}

		for _, line := range newLines {
			observe.GlobalTrace("range newLines")
			b.WriteString(ContentIndent)
			b.WriteString(diffAdd.Render("+ " + truncateLine(line, width-len(ContentIndent)-2)))
			b.WriteString("\n")
		}
	} else {
		observe.GlobalTrace("else: params.OldString != \"\" || params.NewString != \"\"")

		b.WriteString(WrapWithBracket(content, false, width))
		b.WriteString("\n")
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

// renderWrite renders FileWrite tool output.
func renderWrite(input json.RawMessage, content string, isError bool, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if isError {
		observe.GlobalTrace("if: isError")
		observe.GlobalTrace("return: WrapWithBracket(content, true, width)")
		return WrapWithBracket(content, true, width)
	}

	var params struct {
		FilePath string `json:"file_path"`
	}
	json.Unmarshal(input, &params)

	msg := content
	if params.FilePath != "" {
		observe.GlobalTrace("if: params.FilePath != \"\"")
		byteCount := len(content)
		msg = fmt.Sprintf("Wrote %d bytes to %s", byteCount, params.FilePath)
	}
	observe.GlobalTrace("return: bracketDim.Render(BracketPrefix) + dimText.Render(msg)")

	return bracketDim.Render(BracketPrefix) + dimText.Render(msg)
}

// renderGrep renders Grep tool search results.
// Shows summary + file list.
func renderGrep(input json.RawMessage, content string, isError bool, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if isError {
		observe.GlobalTrace("if: isError")
		observe.GlobalTrace("return: WrapWithBracket(content, true, width)")
		return WrapWithBracket(content, true, width)
	}

	lines := strings.Split(strings.TrimSpace(content), "\n")
	fileCount := 0
	for _, line := range lines {
		observe.GlobalTrace("range lines")
		if strings.TrimSpace(line) != "" {
			observe.GlobalTrace("if: strings.TrimSpace(line) != \"\"")
			fileCount++
		}
	}

	var b strings.Builder
	b.WriteString(bracketDim.Render(BracketPrefix))

	if fileCount == 0 {
		observe.GlobalTrace("if: fileCount == 0")
		b.WriteString(dimText.Render("No matches found"))
	} else {
		observe.GlobalTrace("else: fileCount == 0")
		b.WriteString(fmt.Sprintf("Found results in %d files", fileCount))
	}
	b.WriteString("\n")

	maxFiles := 10
	shown := 0
	for _, line := range lines {
		observe.GlobalTrace("range lines")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			observe.GlobalTrace("if: trimmed == \"\"")
			continue
		}
		if shown >= maxFiles {
			observe.GlobalTrace("if: shown >= maxFiles")
			remaining := fileCount - maxFiles
			b.WriteString(ContentIndent)
			b.WriteString(dimText.Render(fmt.Sprintf("(+%d more files)", remaining)))
			b.WriteString("\n")
			break
		}
		b.WriteString(ContentIndent)
		b.WriteString(truncateLine(trimmed, width-len(ContentIndent)))
		b.WriteString("\n")
		shown++
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

// renderGlob renders Glob tool file listing results.
func renderGlob(input json.RawMessage, content string, isError bool, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if isError {
		observe.GlobalTrace("if: isError")
		observe.GlobalTrace("return: WrapWithBracket(content, true, width)")
		return WrapWithBracket(content, true, width)
	}

	lines := strings.Split(strings.TrimSpace(content), "\n")
	fileCount := 0
	for _, line := range lines {
		observe.GlobalTrace("range lines")
		if strings.TrimSpace(line) != "" {
			observe.GlobalTrace("if: strings.TrimSpace(line) != \"\"")
			fileCount++
		}
	}

	var b strings.Builder
	b.WriteString(bracketDim.Render(BracketPrefix))

	if fileCount == 0 {
		observe.GlobalTrace("if: fileCount == 0")
		b.WriteString(dimText.Render("No files found"))
	} else {
		observe.GlobalTrace("else: fileCount == 0")
		b.WriteString(fmt.Sprintf("Found %d files", fileCount))
	}
	b.WriteString("\n")

	maxFiles := 15
	shown := 0
	for _, line := range lines {
		observe.GlobalTrace("range lines")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			observe.GlobalTrace("if: trimmed == \"\"")
			continue
		}
		if shown >= maxFiles {
			observe.GlobalTrace("if: shown >= maxFiles")
			remaining := fileCount - maxFiles
			b.WriteString(ContentIndent)
			b.WriteString(dimText.Render(fmt.Sprintf("(+%d more files)", remaining)))
			b.WriteString("\n")
			break
		}
		b.WriteString(ContentIndent)
		b.WriteString(truncateLine(trimmed, width-len(ContentIndent)))
		b.WriteString("\n")
		shown++
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

// renderAgent renders Agent tool results with diamond glyphs.
func renderAgent(input json.RawMessage, content string, isError bool, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder

	var params struct {
		Prompt string `json:"prompt"`
	}
	json.Unmarshal(input, &params)

	glyph := DiamondFilled
	if isError {
		observe.GlobalTrace("if: isError")
		glyph = DiamondOpen
	}

	summary := params.Prompt
	if len(summary) > 60 {
		observe.GlobalTrace("if: len(summary) > 60")
		summary = summary[:57] + "..."
	}

	b.WriteString(bracketDim.Render(BracketPrefix))
	b.WriteString(glyph + " ")
	if summary != "" {
		observe.GlobalTrace("if: summary != \"\"")
		b.WriteString(dimText.Render(summary))
	}
	b.WriteString("\n")

	if content != "" {
		observe.GlobalTrace("if: content != \"\"")
		lines := strings.Split(strings.TrimSpace(content), "\n")
		maxLines := 5
		if len(lines) > maxLines {
			observe.GlobalTrace("if: len(lines) > maxLines")
			lines = lines[:maxLines]
		}
		for _, line := range lines {
			observe.GlobalTrace("range lines")
			b.WriteString(ContentIndent)
			b.WriteString(truncateLine(line, width-len(ContentIndent)))
			b.WriteString("\n")
		}
		total := len(strings.Split(strings.TrimSpace(content), "\n"))
		if total > maxLines {
			observe.GlobalTrace("if: total > maxLines")
			b.WriteString(ContentIndent)
			b.WriteString(dimText.Render(fmt.Sprintf("(+%d more lines)", total-maxLines)))
			b.WriteString("\n")
		}
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

// truncateContent truncates multi-line content to maxLines.
func truncateContent(content string, width, maxLines int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	lines := strings.Split(content, "\n")
	if len(lines) <= maxLines {
		observe.GlobalTrace("if: len(lines) <= maxLines")
		observe.GlobalTrace("return: content")
		return content
	}
	var b strings.Builder
	for _, line := range lines[:maxLines] {
		observe.GlobalTrace("range lines[:maxLines]")
		b.WriteString(truncateLine(line, width))
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf("(+%d more lines)", len(lines)-maxLines))
	observe.GlobalTrace("return: b.String()")
	return b.String()
}
