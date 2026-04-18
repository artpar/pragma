package tui

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/tui/render"
)

// toolbar renders a single-line status bar at the bottom of the viewport.
// Layout matches TS reference PromptInputFooter + StatusLine:
//
//	⏺ model · provider · 5m · $0.0042 · 1.5k in / 2.3k out / 500 cache (12% ctx)    ready
type toolbar struct {
	modelName     string
	provider      string
	workspace     string
	totalCost     float64
	status        string
	inputTokens   int
	outputTokens  int
	cacheTokens   int // combined cache creation + cache read
	contextSize      int
	latestContextFill int // latest request's actual context window fill
	startTime        time.Time // session start for elapsed display
	teammateCount    int       // number of running teammates
}

func newToolbar(modelName, provider, workspace string, startTime time.Time) toolbar {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if startTime.IsZero() {
		startTime = time.Now()
	}
	observe.GlobalTrace("return: toolbar{\n\tmodelName:\tmodelName,\n\tprovider:\tprovider,\n\tworkspace:\tfilepath.Bas...")
	return toolbar{
		modelName: modelName,
		provider:  provider,
		workspace: filepath.Base(workspace),
		status:    "ready",
		startTime: startTime,
	}
}

// View renders the toolbar at the given width.
func (t toolbar) View(width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	left := fmt.Sprintf(" %s %s · %s", render.BlackCircle, t.modelName, t.provider)

	if width >= 100 && t.workspace != "" {
		observe.GlobalTrace("if: width >= 100 && t.workspace != \"\"")
		left += " · " + t.workspace
	}

	elapsed := formatDuration(time.Since(t.startTime))
	left += " · " + elapsed + " · " + formatCost(t.totalCost)

	if width >= 60 && t.contextSize > 0 {
		observe.GlobalTrace("if: width >= 60 && t.contextSize > 0")

		pct := float64(t.latestContextFill) / float64(t.contextSize) * 100

		tokenStr := fmt.Sprintf(" · %s in / %s out",
			formatTokens(t.inputTokens),
			formatTokens(t.outputTokens))
		if t.cacheTokens > 0 {
			observe.GlobalTrace("if: t.cacheTokens > 0")
			tokenStr += " / " + formatTokens(t.cacheTokens) + " cache"
		}
		tokenStr += fmt.Sprintf(" (%d%% ctx)", int(pct))

		tokenStyled := t.styleTokenStr(tokenStr, pct)
		left += tokenStyled
	}

	if t.teammateCount > 0 {
		observe.GlobalTrace("if: t.teammateCount > 0")
		tmStyle := lipgloss.NewStyle().Faint(true)
		left += tmStyle.Render(fmt.Sprintf(" · %d teammates", t.teammateCount))
	}

	right := fmt.Sprintf(" %s ", t.status)
	styledRight := statusActiveStyle.Render(right)

	gap := max(width-lipgloss.Width(left)-len(right), 0)
	bar := left + strings.Repeat(" ", gap) + styledRight
	observe.GlobalTrace("return: statusBarStyle.Render(bar)")

	return statusBarStyle.Render(bar)
}

// styleTokenStr applies color coding based on context usage percentage.
func (t toolbar) styleTokenStr(s string, pct float64) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch {
	case pct >= 80:
		observe.GlobalTrace("case: pct >= 80")
		return statusTokensRedStyle.Render(s)
	case pct >= 50:
		observe.GlobalTrace("case: pct >= 50")
		return statusTokensYellowStyle.Render(s)
	default:
		observe.GlobalTrace("default")
		return statusTokensGreenStyle.Render(s)
	}
}

// SetModel updates the displayed model name (e.g. after /model switch).
func (t *toolbar) SetModel(name string) {
	t.modelName = name
}

// SetStatus updates the status text.
func (t *toolbar) SetStatus(status string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	t.status = status
}

// UpdateCost sets the total cost.
func (t *toolbar) UpdateCost(cost float64) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	t.totalCost = cost
}

// UpdateTokens updates the token display values.
// latestContextFill is the most recent API request's InputTokens + CacheReadInputTokens,
// representing how full the context window actually is right now.
func (t *toolbar) UpdateTokens(input, output, cache, contextSize, latestContextFill int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	t.inputTokens = input
	t.outputTokens = output
	t.cacheTokens = cache
	t.contextSize = contextSize
	t.latestContextFill = latestContextFill
}

// CostSummary returns a one-line session summary for display on exit.
func (t toolbar) CostSummary() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	summary := fmt.Sprintf("Session cost: %s · Duration: %s · Tokens: %s in / %s out",
		formatCost(t.totalCost),
		formatDuration(time.Since(t.startTime)),
		formatTokens(t.inputTokens),
		formatTokens(t.outputTokens))
	if t.cacheTokens > 0 {
		observe.GlobalTrace("if: t.cacheTokens > 0")
		summary += " / " + formatTokens(t.cacheTokens) + " cache"
	}
	observe.GlobalTrace("return: summary")
	return summary
}

// formatTokens formats a token count for display.
// Matches TS formatTokens = formatNumber(count).replace('.0', ”).toLowerCase():
// 900 → "900", 1000 → "1k", 1500 → "1.5k", 1000000 → "1m"
func formatTokens(n int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if n >= 1_000_000 {
		observe.GlobalTrace("if: n >= 1_000_000")
		s := fmt.Sprintf("%.1fm", float64(n)/1_000_000)
		observe.GlobalTrace("return: stripTrailingZeroDecimal(s)")
		return stripTrailingZeroDecimal(s)
	}
	if n >= 1_000 {
		observe.GlobalTrace("if: n >= 1_000")
		s := fmt.Sprintf("%.1fk", float64(n)/1_000)
		observe.GlobalTrace("return: stripTrailingZeroDecimal(s)")
		return stripTrailingZeroDecimal(s)
	}
	observe.GlobalTrace("return: fmt.Sprintf(\"%d\", n)")
	return fmt.Sprintf("%d", n)
}

// stripTrailingZeroDecimal removes ".0" before the suffix letter.
// "1.0k" → "1k", "1.5k" → "1.5k", "1.0m" → "1m"
func stripTrailingZeroDecimal(s string) string {
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

// formatDuration formats a duration for the toolbar.
// Matches TS utils/format.ts (default options):
// <60s → "Ns", <60m → "Nm Ns", <24h → "Nh Nm Ns", >=24h → "Nd Nh Nm"
func formatDuration(d time.Duration) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	totalSec := int(d.Seconds())
	if totalSec < 60 {
		observe.GlobalTrace("if: totalSec < 60")
		observe.GlobalTrace("return: fmt.Sprintf(\"%ds\", totalSec)")
		return fmt.Sprintf("%ds", totalSec)
	}

	days := totalSec / 86400
	hours := (totalSec % 86400) / 3600
	minutes := (totalSec % 3600) / 60
	seconds := totalSec % 60

	if days > 0 {
		observe.GlobalTrace("if: days > 0")
		observe.GlobalTrace("return: fmt.Sprintf(\"%dd %dh %dm\", days, hours, minutes)")
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		observe.GlobalTrace("if: hours > 0")
		observe.GlobalTrace("return: fmt.Sprintf(\"%dh %dm %ds\", hours, minutes, seconds)")
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if seconds > 0 {
		observe.GlobalTrace("if: seconds > 0")
		observe.GlobalTrace("return: fmt.Sprintf(\"%dm %ds\", minutes, seconds)")
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	observe.GlobalTrace("return: fmt.Sprintf(\"%dm\", minutes)")
	return fmt.Sprintf("%dm", minutes)
}

// formatCost formats a USD cost for display.
// Matches TS cost-tracker.ts: ≤$0.50 → 4 decimals, >$0.50 → 2 decimals.
func formatCost(cost float64) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if cost > 0.5 {
		observe.GlobalTrace("if: cost > 0.5")
		rounded := math.Round(cost*100) / 100
		observe.GlobalTrace("return: fmt.Sprintf(\"$%.2f\", rounded)")
		return fmt.Sprintf("$%.2f", rounded)
	}
	observe.GlobalTrace("return: fmt.Sprintf(\"$%.4f\", cost)")
	return fmt.Sprintf("$%.4f", cost)
}
