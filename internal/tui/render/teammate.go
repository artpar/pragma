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
	teammateName    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "30", Dark: "87"})
	teammateNameBold = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "30", Dark: "87"})
	teammateDim     = lipgloss.NewStyle().Faint(true)
	teammateStop    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "124", Dark: "203"})
)

// RenderTeammateTree renders a tree of running teammates below the spinner.
// Matches TS TeammateSpinnerTree + TeammateSpinnerLine layout.
func RenderTeammateTree(entries []TeammateEntry, verbose bool, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(entries) == 0 {
		return ""
	}

	var b strings.Builder

	// Leader row.
	b.WriteString(ContentIndent)
	b.WriteString(teammateDim.Render("┌─ "))
	b.WriteString(teammateNameBold.Render("team-lead"))
	b.WriteByte('\n')

	// Teammate rows.
	for i, e := range entries {
		isLast := i == len(entries)-1
		b.WriteString(ContentIndent)

		glyph := "├─"
		if isLast {
			glyph = "└─"
		}
		b.WriteString(teammateDim.Render(glyph + " "))
		b.WriteString(teammateName.Render(e.Name))

		if width >= 60 {
			b.WriteString(renderTeammateActivity(e))
		}

		if width >= 80 {
			b.WriteString(renderTeammateStats(e))
		}

		b.WriteByte('\n')
	}

	return b.String()
}

// renderTeammateActivity returns the activity/idle/stopping text for a teammate.
func renderTeammateActivity(e TeammateEntry) string {
	if e.ShutdownRequested {
		return " " + teammateStop.Render("[stopping]")
	}
	if e.IsIdle {
		dur := formatIdleDuration(e.IdleSince)
		return teammateDim.Render(" · Idle for " + dur)
	}
	if e.LastTool != "" {
		return teammateDim.Render(" · " + e.LastTool + "…")
	}
	return teammateDim.Render(" · working…")
}

// renderTeammateStats returns the token/tool count suffix.
func renderTeammateStats(e TeammateEntry) string {
	var parts []string
	if e.ToolCount > 0 {
		parts = append(parts, fmt.Sprintf("%d tools", e.ToolCount))
	}
	if e.TokenCount > 0 {
		parts = append(parts, formatTeammateTokens(e.TokenCount)+" tokens")
	}
	if len(parts) == 0 {
		return ""
	}
	return teammateDim.Render(" · " + strings.Join(parts, " · "))
}

// formatIdleDuration formats the idle duration as "Xs", "Xm Ys", etc.
func formatIdleDuration(since time.Time) string {
	if since.IsZero() {
		return "0s"
	}
	d := time.Since(since)
	sec := int(d.Seconds())
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	min := sec / 60
	sec = sec % 60
	if sec > 0 {
		return fmt.Sprintf("%dm %ds", min, sec)
	}
	return fmt.Sprintf("%dm", min)
}

// formatTeammateTokens formats a token count for display (same as toolbar).
func formatTeammateTokens(n int) string {
	if n >= 1_000_000 {
		s := fmt.Sprintf("%.1fm", float64(n)/1_000_000)
		return stripDotZero(s)
	}
	if n >= 1_000 {
		s := fmt.Sprintf("%.1fk", float64(n)/1_000)
		return stripDotZero(s)
	}
	return fmt.Sprintf("%d", n)
}

// TeammateStatusText returns a plain status string for a teammate entry.
// Used by the teams dialog for per-entry status display.
func TeammateStatusText(e TeammateEntry) string {
	switch e.Status {
	case "completed":
		return "done"
	case "failed":
		return "failed"
	case "cancelled":
		return "cancelled"
	}
	if e.ShutdownRequested {
		return "stopping"
	}
	if e.IsIdle {
		return "Idle for " + formatIdleDuration(e.IdleSince)
	}
	if e.LastTool != "" {
		return e.LastTool
	}
	return "running"
}

func stripDotZero(s string) string {
	if len(s) >= 4 && s[len(s)-3:len(s)-1] == ".0" {
		return s[:len(s)-3] + s[len(s)-1:]
	}
	return s
}
