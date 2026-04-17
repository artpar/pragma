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
	modelName    string
	provider     string
	workspace    string
	totalCost    float64
	status       string
	inputTokens  int
	outputTokens int
	cacheTokens  int       // combined cache creation + cache read
	contextSize  int
	startTime    time.Time // session start for elapsed display
}

func newToolbar(modelName, provider, workspace string) toolbar {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	return toolbar{
		modelName: modelName,
		provider:  provider,
		workspace: filepath.Base(workspace),
		status:    "ready",
		startTime: time.Now(),
	}
}

// View renders the toolbar at the given width.
func (t toolbar) View(width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	// Left section: model · provider · duration · cost · tokens
	left := fmt.Sprintf(" %s %s · %s", render.BlackCircle, t.modelName, t.provider)

	// Workspace only at wide terminals (between provider and duration)
	if width >= 100 && t.workspace != "" {
		observe.GlobalTrace("if: width >= 100 && t.workspace != \"\"")
		left += " · " + t.workspace
	}

	elapsed := formatDuration(time.Since(t.startTime))
	left += " · " + elapsed + " · " + formatCost(t.totalCost)

	if width >= 60 && t.contextSize > 0 {
		observe.GlobalTrace("if: width >= 60 && t.contextSize > 0")
		// Context pct: (input + cache) / contextSize — excludes output tokens (TS #28167)
		contextTokens := t.inputTokens + t.cacheTokens
		pct := float64(contextTokens) / float64(t.contextSize) * 100

		tokenStr := fmt.Sprintf(" · %s in / %s out",
			formatTokens(t.inputTokens),
			formatTokens(t.outputTokens))
		if t.cacheTokens > 0 {
			tokenStr += " / " + formatTokens(t.cacheTokens) + " cache"
		}
		tokenStr += fmt.Sprintf(" (%d%% ctx)", int(pct))

		tokenStyled := t.styleTokenStr(tokenStr, pct)
		left += tokenStyled
	}

	right := fmt.Sprintf(" %s ", t.status)
	styledRight := statusActiveStyle.Render(right)

	gap := max(width-lipgloss.Width(left)-len(right), 0)
	bar := left + strings.Repeat(" ", gap) + styledRight

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
func (t *toolbar) UpdateTokens(input, output, cache, contextSize int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	t.inputTokens = input
	t.outputTokens = output
	t.cacheTokens = cache
	t.contextSize = contextSize
}

// CostSummary returns a one-line session summary for display on exit.
func (t toolbar) CostSummary() string {
	summary := fmt.Sprintf("Session cost: %s · Duration: %s · Tokens: %s in / %s out",
		formatCost(t.totalCost),
		formatDuration(time.Since(t.startTime)),
		formatTokens(t.inputTokens),
		formatTokens(t.outputTokens))
	if t.cacheTokens > 0 {
		summary += " / " + formatTokens(t.cacheTokens) + " cache"
	}
	return summary
}

// formatTokens formats a token count for display.
// Matches TS formatTokens = formatNumber(count).replace('.0', '').toLowerCase():
// 900 → "900", 1000 → "1k", 1500 → "1.5k", 1000000 → "1m"
func formatTokens(n int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if n >= 1_000_000 {
		observe.GlobalTrace("if: n >= 1_000_000")
		s := fmt.Sprintf("%.1fm", float64(n)/1_000_000)
		return stripTrailingZeroDecimal(s)
	}
	if n >= 1_000 {
		observe.GlobalTrace("if: n >= 1_000")
		s := fmt.Sprintf("%.1fk", float64(n)/1_000)
		return stripTrailingZeroDecimal(s)
	}
	return fmt.Sprintf("%d", n)
}

// stripTrailingZeroDecimal removes ".0" before the suffix letter.
// "1.0k" → "1k", "1.5k" → "1.5k", "1.0m" → "1m"
func stripTrailingZeroDecimal(s string) string {
	if len(s) >= 4 && s[len(s)-3:len(s)-1] == ".0" {
		return s[:len(s)-3] + s[len(s)-1:]
	}
	return s
}

// formatDuration formats a duration for the toolbar.
// Matches TS utils/format.ts (default options):
// <60s → "Ns", <60m → "Nm Ns", <24h → "Nh Nm Ns", >=24h → "Nd Nh Nm"
func formatDuration(d time.Duration) string {
	totalSec := int(d.Seconds())
	if totalSec < 60 {
		return fmt.Sprintf("%ds", totalSec)
	}

	days := totalSec / 86400
	hours := (totalSec % 86400) / 3600
	minutes := (totalSec % 3600) / 60
	seconds := totalSec % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if seconds > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%dm", minutes)
}

// formatCost formats a USD cost for display.
// Matches TS cost-tracker.ts: ≤$0.50 → 4 decimals, >$0.50 → 2 decimals.
func formatCost(cost float64) string {
	if cost > 0.5 {
		rounded := math.Round(cost*100) / 100
		return fmt.Sprintf("$%.2f", rounded)
	}
	return fmt.Sprintf("$%.4f", cost)
}
