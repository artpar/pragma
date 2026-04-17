package render

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GroupEntry holds one tool call + result pair for rendering within a collapsed group.
type GroupEntry struct {
	CallHeader string          // pre-rendered "⏺ Read(file.go)\n"
	Name       string          // tool name
	Input      json.RawMessage // raw tool input
	Content    string          // tool result content
	IsError    bool
	Display    string // TUI-only display data (e.g., unified diff)
	HasResult  bool   // false while waiting for result
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
	if verbose {
		return renderGroupVerbose(g, width)
	}
	return renderGroupCollapsed(g, width)
}

// renderGroupVerbose renders each tool call and result individually.
func renderGroupVerbose(g GroupData, width int) string {
	var b strings.Builder
	for _, e := range g.Entries {
		b.WriteString(e.CallHeader)
		if e.HasResult {
			b.WriteString(RenderToolOutput(e.Name, e.Input, e.Content, e.IsError, width, e.Display, true))
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderGroupCollapsed renders the compact summary badge.
func renderGroupCollapsed(g GroupData, width int) string {
	var b strings.Builder

	summary := GenerateGroupSummary(g.SearchCount, g.ReadCount, g.Active)
	if summary == "" {
		// All counts zero (e.g., only "silent" entries like ToolSearch) — show nothing
		return ""
	}

	b.WriteString(bracketDim.Render(BracketPrefix))
	b.WriteString(dimText.Render(summary))
	b.WriteString("\n")

	// Show latest display hint when group is active (matching TS latestDisplayHint behavior)
	if g.Active && g.LatestHint != "" {
		b.WriteString(ContentIndent)
		b.WriteString(dimText.Render(g.LatestHint))
		b.WriteString("\n")
	}

	// CtrlOToExpand shown unconditionally (matching TS CollapsedReadSearchContent.tsx line 459)
	appendExpandHint(&b)

	return strings.TrimRight(b.String(), "\n")
}

// GenerateGroupSummary builds the summary text for a collapsed group.
// Matches TS getSearchReadSummaryText: proper tense, capitalization, grammar.
// Active: present tense + trailing "…" (U+2026). Completed: past tense.
func GenerateGroupSummary(searchCount, readCount int, active bool) string {
	var parts []string

	if searchCount > 0 {
		noun := "pattern"
		if searchCount > 1 {
			noun = "patterns"
		}
		var verb string
		if active {
			if len(parts) == 0 {
				verb = "Searching for"
			} else {
				verb = "searching for"
			}
		} else {
			if len(parts) == 0 {
				verb = "Searched for"
			} else {
				verb = "searched for"
			}
		}
		parts = append(parts, fmt.Sprintf("%s %d %s", verb, searchCount, noun))
	}

	if readCount > 0 {
		noun := "file"
		if readCount > 1 {
			noun = "files"
		}
		var verb string
		if active {
			if len(parts) == 0 {
				verb = "Reading"
			} else {
				verb = "reading"
			}
		} else {
			if len(parts) == 0 {
				verb = "Read"
			} else {
				verb = "read"
			}
		}
		parts = append(parts, fmt.Sprintf("%s %d %s", verb, readCount, noun))
	}

	if len(parts) == 0 {
		return ""
	}

	text := strings.Join(parts, ", ")
	if active {
		text += "\u2026" // U+2026 horizontal ellipsis, matching TS reference
	}
	return text
}
