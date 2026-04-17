package render

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// LifecycleStep holds progress for a single superstep (passed from tui model).
type LifecycleStep struct {
	Step        int
	Nodes       []string
	Results     map[string]LifecycleNodeResult
	Transitions []LifecycleTransition
	Status      string // "running", "completed"
}

// LifecycleNodeResult holds the outcome of a single node execution.
type LifecycleNodeResult struct {
	Duration time.Duration
	Error    string
}

// LifecycleTransition records an edge traversal in the graph.
type LifecycleTransition struct {
	From     string
	To       string
	RouteKey string
}

var (
	glyphRunning   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "33", Dark: "75"}).Render("●")
	glyphCompleted = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "114"}).Render("✓")
	glyphError     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "124", Dark: "210"}).Render("✗")
	lifecycleDim   = lipgloss.NewStyle().Faint(true)
)

// RenderLifecycleProgress renders lifecycle execution progress as a tree.
// Non-verbose: compact single-line showing current step.
// Verbose: full tree with all steps, node durations, and transitions.
func RenderLifecycleProgress(steps []LifecycleStep, completed bool, errMsg string, verbose bool, width int) string {
	if len(steps) == 0 && !completed {
		return ContentIndent + lifecycleDim.Render("Lifecycle: initializing…")
	}

	if !verbose {
		return renderLifecycleCompact(steps, completed, errMsg)
	}
	return renderLifecycleVerbose(steps, completed, errMsg)
}

func renderLifecycleCompact(steps []LifecycleStep, completed bool, errMsg string) string {
	var b strings.Builder

	if completed {
		if errMsg != "" {
			b.WriteString(ContentIndent)
			b.WriteString(glyphError)
			b.WriteString(fmt.Sprintf(" Lifecycle failed (%d steps) — %s", len(steps), truncate(errMsg, 60)))
		} else {
			b.WriteString(ContentIndent)
			b.WriteString(glyphCompleted)
			b.WriteString(fmt.Sprintf(" Lifecycle completed (%d steps)", len(steps)))
		}
		b.WriteString("\n")
		appendExpandHint(&b)
		return strings.TrimRight(b.String(), "\n")
	}

	// Show current (last) step
	if len(steps) > 0 {
		current := steps[len(steps)-1]
		b.WriteString(ContentIndent)
		b.WriteString(glyphRunning)
		b.WriteString(fmt.Sprintf(" Step %d/%d: %s", current.Step, len(steps), strings.Join(current.Nodes, ", ")))
		if current.Status == "running" {
			b.WriteString("…")
		}
		b.WriteString("\n")
		appendExpandHint(&b)
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderLifecycleVerbose(steps []LifecycleStep, completed bool, errMsg string) string {
	var b strings.Builder

	totalSteps := len(steps)
	for i, step := range steps {
		isLast := i == totalSteps-1 && completed
		treeChar := "├─"
		contChar := "│ "
		if isLast {
			treeChar = "└─"
			contChar = "  "
		}

		// Step header with nodes and status glyph
		glyph := glyphRunning
		if step.Status == "completed" {
			glyph = glyphCompleted
		}
		// Check for node errors
		for _, r := range step.Results {
			if r.Error != "" {
				glyph = glyphError
				break
			}
		}

		b.WriteString(ContentIndent)
		b.WriteString(lifecycleDim.Render(treeChar))
		b.WriteString(fmt.Sprintf(" Step %d: ", step.Step))
		b.WriteString(strings.Join(step.Nodes, ", "))
		b.WriteString(" ")
		b.WriteString(glyph)

		// Show duration for completed nodes
		if step.Status == "completed" && len(step.Results) > 0 {
			var totalDur time.Duration
			for _, r := range step.Results {
				if r.Duration > totalDur {
					totalDur = r.Duration
				}
			}
			if totalDur > 0 {
				b.WriteString(lifecycleDim.Render(fmt.Sprintf(" (%s)", totalDur.Round(100*time.Millisecond))))
			}
		}
		b.WriteString("\n")

		// Show transitions
		for j, tr := range step.Transitions {
			trIsLast := j == len(step.Transitions)-1
			trTree := "├─"
			if trIsLast {
				trTree = "└─"
			}
			b.WriteString(ContentIndent)
			b.WriteString(lifecycleDim.Render(contChar))
			b.WriteString(" ")
			b.WriteString(lifecycleDim.Render(trTree))
			b.WriteString(" ")
			route := tr.From + " → " + tr.To
			if tr.RouteKey != "" {
				route += " (route: " + tr.RouteKey + ")"
			}
			b.WriteString(lifecycleDim.Render(route))
			b.WriteString("\n")
		}
	}

	// Completion line
	if completed {
		if errMsg != "" {
			b.WriteString(ContentIndent)
			b.WriteString(lifecycleDim.Render("└─"))
			b.WriteString(" ")
			b.WriteString(glyphError)
			b.WriteString(" Failed: ")
			b.WriteString(truncate(errMsg, 80))
			b.WriteString("\n")
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// renderLifecycleRun renders the LifecycleRun tool result for historical sessions.
// Parses the JSON result and shows a summary.
func renderLifecycleRun(_ json.RawMessage, content string, isError bool, width int, display string, verbose bool) string {
	if isError {
		return WrapWithBracket(content, true, width, verbose)
	}

	// Parse the lifecycle result JSON
	var result struct {
		Status      string   `json:"status"`
		Result      string   `json:"result,omitempty"`
		Steps       int      `json:"steps"`
		Passed      bool     `json:"passed,omitempty"`
		Score       float64  `json:"score,omitempty"`
		Reflections []string `json:"reflections,omitempty"`
		Error       string   `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return WrapWithBracket(content, false, width, verbose)
	}

	var b strings.Builder

	if !verbose {
		// Compact: single-line summary
		b.WriteString(ContentIndent)
		if result.Status == "completed" {
			b.WriteString(glyphCompleted)
			b.WriteString(fmt.Sprintf(" Lifecycle: completed (%d steps)", result.Steps))
		} else {
			b.WriteString(glyphError)
			b.WriteString(fmt.Sprintf(" Lifecycle: %s", result.Status))
			if result.Error != "" {
				b.WriteString(" — " + truncate(result.Error, 60))
			}
		}
		b.WriteString("\n")
		appendExpandHint(&b)
		return strings.TrimRight(b.String(), "\n")
	}

	// Verbose: full details
	b.WriteString(ContentIndent)
	if result.Status == "completed" {
		b.WriteString(glyphCompleted)
	} else {
		b.WriteString(glyphError)
	}
	b.WriteString(fmt.Sprintf(" Lifecycle: %s (%d steps)", result.Status, result.Steps))
	b.WriteString("\n")

	if result.Passed {
		b.WriteString(ContentIndent)
		b.WriteString(lifecycleDim.Render("  passed: true"))
		b.WriteString("\n")
	}
	if result.Score > 0 {
		b.WriteString(ContentIndent)
		b.WriteString(lifecycleDim.Render(fmt.Sprintf("  score: %.2f", result.Score)))
		b.WriteString("\n")
	}
	if result.Error != "" {
		b.WriteString(ContentIndent)
		b.WriteString(lifecycleDim.Render("  error: " + truncate(result.Error, 80)))
		b.WriteString("\n")
	}
	if len(result.Reflections) > 0 {
		b.WriteString(ContentIndent)
		b.WriteString(lifecycleDim.Render(fmt.Sprintf("  reflections: %d", len(result.Reflections))))
		b.WriteString("\n")
	}
	if result.Result != "" {
		// Show first 5 lines of result
		lines := strings.Split(result.Result, "\n")
		maxLines := 5
		if len(lines) > maxLines {
			lines = lines[:maxLines]
		}
		for _, line := range lines {
			b.WriteString(ContentIndent)
			b.WriteString("  ")
			b.WriteString(truncate(line, 100))
			b.WriteString("\n")
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-3]) + "..."
}
