package render

import (
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/observe"
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
	agentDim  = lipgloss.NewStyle().Faint(true)
)

// RenderAgentProgress renders agent execution progress as a tree.
// Matches TS AgentProgressLine + renderGroupedAgentToolUse.
// Non-verbose: per-agent line with metrics + status. Verbose: same (no extra detail for agents).
func RenderAgentProgress(agents []AgentProgressEntry, verbose bool, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(agents) == 0 {
		observe.GlobalTrace("if: len(agents) == 0")
		observe.GlobalTrace("return: ContentIndent + agentDim.Render(\"Agent: initializing…\")")
		return ContentIndent + agentDim.Render("Agent: initializing…")
	}

	var b strings.Builder

	if len(agents) > 1 {
		observe.GlobalTrace("if: len(agents) > 1")
		allBackground := true
		allComplete := true
		for _, a := range agents {
			observe.GlobalTrace("range agents")
			if a.Status != "completed" && a.Status != "error" {
				observe.GlobalTrace("if: a.Status != \"completed\" && a.Status != \"error\"")
				allComplete = false
			}
			if !a.Background {
				observe.GlobalTrace("if: !a.Background")
				allBackground = false
			}
		}

		b.WriteString(ContentIndent)
		if allComplete {
			observe.GlobalTrace("if: allComplete")
			if allBackground {
				observe.GlobalTrace("if: allBackground")
				b.WriteString(fmt.Sprintf("%s background agents launched", agentBold.Render(fmt.Sprintf("%d", len(agents)))))
			} else {
				observe.GlobalTrace("else: allBackground")
				b.WriteString(fmt.Sprintf("%s agents finished", agentBold.Render(fmt.Sprintf("%d", len(agents)))))
			}
		} else {
			observe.GlobalTrace("else: allComplete")
			b.WriteString(fmt.Sprintf("Running %s agents…", agentBold.Render(fmt.Sprintf("%d", len(agents)))))
		}

		if !allBackground && !verbose {
			observe.GlobalTrace("if: !allBackground && !verbose")
			b.WriteString(" ")
			b.WriteString(agentDim.Render("(ctrl+o to expand)"))
		}
		b.WriteString("\n")
	}

	for i, a := range agents {
		observe.GlobalTrace("range agents")
		isLast := i == len(agents)-1
		renderAgentLine(&b, a, isLast, len(agents) > 1)
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

// renderAgentLine renders a single agent progress entry with tree glyphs.
// Matches TS AgentProgressLine layout:
//
//	├─ Agent (description) · N tool uses · M tokens
//	│  ⎿  Initializing… / lastTool / Done
func renderAgentLine(b *strings.Builder, a AgentProgressEntry, isLast bool, hasMultiple bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	treeChar := "├─"
	contChar := "│ "
	if isLast {
		observe.GlobalTrace("if: isLast")
		treeChar = "└─"
		contChar = "  "
	}

	isResolved := a.Status == "completed" || a.Status == "error"
	isBackgrounded := a.Background && isResolved

	b.WriteString(ContentIndent)
	if hasMultiple {
		observe.GlobalTrace("if: hasMultiple")
		b.WriteString(agentDim.Render(treeChar))
		b.WriteString(" ")
	}

	label := "Agent"
	if a.Description != "" {
		observe.GlobalTrace("if: a.Description != \"\"")
		label += " (" + truncateAgentText(a.Description, 50) + ")"
	}

	metrics := ""
	if !isBackgrounded {
		observe.GlobalTrace("if: !isBackgrounded")
		metrics = fmt.Sprintf(" · %d tool %s", a.ToolCount, pluralize(a.ToolCount, "use", "uses"))
		if a.TokenCount > 0 {
			observe.GlobalTrace("if: a.TokenCount > 0")
			metrics += " · " + formatTokenCount(a.TokenCount) + " tokens"
		}
	}

	if !isResolved {
		observe.GlobalTrace("if: !isResolved")
		b.WriteString(agentDim.Render(label + metrics))
	} else {
		observe.GlobalTrace("else: !isResolved")
		b.WriteString(label + metrics)
	}
	b.WriteString("\n")

	if !isBackgrounded {
		observe.GlobalTrace("if: !isBackgrounded")
		if hasMultiple {
			observe.GlobalTrace("if: hasMultiple")
			b.WriteString(ContentIndent)
			b.WriteString(agentDim.Render(contChar))
			b.WriteString(" ")
		} else {
			observe.GlobalTrace("else: hasMultiple")
			b.WriteString(ContentIndent)
		}
		b.WriteString(agentDim.Render("⎿  "))
		b.WriteString(agentDim.Render(agentStatusText(a)))
		b.WriteString("\n")
	}
}

func truncateAgentText(s string, maxLen int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if maxLen <= 0 || len(s) <= maxLen {
		observe.GlobalTrace("if: maxLen <= 0 || len(s) <= maxLen")
		observe.GlobalTrace("return: s")
		return s
	}
	if maxLen <= 1 {
		observe.GlobalTrace("if: maxLen <= 1")
		observe.GlobalTrace("return: \"…\"")
		return "…"
	}
	observe.GlobalTrace("return: s[:maxLen-1] + \"…\"")
	return s[:maxLen-1] + "…"
}

// agentStatusText returns the status text for an agent entry.
// Matches TS getStatusText logic.
func agentStatusText(a AgentProgressEntry) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if a.Status != "completed" && a.Status != "error" {
		observe.GlobalTrace("if: a.Status != \"completed\" && a.Status != \"error\"")
		if a.LastTool != "" {
			observe.GlobalTrace("if: a.LastTool != \"\"")
			observe.GlobalTrace("return: a.LastTool")
			return a.LastTool
		}
		observe.GlobalTrace("return: \"Initializing…\"")
		return "Initializing…"
	}
	if a.Background {
		observe.GlobalTrace("if: a.Background")
		observe.GlobalTrace("return: \"Running in the background\"")
		return "Running in the background"
	}
	if a.Status == "error" && a.Error != "" {
		observe.GlobalTrace("if: a.Status == \"error\" && a.Error != \"\"")
		msg := a.Error
		if len(msg) > 60 {
			observe.GlobalTrace("if: len(msg) > 60")
			msg = msg[:57] + "..."
		}
		observe.GlobalTrace("return: \"Error: \" + msg")
		return "Error: " + msg
	}
	observe.GlobalTrace("return: \"Done\"")
	return "Done"
}

// formatTokenCount formats a token count for display.
// e.g., 1234 → "1,234", 1234567 → "1.2M" (matching TS formatNumber).
func formatTokenCount(n int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if n >= 1_000_000 {
		observe.GlobalTrace("if: n >= 1_000_000")
		observe.GlobalTrace("return: fmt.Sprintf(\"%.1fM\", float64(n)/1_000_000)")
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1000 {
		observe.GlobalTrace("if: n >= 1000")
		observe.GlobalTrace("return: fmt.Sprintf(\"%d,%03d\", n/1000, n%1000)")
		return fmt.Sprintf("%d,%03d", n/1000, n%1000)
	}
	observe.GlobalTrace("return: fmt.Sprintf(\"%d\", n)")
	return fmt.Sprintf("%d", n)
}

func pluralize(n int, singular, plural string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if n == 1 {
		observe.GlobalTrace("if: n == 1")
		observe.GlobalTrace("return: singular")
		return singular
	}
	observe.GlobalTrace("return: plural")
	return plural
}
