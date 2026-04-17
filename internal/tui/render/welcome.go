package render

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Mascot glyphs matching TS Clawd default pose (Clawd.tsx).
// 3 rows × 9 display columns.
// TS colors: clawd_body = rgb(215,119,87) / ansi:redBright
//            clawd_background = rgb(0,0,0) / ansi:black
var (
	mascotBody = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "167", Dark: "167"}) // ANSI 167 ≈ rgb(215,95,95) closest to rgb(215,119,87)

	mascotFace = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "167", Dark: "167"}).
			Background(lipgloss.AdaptiveColor{Light: "0", Dark: "0"}) // black background matching TS

	mascotRow1L = " ▐"    // 2 cols
	mascotRow1E = "▛███▜" // 5 cols (with background)
	mascotRow1R = "▌ "    // 2 cols (trailing space pads row to 9)
	mascotRow2L = "▝▜"    // 2 cols (arms extend left — no leading space)
	mascotRow2B = "█████" // 5 cols (with background)
	mascotRow2R = "▛▘"    // 2 cols → row total = 9
	mascotRow3  = "  ▘▘ ▝▝  " // 9 cols (feet with trailing spaces)
)

const mascotWidth = 9 // display columns occupied by mascot
const mascotGap = 2   // gap between mascot and text

// welcomeNameStyle is the bold tool name — bold only, no color (matches TS CondensedLogo).
var welcomeNameStyle = lipgloss.NewStyle().Bold(true)

var welcomeDim = lipgloss.NewStyle().Faint(true)

// RenderWelcome renders the condensed logo matching TS CondensedLogo.
// Always the first element in the viewport; scrolls with content.
func RenderWelcome(version, modelName, provider, workspace string, width int) string {
	path := DisplayPath(workspace)
	modelLine := modelName + " · " + provider
	hintLine := "Enter to send · Alt+Enter for newlines · /help for commands"

	namePart := welcomeNameStyle.Render("pragma")
	versionPart := welcomeDim.Render(" v" + version)
	line1 := namePart + versionPart
	line2 := welcomeDim.Render(modelLine)
	line3 := welcomeDim.Render(path)
	line4 := welcomeDim.Render(hintLine)

	// Narrow terminal: text-only with 2-space indent
	if width < 40 {
		return "\n  " + line1 + "\n  " + line2 + "\n  " + line3 + "\n  " + line4 + "\n\n"
	}

	// Wide terminal: mascot on left, text on right
	textIndent := strings.Repeat(" ", mascotWidth+mascotGap)

	// Truncate workspace path if needed
	availWidth := width - mascotWidth - mascotGap - 1
	if availWidth > 0 && runewidth.StringWidth(path) > availWidth {
		path = TruncatePath(path, availWidth)
		line3 = welcomeDim.Render(path)
	}

	// Truncate model line if needed
	if availWidth > 0 && runewidth.StringWidth(modelLine) > availWidth {
		modelLine = truncateMiddle(modelLine, availWidth)
		line2 = welcomeDim.Render(modelLine)
	}

	// Build mascot rows with styled glyphs
	mRow1 := mascotBody.Render(mascotRow1L) + mascotFace.Render(mascotRow1E) + mascotBody.Render(mascotRow1R)
	mRow2 := mascotBody.Render(mascotRow2L) + mascotFace.Render(mascotRow2B) + mascotBody.Render(mascotRow2R)
	mRow3 := mascotBody.Render(mascotRow3)

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(mRow1 + "  " + line1 + "\n")
	b.WriteString(mRow2 + "  " + line2 + "\n")
	b.WriteString(mRow3 + "  " + line3 + "\n")
	b.WriteString(textIndent + line4 + "\n")
	b.WriteString("\n")
	return b.String()
}

// DisplayPath converts an absolute path to a display-friendly form.
// Uses ~/ notation for paths under $HOME, otherwise returns as-is.
func DisplayPath(absPath string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return absPath
	}
	if strings.HasPrefix(absPath, home+string(filepath.Separator)) {
		return "~" + absPath[len(home):]
	}
	if absPath == home {
		return "~"
	}
	return absPath
}

// TruncatePath truncates a path in the middle if too long.
// Preserves first and last path segments with … in between.
func TruncatePath(path string, maxWidth int) string {
	if runewidth.StringWidth(path) <= maxWidth {
		return path
	}

	sep := string(filepath.Separator)
	parts := strings.Split(path, sep)
	if len(parts) <= 2 {
		return truncateMiddle(path, maxWidth)
	}

	first := parts[0]
	last := parts[len(parts)-1]

	// Try: first/…/last
	candidate := first + sep + "…" + sep + last
	if runewidth.StringWidth(candidate) <= maxWidth {
		// Try to include more trailing segments
		for i := len(parts) - 2; i > 0; i-- {
			next := first + sep + "…" + sep + strings.Join(parts[i:], sep)
			if runewidth.StringWidth(next) <= maxWidth {
				candidate = next
			} else {
				break
			}
		}
		return candidate
	}

	return truncateMiddle(path, maxWidth)
}

// truncateMiddle truncates a string in the middle with …
func truncateMiddle(s string, maxWidth int) string {
	if maxWidth <= 1 {
		return "…"
	}
	w := runewidth.StringWidth(s)
	if w <= maxWidth {
		return s
	}
	// Split roughly in half
	runes := []rune(s)
	half := (maxWidth - 1) / 2
	tail := maxWidth - 1 - half
	return string(runes[:half]) + "…" + string(runes[len(runes)-tail:])
}
