package render

import (
	"fmt"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/observe"
	"github.com/charmbracelet/lipgloss"
)

// TeammateEntry holds a single teammate's state for rendering.
type TeammateEntry struct {
	Name              string
	TaskID            string
	Status            string // task status: "running", "completed", "failed", "cancelled", "pending"
	TokenCount        int
	ToolCount         int
	LastTool          string
	IsIdle            bool
	IdleSince         time.Time
	ShutdownRequested bool
}

var (
	teammateName     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "30", Dark: "87"})
	teammateNameBold = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "30", Dark: "87"})
	teammateDim      = lipgloss.NewStyle().Faint(true)
	teammateStop     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "124", Dark: "203"})
)

// RenderTeammateTree renders a tree of running teammates below the spinner.
// Matches TS TeammateSpinnerTree + TeammateSpinnerLine layout.
func RenderTeammateTree(entries []TeammateEntry, verbose bool, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(entries) == 0 {
		observe.GlobalTrace("if: len(entries) == 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	var b strings.Builder

	b.WriteString(ContentIndent)
	b.WriteString(teammateDim.Render("┌─ "))
	b.WriteString(teammateNameBold.Render("team-lead"))
	b.WriteByte('\n')

	for i, e := range entries {
		observe.GlobalTrace("range entries")
		isLast := i == len(entries)-1
		b.WriteString(ContentIndent)

		glyph := "├─"
		if isLast {
			observe.GlobalTrace("if: isLast")
			glyph = "└─"
		}
		b.WriteString(teammateDim.Render(glyph + " "))
		b.WriteString(teammateName.Render(e.Name))

		if width >= 60 {
			observe.GlobalTrace("if: width >= 60")
			b.WriteString(renderTeammateActivity(e))
		}

		if width >= 80 {
			observe.GlobalTrace("if: width >= 80")
			b.WriteString(renderTeammateStats(e))
		}

		b.WriteByte('\n')
	}
	observe.GlobalTrace("return: b.String()")

	return b.String()
}

// renderTeammateActivity returns the activity/idle/stopping text for a teammate.
func renderTeammateActivity(e TeammateEntry) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if e.ShutdownRequested {
		observe.GlobalTrace("if: e.ShutdownRequested")
		observe.GlobalTrace("return: \" \" + teammateStop.Render(\"[stopping]\")")
		return " " + teammateStop.Render("[stopping]")
	}
	if e.IsIdle {
		observe.GlobalTrace("if: e.IsIdle")
		dur := formatIdleDuration(e.IdleSince)
		observe.GlobalTrace("return: teammateDim.Render(\" · Idle for \" + dur)")
		return teammateDim.Render(" · Idle for " + dur)
	}
	if e.LastTool != "" {
		observe.GlobalTrace("if: e.LastTool != \"\"")
		observe.GlobalTrace("return: teammateDim.Render(\" · \" + e.LastTool + \"…\")")
		return teammateDim.Render(" · " + e.LastTool + "…")
	}
	observe.GlobalTrace("return: teammateDim.Render(\" · working…\")")
	return teammateDim.Render(" · working…")
}

// renderTeammateStats returns the token/tool count suffix.
func renderTeammateStats(e TeammateEntry) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var parts []string
	if e.ToolCount > 0 {
		observe.GlobalTrace("if: e.ToolCount > 0")
		parts = append(parts, fmt.Sprintf("%d tools", e.ToolCount))
	}
	if e.TokenCount > 0 {
		observe.GlobalTrace("if: e.TokenCount > 0")
		parts = append(parts, formatTeammateTokens(e.TokenCount)+" tokens")
	}
	if len(parts) == 0 {
		observe.GlobalTrace("if: len(parts) == 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	observe.GlobalTrace("return: teammateDim.Render(\" · \" + strings.Join(parts, \" · \"))")
	return teammateDim.Render(" · " + strings.Join(parts, " · "))
}

// formatIdleDuration formats the idle duration as "Xs", "Xm Ys", etc.
func formatIdleDuration(since time.Time) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if since.IsZero() {
		observe.GlobalTrace("if: since.IsZero()")
		observe.GlobalTrace("return: \"0s\"")
		return "0s"
	}
	d := time.Since(since)
	sec := int(d.Seconds())
	if sec < 60 {
		observe.GlobalTrace("if: sec < 60")
		observe.GlobalTrace("return: fmt.Sprintf(\"%ds\", sec)")
		return fmt.Sprintf("%ds", sec)
	}
	min := sec / 60
	sec = sec % 60
	if sec > 0 {
		observe.GlobalTrace("if: sec > 0")
		observe.GlobalTrace("return: fmt.Sprintf(\"%dm %ds\", min, sec)")
		return fmt.Sprintf("%dm %ds", min, sec)
	}
	observe.GlobalTrace("return: fmt.Sprintf(\"%dm\", min)")
	return fmt.Sprintf("%dm", min)
}

// formatTeammateTokens formats a token count for display (same as toolbar).
func formatTeammateTokens(n int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if n >= 1_000_000 {
		observe.GlobalTrace("if: n >= 1_000_000")
		s := fmt.Sprintf("%.1fm", float64(n)/1_000_000)
		observe.GlobalTrace("return: stripDotZero(s)")
		return stripDotZero(s)
	}
	if n >= 1_000 {
		observe.GlobalTrace("if: n >= 1_000")
		s := fmt.Sprintf("%.1fk", float64(n)/1_000)
		observe.GlobalTrace("return: stripDotZero(s)")
		return stripDotZero(s)
	}
	observe.GlobalTrace("return: fmt.Sprintf(\"%d\", n)")
	return fmt.Sprintf("%d", n)
}

// TeammateStatusText returns a plain status string for a teammate entry.
// Used by the teams dialog for per-entry status display.
func TeammateStatusText(e TeammateEntry) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch e.Status {
	case "completed":
		observe.GlobalTrace("case: \"completed\"")
		return "done"
	case "failed":
		observe.GlobalTrace("case: \"failed\"")
		return "failed"
	case "cancelled":
		observe.GlobalTrace("case: \"cancelled\"")
		return "cancelled"
	}
	if e.ShutdownRequested {
		observe.GlobalTrace("if: e.ShutdownRequested")
		observe.GlobalTrace("return: \"stopping\"")
		return "stopping"
	}
	if e.IsIdle {
		observe.GlobalTrace("if: e.IsIdle")
		observe.GlobalTrace("return: \"Idle for \" + formatIdleDuration(e.IdleSince)")
		return "Idle for " + formatIdleDuration(e.IdleSince)
	}
	if e.LastTool != "" {
		observe.GlobalTrace("if: e.LastTool != \"\"")
		observe.GlobalTrace("return: e.LastTool")
		return e.LastTool
	}
	observe.GlobalTrace("return: \"running\"")
	return "running"
}

func stripDotZero(s string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(s) >= 4 && s[len(s)-3:len(s)-1] == ".0" {
		observe.GlobalTrace("if: len(s) >= 4 && s[len(s)-3:len(s)-1] == \".0\"")
		observe.GlobalTrace("return: s[:len(s)-3] + s[len(s)-1:]")
		return s[:len(s)-3] + s[len(s)-1:]
	}
	observe.GlobalTrace("return: s")
	return s
}
