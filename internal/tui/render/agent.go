package render

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// AgentProgressEntry holds one agent's progress for rendering.
type AgentProgressEntry struct {
	AgentID     string
	Description string
	LastTool    string
	Status      string // "initializing", "running", "completed", "error"
	Error       string
	ToolCount   int
	TokenCount  int
	Background  bool
}

var (
	agentBold = lipgloss.NewStyle().Bold(true)
)

// RenderAgentProgress renders agent execution progress as a tree.
// Matches TS AgentProgressLine + renderGroupedAgentToolUse.
// Non-verbose: per-agent line with metrics + status. Verbose: same (no extra detail for agents).
func RenderAgentProgress(agents []AgentProgressEntry, verbose bool, width int) string {
	if len(agents) == 0 {
		return ContentIndent + lifecycleDim.Render("Agent: initializing…")
	}

	var b strings.Builder

	// Header for multiple agents (matching TS renderGroupedAgentToolUse)
	if len(agents) > 1 {
		allBackground := true
		allComplete := true
		for _, a := range agents {
			if a.Status != "completed" && a.Status != "error" {
				allComplete = false
			}
			if !a.Background {
				allBackground = false
			}
		}

		b.WriteString(ContentIndent)
		if allComplete {
			if allBackground {
				b.WriteString(fmt.Sprintf("%s background agents launched", agentBold.Render(fmt.Sprintf("%d", len(agents)))))
			} else {
				b.WriteString(fmt.Sprintf("%s agents finished", agentBold.Render(fmt.Sprintf("%d", len(agents)))))
			}
		} else {
			b.WriteString(fmt.Sprintf("Running %s agents…", agentBold.Render(fmt.Sprintf("%d", len(agents)))))
		}

		if !allBackground && !verbose {
			b.WriteString(" ")
			b.WriteString(lifecycleDim.Render("(ctrl+o to expand)"))
		}
		b.WriteString("\n")
	}

	// Per-agent lines
	for i, a := range agents {
		isLast := i == len(agents)-1
		renderAgentLine(&b, a, isLast, len(agents) > 1)
	}

	return strings.TrimRight(b.String(), "\n")
}

// renderAgentLine renders a single agent progress entry with tree glyphs.
// Matches TS AgentProgressLine layout:
//
//	├─ Agent (description) · N tool uses · M tokens
//	│  ⎿  Initializing… / lastTool / Done
func renderAgentLine(b *strings.Builder, a AgentProgressEntry, isLast bool, hasMultiple bool) {
	treeChar := "├─"
	contChar := "│ "
	if isLast {
		treeChar = "└─"
		contChar = "  "
	}

	isResolved := a.Status == "completed" || a.Status == "error"
	isBackgrounded := a.Background && isResolved

	// Line 1: tree glyph + agent type + description + metrics
	b.WriteString(ContentIndent)
	if hasMultiple {
		b.WriteString(lifecycleDim.Render(treeChar))
		b.WriteString(" ")
	}

	// Agent type + description — dim when not resolved (matching TS dimColor={!isResolved})
	label := "Agent"
	if a.Description != "" {
		label += " (" + truncate(a.Description, 50) + ")"
	}

	// Metrics — hidden for background agents (matching TS !isBackgrounded)
	metrics := ""
	if !isBackgrounded {
		metrics = fmt.Sprintf(" · %d tool %s", a.ToolCount, pluralize(a.ToolCount, "use", "uses"))
		if a.TokenCount > 0 {
			metrics += " · " + formatTokenCount(a.TokenCount) + " tokens"
		}
	}

	if !isResolved {
		b.WriteString(lifecycleDim.Render(label + metrics))
	} else {
		b.WriteString(label + metrics)
	}
	b.WriteString("\n")

	// Line 2: status line — hidden for backgrounded agents (matching TS !isBackgrounded)
	if !isBackgrounded {
		if hasMultiple {
			b.WriteString(ContentIndent)
			b.WriteString(lifecycleDim.Render(contChar))
			b.WriteString(" ")
		} else {
			b.WriteString(ContentIndent)
		}
		b.WriteString(lifecycleDim.Render("⎿  "))
		b.WriteString(lifecycleDim.Render(agentStatusText(a)))
		b.WriteString("\n")
	}
}

// agentStatusText returns the status text for an agent entry.
// Matches TS getStatusText logic.
func agentStatusText(a AgentProgressEntry) string {
	if a.Status != "completed" && a.Status != "error" {
		if a.LastTool != "" {
			return a.LastTool
		}
		return "Initializing…"
	}
	if a.Background {
		return "Running in the background"
	}
	if a.Status == "error" && a.Error != "" {
		msg := a.Error
		if len(msg) > 60 {
			msg = msg[:57] + "..."
		}
		return "Error: " + msg
	}
	return "Done"
}

// formatTokenCount formats a token count for display.
// e.g., 1234 → "1,234", 1234567 → "1.2M" (matching TS formatNumber).
func formatTokenCount(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%d,%03d", n/1000, n%1000)
	}
	return fmt.Sprintf("%d", n)
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

