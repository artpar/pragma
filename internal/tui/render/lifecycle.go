package render

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/observe"
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(steps) == 0 && !completed {
		observe.GlobalTrace("if: len(steps) == 0 && !completed")
		observe.GlobalTrace("return: ContentIndent + lifecycleDim.Render(\"Lifecycle: initializing…\")")
		return ContentIndent + lifecycleDim.Render("Lifecycle: initializing…")
	}

	if !verbose {
		observe.GlobalTrace("if: !verbose")
		observe.GlobalTrace("return: renderLifecycleCompact(steps, completed, errMsg)")
		return renderLifecycleCompact(steps, completed, errMsg)
	}
	observe.GlobalTrace("return: renderLifecycleVerbose(steps, completed, errMsg)")
	return renderLifecycleVerbose(steps, completed, errMsg)
}

func renderLifecycleCompact(steps []LifecycleStep, completed bool, errMsg string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder

	if completed {
		observe.GlobalTrace("if: completed")
		if errMsg != "" {
			observe.GlobalTrace("if: errMsg != \"\"")
			b.WriteString(ContentIndent)
			b.WriteString(glyphError)
			b.WriteString(fmt.Sprintf(" Lifecycle failed (%d steps) — %s", len(steps), truncate(errMsg, 60)))
		} else {
			observe.GlobalTrace("else: errMsg != \"\"")
			b.WriteString(ContentIndent)
			b.WriteString(glyphCompleted)
			b.WriteString(fmt.Sprintf(" Lifecycle completed (%d steps)", len(steps)))
		}
		b.WriteString("\n")
		appendExpandHint(&b)
		observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")
		return strings.TrimRight(b.String(), "\n")
	}

	if len(steps) > 0 {
		observe.GlobalTrace("if: len(steps) > 0")
		current := steps[len(steps)-1]
		b.WriteString(ContentIndent)
		b.WriteString(glyphRunning)
		b.WriteString(fmt.Sprintf(" Step %d/%d: %s", current.Step, len(steps), strings.Join(current.Nodes, ", ")))
		if current.Status == "running" {
			observe.GlobalTrace("if: current.Status == \"running\"")
			b.WriteString("…")
		}
		b.WriteString("\n")
		appendExpandHint(&b)
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")
	return strings.TrimRight(b.String(), "\n")
}

func renderLifecycleVerbose(steps []LifecycleStep, completed bool, errMsg string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder

	totalSteps := len(steps)
	for i, step := range steps {
		observe.GlobalTrace("range steps")
		isLast := i == totalSteps-1 && completed
		treeChar := "├─"
		contChar := "│ "
		if isLast {
			observe.GlobalTrace("if: isLast")
			treeChar = "└─"
			contChar = "  "
		}

		glyph := glyphRunning
		if step.Status == "completed" {
			observe.GlobalTrace("if: step.Status == \"completed\"")
			glyph = glyphCompleted
		}

		for _, r := range step.Results {
			observe.GlobalTrace("range step.Results")
			if r.Error != "" {
				observe.GlobalTrace("if: r.Error != \"\"")
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

		if step.Status == "completed" && len(step.Results) > 0 {
			observe.GlobalTrace("if: step.Status == \"completed\" && len(step.Results) > 0")
			var totalDur time.Duration
			for _, r := range step.Results {
				observe.GlobalTrace("range step.Results")
				if r.Duration > totalDur {
					observe.GlobalTrace("if: r.Duration > totalDur")
					totalDur = r.Duration
				}
			}
			if totalDur > 0 {
				observe.GlobalTrace("if: totalDur > 0")
				b.WriteString(lifecycleDim.Render(fmt.Sprintf(" (%s)", totalDur.Round(100*time.Millisecond))))
			}
		}
		b.WriteString("\n")

		for j, tr := range step.Transitions {
			observe.GlobalTrace("range step.Transitions")
			trIsLast := j == len(step.Transitions)-1
			trTree := "├─"
			if trIsLast {
				observe.GlobalTrace("if: trIsLast")
				trTree = "└─"
			}
			b.WriteString(ContentIndent)
			b.WriteString(lifecycleDim.Render(contChar))
			b.WriteString(" ")
			b.WriteString(lifecycleDim.Render(trTree))
			b.WriteString(" ")
			route := tr.From + " → " + tr.To
			if tr.RouteKey != "" {
				observe.GlobalTrace("if: tr.RouteKey != \"\"")
				route += " (route: " + tr.RouteKey + ")"
			}
			b.WriteString(lifecycleDim.Render(route))
			b.WriteString("\n")
		}
	}

	if completed {
		observe.GlobalTrace("if: completed")
		if errMsg != "" {
			observe.GlobalTrace("if: errMsg != \"\"")
			b.WriteString(ContentIndent)
			b.WriteString(lifecycleDim.Render("└─"))
			b.WriteString(" ")
			b.WriteString(glyphError)
			b.WriteString(" Failed: ")
			b.WriteString(truncate(errMsg, 80))
			b.WriteString("\n")
		}
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

// renderLifecycleRun renders the LifecycleRun tool result for historical sessions.
// Parses the JSON result and shows a summary.
func renderLifecycleRun(_ json.RawMessage, content string, isError bool, width int, display string, verbose bool) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if isError {
		observe.GlobalTrace("if: isError")
		observe.GlobalTrace("return: WrapWithBracket(content, true, width, verbose)")
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
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: WrapWithBracket(content, false, width, verbose)")
		return WrapWithBracket(content, false, width, verbose)
	}

	var b strings.Builder

	if !verbose {
		observe.GlobalTrace("if: !verbose")

		b.WriteString(ContentIndent)
		if result.Status == "completed" {
			observe.GlobalTrace("if: result.Status == \"completed\"")
			b.WriteString(glyphCompleted)
			b.WriteString(fmt.Sprintf(" Lifecycle: completed (%d steps)", result.Steps))
		} else {
			observe.GlobalTrace("else: result.Status == \"completed\"")
			b.WriteString(glyphError)
			b.WriteString(fmt.Sprintf(" Lifecycle: %s", result.Status))
			if result.Error != "" {
				observe.GlobalTrace("if: result.Error != \"\"")
				b.WriteString(" — " + truncate(result.Error, 60))
			}
		}
		b.WriteString("\n")
		appendExpandHint(&b)
		observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")
		return strings.TrimRight(b.String(), "\n")
	}

	b.WriteString(ContentIndent)
	if result.Status == "completed" {
		observe.GlobalTrace("if: result.Status == \"completed\"")
		b.WriteString(glyphCompleted)
	} else {
		observe.GlobalTrace("else: result.Status == \"completed\"")
		b.WriteString(glyphError)
	}
	b.WriteString(fmt.Sprintf(" Lifecycle: %s (%d steps)", result.Status, result.Steps))
	b.WriteString("\n")

	if result.Passed {
		observe.GlobalTrace("if: result.Passed")
		b.WriteString(ContentIndent)
		b.WriteString(lifecycleDim.Render("  passed: true"))
		b.WriteString("\n")
	}
	if result.Score > 0 {
		observe.GlobalTrace("if: result.Score > 0")
		b.WriteString(ContentIndent)
		b.WriteString(lifecycleDim.Render(fmt.Sprintf("  score: %.2f", result.Score)))
		b.WriteString("\n")
	}
	if result.Error != "" {
		observe.GlobalTrace("if: result.Error != \"\"")
		b.WriteString(ContentIndent)
		b.WriteString(lifecycleDim.Render("  error: " + truncate(result.Error, 80)))
		b.WriteString("\n")
	}
	if len(result.Reflections) > 0 {
		observe.GlobalTrace("if: len(result.Reflections) > 0")
		b.WriteString(ContentIndent)
		b.WriteString(lifecycleDim.Render(fmt.Sprintf("  reflections: %d", len(result.Reflections))))
		b.WriteString("\n")
	}
	if result.Result != "" {
		observe.GlobalTrace("if: result.Result != \"\"")

		lines := strings.Split(result.Result, "\n")
		maxLines := 5
		if len(lines) > maxLines {
			observe.GlobalTrace("if: len(lines) > maxLines")
			lines = lines[:maxLines]
		}
		for _, line := range lines {
			observe.GlobalTrace("range lines")
			b.WriteString(ContentIndent)
			b.WriteString("  ")
			b.WriteString(truncate(line, 100))
			b.WriteString("\n")
		}
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

func truncate(s string, max int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	runes := []rune(s)
	if len(runes) <= max {
		observe.GlobalTrace("if: len(runes) <= max")
		observe.GlobalTrace("return: s")
		return s
	}
	observe.GlobalTrace("return: string(runes[:max-3]) + \"...\"")
	return string(runes[:max-3]) + "..."
}
