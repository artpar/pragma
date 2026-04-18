package render

import (
	"encoding/json"
	"fmt"
	"strconv"
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
// display carries optional TUI-only content from InvokeResult.Display (e.g., unified diff).
// verbose controls expanded (full output) vs compact (truncated summary) rendering.
type ToolRenderer func(input json.RawMessage, content string, isError bool, width int, display string, verbose bool) string

// toolRenderers maps tool names to their specific renderer.
var toolRenderers = map[string]ToolRenderer{
	"Bash":            renderBash,
	"Read":            renderRead,
	"Edit":            renderEdit,
	"Write":           renderWrite,
	"Grep":            renderGrep,
	"Glob":            renderGlob,
	"Agent":           renderAgent,
	"AskUserQuestion": renderAskResult,
	"LifecycleRun":    renderLifecycleRun,
}

// RenderToolOutput dispatches to a tool-specific renderer or generic fallback.
func RenderToolOutput(name string, input json.RawMessage, content string, isError bool, width int, display string, verbose bool) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if renderer, ok := toolRenderers[name]; ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: renderer(input, content, isError, width, display, verbose)")
		return renderer(input, content, isError, width, display, verbose)
	}
	observe.GlobalTrace("return: WrapWithBracket(content, isError, width, verbose)")
	return WrapWithBracket(content, isError, width, verbose)
}

// appendExpandHint appends dim "(ctrl+o to expand)" hint to a builder.
func appendExpandHint(b *strings.Builder) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	b.WriteString(ContentIndent)
	b.WriteString(dimText.Render("(ctrl+o to expand)"))
	b.WriteString("\n")
}

// renderBash renders Bash tool output.
// Shows last 5 lines (matching TS non-verbose ShellProgressMessage), with styled
// exit code/timeout when present in the Display field.
// The command itself is already shown in the tool call line (⏺ Bash(cmd)).
func renderBash(_ json.RawMessage, content string, isError bool, width int, display string, verbose bool) string {
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

	// Separate trailing status line ("Exit code N" / "Command timed out...")
	// for styled rendering below.
	var statusLine string
	if len(lines) > 0 {
		observe.GlobalTrace("if: len(lines) > 0")
		last := lines[len(lines)-1]
		if strings.HasPrefix(last, "Exit code ") || strings.HasPrefix(last, "Command timed out") {
			observe.GlobalTrace("if: strings.HasPrefix(last, \"Exit code \") || strings.HasPrefix(last, \"Command tim...")
			statusLine = last
			lines = lines[:len(lines)-1]
		}
	}

	truncated := false
	if !verbose {
		observe.GlobalTrace("if: !verbose")
		maxLines := 5
		if len(lines) > maxLines {
			observe.GlobalTrace("if: len(lines) > maxLines")
			truncated = true
			skipped := len(lines) - maxLines
			b.WriteString(ContentIndent)
			b.WriteString(dimText.Render(fmt.Sprintf("(%d lines hidden)", skipped)))
			b.WriteString("\n")
			lines = lines[skipped:]
		}
	}

	for _, line := range lines {
		observe.GlobalTrace("range lines")
		b.WriteString(ContentIndent)
		b.WriteString(truncateLine(line, width-len(ContentIndent)))
		b.WriteString("\n")
	}

	if statusLine != "" {
		observe.GlobalTrace("if: statusLine != \"\"")
		exitCode := parseBashExitCode(display)
		b.WriteString(ContentIndent)
		if exitCode == 2 && strings.Contains(content, "blocked") {
			observe.GlobalTrace("if: exitCode == 2 && strings.Contains(content, \"blocked\")")

			b.WriteString(dimText.Render(statusLine))
		} else {
			observe.GlobalTrace("else: exitCode == 2 && strings.Contains(content, \"blocked\")")

			b.WriteString(errBold.Render(statusLine))
		}
		b.WriteString("\n")
	}

	if truncated && !verbose {
		observe.GlobalTrace("if: truncated && !verbose")
		appendExpandHint(&b)
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

// parseBashExitCode extracts exit code from display metadata.
// Display format: "exit_code:N" or "timeout:N".
func parseBashExitCode(display string) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if strings.HasPrefix(display, "exit_code:") {
		observe.GlobalTrace("if: strings.HasPrefix(display, \"exit_code:\")")
		code, err := strconv.Atoi(display[len("exit_code:"):])
		if err == nil {
			observe.GlobalTrace("if: err == nil")
			observe.GlobalTrace("return: code")
			return code
		}
	}
	observe.GlobalTrace("return: -1")
	return -1
}

// renderRead renders FileRead tool output as a compact "Read N lines" summary.
// The file path is already shown in the tool call line (⏺ Read(path)).
func renderRead(input json.RawMessage, content string, isError bool, width int, _ string, verbose bool) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if isError {
		observe.GlobalTrace("if: isError")
		observe.GlobalTrace("return: WrapWithBracket(content, true, width, verbose)")
		return WrapWithBracket(content, true, width, verbose)
	}

	var params struct {
		Offset int `json:"offset"`
		Limit  int `json:"limit"`
	}
	json.Unmarshal(input, &params)

	lineCount := strings.Count(strings.TrimRight(content, "\n"), "\n") + 1
	if strings.TrimSpace(content) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(content) == \"\"")
		lineCount = 0
	}

	noun := "lines"
	if lineCount == 1 {
		observe.GlobalTrace("if: lineCount == 1")
		noun = "line"
	}

	summary := fmt.Sprintf("Read %d %s", lineCount, noun)
	if params.Offset > 0 {
		observe.GlobalTrace("if: params.Offset > 0")
		summary += fmt.Sprintf(" (from line %d)", params.Offset+1)
	}

	if !verbose {
		observe.GlobalTrace("if: !verbose")
		var b strings.Builder
		b.WriteString(bracketDim.Render(BracketPrefix) + dimText.Render(summary))
		b.WriteString("\n")
		appendExpandHint(&b)
		observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")
		return strings.TrimRight(b.String(), "\n")
	}

	// Verbose: show full content with line numbers (size guard: max 2000 lines per #46190)
	var b strings.Builder
	b.WriteString(bracketDim.Render(BracketPrefix) + dimText.Render(summary))
	b.WriteString("\n")

	lines := strings.Split(content, "\n")
	maxVerboseLines := 2000
	showLines := lines
	if len(showLines) > maxVerboseLines {
		observe.GlobalTrace("if: len(showLines) > maxVerboseLines")
		showLines = showLines[:maxVerboseLines]
	}
	startLine := params.Offset + 1
	for i, line := range showLines {
		observe.GlobalTrace("range showLines")
		b.WriteString(ContentIndent)
		b.WriteString(diffGutter.Render(fmt.Sprintf("%4d ", startLine+i)))
		b.WriteString(truncateLine(line, width-len(ContentIndent)-5))
		b.WriteString("\n")
	}
	if len(lines) > maxVerboseLines {
		observe.GlobalTrace("if: len(lines) > maxVerboseLines")
		b.WriteString(ContentIndent)
		b.WriteString(dimText.Render(fmt.Sprintf("(+%d more lines beyond display limit)", len(lines)-maxVerboseLines)))
		b.WriteString("\n")
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

// renderEdit renders FileEdit tool output as a unified diff.
// When display contains a unified diff (from InvokeResult.Display), renders it with
// line numbers, context, and color. Falls back to simple -/+ from input params.
func renderEdit(input json.RawMessage, content string, isError bool, width int, display string, _ bool) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if isError {
		observe.GlobalTrace("if: isError")
		observe.GlobalTrace("return: WrapWithBracket(content, true, width, false)")
		return WrapWithBracket(content, true, width, false)
	}

	var params struct {
		FilePath  string `json:"file_path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	json.Unmarshal(input, &params)

	var b strings.Builder

	if params.FilePath != "" {
		observe.GlobalTrace("if: params.FilePath != \"\"")
		b.WriteString(bracketDim.Render(BracketPrefix))
		b.WriteString(fileHeader.Render(params.FilePath))
		b.WriteString("\n")
	}

	if display != "" {
		observe.GlobalTrace("if: display != \"\"")
		b.WriteString(renderUnifiedDiff(display, width))
		b.WriteString("\n")
		observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")
		return strings.TrimRight(b.String(), "\n")
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
		b.WriteString(WrapWithBracket(content, false, width, false))
		b.WriteString("\n")
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

// renderUnifiedDiff parses and renders a unified diff string with line numbers and colors.
func renderUnifiedDiff(diff string, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	var b strings.Builder
	lines := strings.Split(diff, "\n")

	var oldLine, newLine int
	hunkIdx := 0

	for _, line := range lines {
		observe.GlobalTrace("range lines")
		if line == "..." {
			observe.GlobalTrace("if: line == \"...\"")

			b.WriteString(ContentIndent)
			b.WriteString(dimText.Render("..."))
			b.WriteString("\n")
			hunkIdx++
			continue
		}

		if strings.HasPrefix(line, "@@") {
			observe.GlobalTrace("if: strings.HasPrefix(line, \"@@\")")

			if hunkIdx > 0 && !strings.HasSuffix(b.String(), "...\n") {
				observe.GlobalTrace("if: hunkIdx > 0 && !strings.HasSuffix(b.String(), \"...\\n\")")

			}
			n, _ := fmt.Sscanf(line, "@@ -%d,%*d +%d,%*d @@", &oldLine, &newLine)
			if n < 2 {
				observe.GlobalTrace("if: n < 2")

				fmt.Sscanf(line, "@@ -%d,0 +%d,%*d @@", &oldLine, &newLine)
			}
			continue
		}

		gutterWidth := 11
		contentWidth := width - len(ContentIndent) - gutterWidth - 2
		if contentWidth < 10 {
			observe.GlobalTrace("if: contentWidth < 10")
			contentWidth = 10
		}

		switch {
		case strings.HasPrefix(line, " "):
			observe.GlobalTrace("case: strings.HasPrefix(line, \" \")")

			code := line[1:]
			gutter := diffGutter.Render(fmt.Sprintf("%4d %4d ", oldLine, newLine))
			b.WriteString(ContentIndent)
			b.WriteString(gutter)
			b.WriteString(dimText.Render(" " + truncateLine(code, contentWidth)))
			b.WriteString("\n")
			oldLine++
			newLine++

		case strings.HasPrefix(line, "-"):
			observe.GlobalTrace("case: strings.HasPrefix(line, \"-\")")

			code := line[1:]
			gutter := diffGutter.Render(fmt.Sprintf("%4d      ", oldLine))
			b.WriteString(ContentIndent)
			b.WriteString(gutter)
			b.WriteString(diffRemove.Render("-" + truncateLine(code, contentWidth)))
			b.WriteString("\n")
			oldLine++

		case strings.HasPrefix(line, "+"):
			observe.GlobalTrace("case: strings.HasPrefix(line, \"+\")")

			code := line[1:]
			gutter := diffGutter.Render(fmt.Sprintf("     %4d ", newLine))
			b.WriteString(ContentIndent)
			b.WriteString(gutter)
			b.WriteString(diffAdd.Render("+" + truncateLine(code, contentWidth)))
			b.WriteString("\n")
			newLine++
		}
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

// renderWrite renders FileWrite tool output.
// When display contains a unified diff (from InvokeResult.Display), renders it with
// line numbers, context, and color. Falls back to line count summary.
func renderWrite(input json.RawMessage, content string, isError bool, width int, display string, _ bool) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if isError {
		observe.GlobalTrace("if: isError")
		observe.GlobalTrace("return: WrapWithBracket(content, true, width, false)")
		return WrapWithBracket(content, true, width, false)
	}

	var params struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	json.Unmarshal(input, &params)

	var b strings.Builder

	if params.FilePath != "" {
		observe.GlobalTrace("if: params.FilePath != \"\"")
		b.WriteString(bracketDim.Render(BracketPrefix))
		b.WriteString(fileHeader.Render(params.FilePath))
		b.WriteString("\n")
	}

	if display != "" {
		observe.GlobalTrace("if: display != \"\"")
		isCreate := strings.Contains(content, "created")
		if isCreate {
			observe.GlobalTrace("if: isCreate")

			numLines := countContentLines(params.Content)
			b.WriteString(bracketDim.Render(BracketPrefix))
			b.WriteString(dimText.Render(fmt.Sprintf("Wrote %d lines", numLines)))
			b.WriteString("\n")
		} else {
			observe.GlobalTrace("else: isCreate")

			added, removed := countDiffLines(display)
			b.WriteString(bracketDim.Render(BracketPrefix))
			b.WriteString(dimText.Render(formatChangeSummary(added, removed)))
			b.WriteString("\n")
		}
		b.WriteString(renderUnifiedDiff(display, width))
		b.WriteString("\n")
		observe.GlobalTrace("return: rendered display diff")
		observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")
		return strings.TrimRight(b.String(), "\n")
	}

	numLines := countContentLines(params.Content)
	b.WriteString(bracketDim.Render(BracketPrefix))
	b.WriteString(dimText.Render(fmt.Sprintf("Wrote %d lines", numLines)))
	observe.GlobalTrace("return: fallback line count summary")
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")
	return strings.TrimRight(b.String(), "\n")
}

// countContentLines counts visible lines in file content.
// A trailing newline is treated as a line terminator, matching editor line numbering.
func countContentLines(content string) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if content == "" {
		observe.GlobalTrace("if: content == \"\"")
		observe.GlobalTrace("return: 0")
		return 0
	}
	n := strings.Count(content, "\n") + 1
	if strings.HasSuffix(content, "\n") {
		observe.GlobalTrace("if: strings.HasSuffix(content, \"\\n\")")
		n--
	}
	observe.GlobalTrace("return: n")
	return n
}

// countDiffLines counts added (+) and removed (-) lines in a unified diff string.
func countDiffLines(diff string) (added, removed int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, line := range strings.Split(diff, "\n") {
		observe.GlobalTrace("range strings.Split(diff, \"\\n\")")
		if strings.HasPrefix(line, "+") {
			observe.GlobalTrace("if: strings.HasPrefix(line, \"+\")")
			added++
		} else if strings.HasPrefix(line, "-") {
			observe.GlobalTrace("else-if: strings.HasPrefix(line, \"-\")")
			removed++
		}
	}
	return
}

// formatChangeSummary builds "Added N lines, removed N lines" string.
func formatChangeSummary(added, removed int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var parts []string
	if added > 0 {
		observe.GlobalTrace("if: added > 0")
		noun := "line"
		if added > 1 {
			observe.GlobalTrace("if: added > 1")
			noun = "lines"
		}
		parts = append(parts, fmt.Sprintf("Added %d %s", added, noun))
	}
	if removed > 0 {
		observe.GlobalTrace("if: removed > 0")
		noun := "line"
		if removed > 1 {
			observe.GlobalTrace("if: removed > 1")
			noun = "lines"
		}
		prefix := "Removed"
		if added > 0 {
			observe.GlobalTrace("if: added > 0")
			prefix = "removed"
		}
		parts = append(parts, fmt.Sprintf("%s %d %s", prefix, removed, noun))
	}
	if len(parts) == 0 {
		observe.GlobalTrace("if: len(parts) == 0")
		observe.GlobalTrace("return: \"No changes\"")
		return "No changes"
	}
	observe.GlobalTrace("return: strings.Join(parts, \", \")")
	return strings.Join(parts, ", ")
}

// renderGrep renders Grep tool search results.
// Shows summary + file list.
func renderGrep(_ json.RawMessage, content string, isError bool, width int, _ string, verbose bool) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if isError {
		observe.GlobalTrace("if: isError")
		observe.GlobalTrace("return: WrapWithBracket(content, true, width, verbose)")
		return WrapWithBracket(content, true, width, verbose)
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
	if verbose {
		observe.GlobalTrace("if: verbose")
		maxFiles = fileCount
	}
	truncated := false
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
			truncated = true
			remaining := fileCount - shown
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

	if truncated && !verbose {
		observe.GlobalTrace("if: truncated && !verbose")
		appendExpandHint(&b)
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

// renderGlob renders Glob tool file listing results.
func renderGlob(_ json.RawMessage, content string, isError bool, width int, _ string, verbose bool) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if isError {
		observe.GlobalTrace("if: isError")
		observe.GlobalTrace("return: WrapWithBracket(content, true, width, verbose)")
		return WrapWithBracket(content, true, width, verbose)
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
	if verbose {
		observe.GlobalTrace("if: verbose")
		maxFiles = fileCount
	}
	truncated := false
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
			truncated = true
			remaining := fileCount - shown
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

	if truncated && !verbose {
		observe.GlobalTrace("if: truncated && !verbose")
		appendExpandHint(&b)
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

// renderAgent renders Agent tool results with diamond glyphs.
func renderAgent(input json.RawMessage, content string, isError bool, width int, _ string, verbose bool) string {
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
		total := len(lines)
		truncated := false
		if !verbose {
			observe.GlobalTrace("if: !verbose")
			maxLines := 5
			if len(lines) > maxLines {
				observe.GlobalTrace("if: len(lines) > maxLines")
				truncated = true
				lines = lines[:maxLines]
			}
		}
		for _, line := range lines {
			observe.GlobalTrace("range lines")
			b.WriteString(ContentIndent)
			b.WriteString(truncateLine(line, width-len(ContentIndent)))
			b.WriteString("\n")
		}
		if truncated {
			observe.GlobalTrace("if: truncated")
			b.WriteString(ContentIndent)
			b.WriteString(dimText.Render(fmt.Sprintf("(+%d more lines)", total-len(lines))))
			b.WriteString("\n")
			appendExpandHint(&b)
		}
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

// renderAskResult renders AskUserQuestion tool result.
// Shows the user's answers in "Q"="A" format, matching TS AskUserQuestionResultMessage.
func renderAskResult(_ json.RawMessage, content string, isError bool, width int, _ string, _ bool) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if isError {
		observe.GlobalTrace("if: isError")
		observe.GlobalTrace("return: WrapWithBracket(content, true, width, false)")
		return WrapWithBracket(content, true, width, false)
	}

	// Content format from tool: "User has answered your questions: "Q1"="A1", "Q2"="A2". ..."
	// or plain answer string for legacy questions
	var b strings.Builder

	if strings.HasPrefix(content, "User has answered") {
		observe.GlobalTrace("if: strings.HasPrefix(content, \"User has answered\")")
		b.WriteString(bracketDim.Render(BracketPrefix))
		b.WriteString(dimText.Render("User answered questions"))
		b.WriteString("\n")

		answersStart := strings.Index(content, ": ")
		answersEnd := strings.Index(content, ". You can")
		if answersStart >= 0 && answersEnd > answersStart {
			observe.GlobalTrace("if: answersStart >= 0 && answersEnd > answersStart")
			answersPart := content[answersStart+2 : answersEnd]

			pairs := splitAnswerPairs(answersPart)
			for _, pair := range pairs {
				observe.GlobalTrace("range pairs")
				b.WriteString(ContentIndent)
				b.WriteString(truncateLine(pair, width-len(ContentIndent)))
				b.WriteString("\n")
			}
		}
	} else {
		observe.GlobalTrace("else: strings.HasPrefix(content, \"User has answered\")")

		b.WriteString(bracketDim.Render(BracketPrefix))
		b.WriteString(truncateLine(content, width-len(BracketPrefix)))
		b.WriteString("\n")
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

// splitAnswerPairs splits "Q1"="A1", "Q2"="A2" respecting Go %q escaped quotes.
// Handles \" inside quoted strings correctly (e.g., "Which \"library\"?"="React").
func splitAnswerPairs(s string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var pairs []string
	var current strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		observe.GlobalTrace("for: i < len(s)")
		ch := s[i]
		if ch == '\\' && inQuote && i+1 < len(s) {
			observe.GlobalTrace("if: ch == '\\\\' && inQuote && i+1 < len(s)")

			current.WriteByte(ch)
			i++
			current.WriteByte(s[i])
		} else if ch == '"' {
			observe.GlobalTrace("else-if: ch == '\"'")
			inQuote = !inQuote
			current.WriteByte(ch)
		} else if ch == ',' && !inQuote {
			pair := strings.TrimSpace(current.String())
			if pair != "" {
				pairs = append(pairs, pair)
			}
			current.Reset()
		} else {
			current.WriteByte(ch)
		}
	}
	pair := strings.TrimSpace(current.String())
	if pair != "" {
		observe.GlobalTrace("if: pair != \"\"")
		pairs = append(pairs, pair)
	}
	observe.GlobalTrace("return: pairs")
	return pairs
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
