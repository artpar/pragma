package render

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/artpar/pragma/internal/observe"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Mascot glyphs matching TS Clawd default pose (Clawd.tsx).
// 3 rows × 9 display columns.
// TS colors: clawd_body = rgb(215,119,87) / ansi:redBright
//
//	clawd_background = rgb(0,0,0) / ansi:black
var (
	mascotBody = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "167", Dark: "167"}) // ANSI 167 ≈ rgb(215,95,95) closest to rgb(215,119,87)

	mascotFace = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "167", Dark: "167"}).
			Background(lipgloss.AdaptiveColor{Light: "0", Dark: "0"}) // black background matching TS

	mascotRow1L = " ▐"        // 2 cols
	mascotRow1E = "▛███▜"     // 5 cols (with background)
	mascotRow1R = "▌ "        // 2 cols (trailing space pads row to 9)
	mascotRow2L = "▝▜"        // 2 cols (arms extend left — no leading space)
	mascotRow2B = "█████"     // 5 cols (with background)
	mascotRow2R = "▛▘"        // 2 cols → row total = 9
	mascotRow3  = "  ▘▘ ▝▝  " // 9 cols (feet with trailing spaces)
)

const mascotWidth = 9 // display columns occupied by mascot
const mascotGap = 2   // gap between mascot and text

// welcomeNameStyle is the bold tool name — bold only, no color (matches TS CondensedLogo).
var welcomeNameStyle = lipgloss.NewStyle().Bold(true)

var welcomeDim = lipgloss.NewStyle().Faint(true)

// RenderWelcome renders the condensed logo matching TS CondensedLogo.
// Always the first element in the viewport; scrolls with content.
// mcpServers lists connected MCP server names (nil/empty = no MCP line shown).
func RenderWelcome(version, modelName, provider, workspace string, mcpServers []string, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	path := DisplayPath(workspace)
	modelLine := modelName + " · " + provider
	hintLine := "Enter to send · Alt+Enter for newlines · /help for commands"

	namePart := welcomeNameStyle.Render("pragma")
	versionPart := welcomeDim.Render(" v" + version)
	line1 := namePart + versionPart
	line2 := welcomeDim.Render(modelLine)
	line3 := welcomeDim.Render(path)
	hintRendered := welcomeDim.Render(hintLine)

	var mcpText string
	if len(mcpServers) > 0 {
		observe.GlobalTrace("if: len(mcpServers) > 0")
		mcpText = fmt.Sprintf("%d MCP server", len(mcpServers))
		if len(mcpServers) > 1 {
			observe.GlobalTrace("if: len(mcpServers) > 1")
			mcpText += "s"
		}
		mcpText += ": " + strings.Join(mcpServers, ", ")
	}

	if width < 40 {
		observe.GlobalTrace("if: width < 40")
		result := "\n  " + line1 + "\n  " + line2 + "\n  " + line3
		if mcpText != "" {
			observe.GlobalTrace("if: mcpText != \"\"")
			result += "\n  " + welcomeDim.Render(mcpText)
		}
		result += "\n  " + hintRendered + "\n\n"
		observe.GlobalTrace("return: result")
		return result
	}

	textIndent := strings.Repeat(" ", mascotWidth+mascotGap)

	availWidth := width - mascotWidth - mascotGap - 1
	if availWidth > 0 && runewidth.StringWidth(path) > availWidth {
		observe.GlobalTrace("if: availWidth > 0 && runewidth.StringWidth(path) > availWidth")
		path = TruncatePath(path, availWidth)
		line3 = welcomeDim.Render(path)
	}

	if availWidth > 0 && runewidth.StringWidth(modelLine) > availWidth {
		observe.GlobalTrace("if: availWidth > 0 && runewidth.StringWidth(modelLine) > availWidth")
		modelLine = truncateMiddle(modelLine, availWidth)
		line2 = welcomeDim.Render(modelLine)
	}

	if mcpText != "" && availWidth > 0 && runewidth.StringWidth(mcpText) > availWidth {
		observe.GlobalTrace("if: mcpText != \"\" && availWidth > 0 && runewidth.StringWidth(mcpText) > availWidth")
		mcpText = truncateMiddle(mcpText, availWidth)
	}

	mRow1 := mascotBody.Render(mascotRow1L) + mascotFace.Render(mascotRow1E) + mascotBody.Render(mascotRow1R)
	mRow2 := mascotBody.Render(mascotRow2L) + mascotFace.Render(mascotRow2B) + mascotBody.Render(mascotRow2R)
	mRow3 := mascotBody.Render(mascotRow3)

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(mRow1 + "  " + line1 + "\n")
	b.WriteString(mRow2 + "  " + line2 + "\n")
	b.WriteString(mRow3 + "  " + line3 + "\n")
	if mcpText != "" {
		observe.GlobalTrace("if: mcpText != \"\"")
		b.WriteString(textIndent + welcomeDim.Render(mcpText) + "\n")
	}
	b.WriteString(textIndent + hintRendered + "\n")
	b.WriteString("\n")
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

// DisplayPath converts an absolute path to a display-friendly form.
// Uses ~/ notation for paths under $HOME, otherwise returns as-is.
func DisplayPath(absPath string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		observe.GlobalTrace("if: err != nil || home == \"\"")
		observe.GlobalTrace("return: absPath")
		return absPath
	}
	if strings.HasPrefix(absPath, home+string(filepath.Separator)) {
		observe.GlobalTrace("if: strings.HasPrefix(absPath, home+string(filepath.Separator))")
		observe.GlobalTrace("return: \"~\" + absPath[len(home):]")
		return "~" + absPath[len(home):]
	}
	if absPath == home {
		observe.GlobalTrace("if: absPath == home")
		observe.GlobalTrace("return: \"~\"")
		return "~"
	}
	observe.GlobalTrace("return: absPath")
	return absPath
}

// TruncatePath truncates a path in the middle if too long.
// Preserves first and last path segments with … in between.
func TruncatePath(path string, maxWidth int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if runewidth.StringWidth(path) <= maxWidth {
		observe.GlobalTrace("if: runewidth.StringWidth(path) <= maxWidth")
		observe.GlobalTrace("return: path")
		return path
	}

	sep := string(filepath.Separator)
	parts := strings.Split(path, sep)
	if len(parts) <= 2 {
		observe.GlobalTrace("if: len(parts) <= 2")
		observe.GlobalTrace("return: truncateMiddle(path, maxWidth)")
		return truncateMiddle(path, maxWidth)
	}

	first := parts[0]
	last := parts[len(parts)-1]

	candidate := first + sep + "…" + sep + last
	if runewidth.StringWidth(candidate) <= maxWidth {
		observe.GlobalTrace("if: runewidth.StringWidth(candidate) <= maxWidth")

		for i := len(parts) - 2; i > 0; i-- {
			observe.GlobalTrace("for: i > 0")
			next := first + sep + "…" + sep + strings.Join(parts[i:], sep)
			if runewidth.StringWidth(next) <= maxWidth {
				observe.GlobalTrace("if: runewidth.StringWidth(next) <= maxWidth")
				candidate = next
			} else {
				observe.GlobalTrace("else: runewidth.StringWidth(next) <= maxWidth")
				break
			}
		}
		observe.GlobalTrace("return: candidate")
		return candidate
	}
	observe.GlobalTrace("return: truncateMiddle(path, maxWidth)")

	return truncateMiddle(path, maxWidth)
}

// truncateMiddle truncates a string in the middle with …
func truncateMiddle(s string, maxWidth int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if maxWidth <= 1 {
		observe.GlobalTrace("if: maxWidth <= 1")
		observe.GlobalTrace("return: \"…\"")
		return "…"
	}
	w := runewidth.StringWidth(s)
	if w <= maxWidth {
		observe.GlobalTrace("if: w <= maxWidth")
		observe.GlobalTrace("return: s")
		return s
	}

	runes := []rune(s)
	half := (maxWidth - 1) / 2
	tail := maxWidth - 1 - half
	observe.GlobalTrace("return: string(runes[:half]) + \"…\" + string(runes[len(runes)-tail:])")
	return string(runes[:half]) + "…" + string(runes[len(runes)-tail:])
}
