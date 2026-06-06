package render

import (
	"encoding/json"
	"fmt"
	"github.com/artpar/pragma/internal/observe"
	"strings"
)

// GroupEntry holds one tool call + result pair for rendering within a collapsed group.
type GroupEntry struct {
	CallHeader string          // pre-rendered "⏺ Read(file.go)\n"
	Name       string          // tool name
	Input      json.RawMessage // raw tool input
	Content    string          // tool result content
	IsError    bool
	HasResult  bool // false while waiting for result
}

// GroupData holds the render-ready data for a collapsed read/search group.
type GroupData struct {
	Entries     []GroupEntry
	SearchCount int    // Grep + Glob count (both are search tools in TS reference)
	ReadCount   int    // unique Read file count
	Active      bool   // true while still accumulating
	LatestHint  string // last file path or "pattern"
}

// RenderToolGroup renders a collapsed read/search group.
// Non-verbose: compact summary badge. Verbose: individual tool calls with results.
func RenderToolGroup(g GroupData, verbose bool, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if verbose {
		observe.GlobalTrace("if: verbose")
		observe.GlobalTrace("return: renderGroupVerbose(g, width)")
		return renderGroupVerbose(g, width)
	}
	observe.GlobalTrace("return: renderGroupCollapsed(g, width)")
	return renderGroupCollapsed(g, width)
}

// renderGroupVerbose renders each tool call and result individually.
func renderGroupVerbose(g GroupData, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder
	for _, e := range g.Entries {
		observe.GlobalTrace("range g.Entries")
		b.WriteString(e.CallHeader)
		if e.HasResult {
			observe.GlobalTrace("if: e.HasResult")
			b.WriteString(RenderToolOutput(e.Name, e.Input, e.Content, e.IsError, width, true))
			b.WriteString("\n")
		}
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")
	return strings.TrimRight(b.String(), "\n")
}

// renderGroupCollapsed renders the compact summary badge.
func renderGroupCollapsed(g GroupData, width int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder

	summary := GenerateGroupSummary(g.SearchCount, g.ReadCount, g.Active)
	if summary == "" {
		observe.GlobalTrace("if: summary == \"\"")
		observe.GlobalTrace("return: \"\"")

		return ""
	}

	b.WriteString(bracketDim.Render(BracketPrefix))
	b.WriteString(dimText.Render(summary))
	b.WriteString("\n")

	if g.Active && g.LatestHint != "" {
		observe.GlobalTrace("if: g.Active && g.LatestHint != \"\"")
		b.WriteString(ContentIndent)
		b.WriteString(dimText.Render(g.LatestHint))
		b.WriteString("\n")
	}

	appendExpandHint(&b)
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")

	return strings.TrimRight(b.String(), "\n")
}

// GenerateGroupSummary builds the summary text for a collapsed group.
// Matches TS getSearchReadSummaryText: proper tense, capitalization, grammar.
// Active: present tense + trailing "…" (U+2026). Completed: past tense.
func GenerateGroupSummary(searchCount, readCount int, active bool) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var parts []string

	if searchCount > 0 {
		observe.GlobalTrace("if: searchCount > 0")
		noun := "pattern"
		if searchCount > 1 {
			observe.GlobalTrace("if: searchCount > 1")
			noun = "patterns"
		}
		var verb string
		if active {
			observe.GlobalTrace("if: active")
			if len(parts) == 0 {
				observe.GlobalTrace("if: len(parts) == 0")
				verb = "Searching for"
			} else {
				observe.GlobalTrace("else: len(parts) == 0")
				verb = "searching for"
			}
		} else {
			observe.GlobalTrace("else: active")
			if len(parts) == 0 {
				observe.GlobalTrace("if: len(parts) == 0")
				verb = "Searched for"
			} else {
				observe.GlobalTrace("else: len(parts) == 0")
				verb = "searched for"
			}
		}
		parts = append(parts, fmt.Sprintf("%s %d %s", verb, searchCount, noun))
	}

	if readCount > 0 {
		observe.GlobalTrace("if: readCount > 0")
		noun := "file"
		if readCount > 1 {
			observe.GlobalTrace("if: readCount > 1")
			noun = "files"
		}
		var verb string
		if active {
			observe.GlobalTrace("if: active")
			if len(parts) == 0 {
				observe.GlobalTrace("if: len(parts) == 0")
				verb = "Reading"
			} else {
				observe.GlobalTrace("else: len(parts) == 0")
				verb = "reading"
			}
		} else {
			observe.GlobalTrace("else: active")
			if len(parts) == 0 {
				observe.GlobalTrace("if: len(parts) == 0")
				verb = "Read"
			} else {
				observe.GlobalTrace("else: len(parts) == 0")
				verb = "read"
			}
		}
		parts = append(parts, fmt.Sprintf("%s %d %s", verb, readCount, noun))
	}

	if len(parts) == 0 {
		observe.GlobalTrace("if: len(parts) == 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	text := strings.Join(parts, ", ")
	if active {
		observe.GlobalTrace("if: active")
		text += "\u2026"
	}
	observe.GlobalTrace("return: text")
	return text
}
