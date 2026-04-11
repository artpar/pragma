package toolsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

type toolSearchInput struct {
	Query      string `json:"query" desc:"Search query — use 'select:Name1,Name2' for exact match, or keywords for fuzzy search"`
	MaxResults int    `json:"max_results" desc:"Maximum results to return (default 5)"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["query"],
	"properties": {
		"query": {
			"type": "string",
			"description": "Search query. Use 'select:Name1,Name2' for direct selection, '+keyword' for required terms, or plain keywords for fuzzy search."
		},
		"max_results": {
			"type": "integer",
			"description": "Maximum number of results to return (default: 5)",
			"default": 5
		}
	}
}`)

// Tool searches available tools by keyword or direct selection.
type Tool struct {
	Registry *tool.Registry
}

func (t *Tool) Name() string                { return "ToolSearch" }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) Description() string {
	return `Fetches full schema definitions for deferred tools so they can be called. Use "select:Read,Edit,Grep" for direct selection, or keywords to search.`
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, "ToolSearch", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in toolSearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Query == "" {
		return tool.InvokeResult{}, fmt.Errorf("query is required")
	}
	if in.MaxResults <= 0 {
		in.MaxResults = 5
	}

	allTools := t.Registry.List()

	// Mode 1: Direct selection (select:Name1,Name2)
	if strings.HasPrefix(in.Query, "select:") {
		names := strings.Split(strings.TrimPrefix(in.Query, "select:"), ",")
		var matches []string
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, ok := t.Registry.Get(name); ok {
				matches = append(matches, name)
			}
		}
		return t.formatResult(matches, in.Query, len(allTools))
	}

	// Mode 2: Exact name match
	if _, ok := t.Registry.Get(in.Query); ok {
		return t.formatResult([]string{in.Query}, in.Query, len(allTools))
	}

	// Mode 3: MCP prefix match
	if strings.HasPrefix(in.Query, "mcp__") || strings.HasPrefix(in.Query, "+mcp__") {
		prefix := strings.TrimPrefix(in.Query, "+")
		var matches []string
		for _, td := range allTools {
			if strings.HasPrefix(td.Name(), prefix) {
				matches = append(matches, td.Name())
			}
		}
		if len(matches) > in.MaxResults {
			matches = matches[:in.MaxResults]
		}
		return t.formatResult(matches, in.Query, len(allTools))
	}

	// Mode 4: Keyword search with scoring
	type scored struct {
		name  string
		score int
	}

	terms := strings.Fields(strings.ToLower(in.Query))
	var required []string
	var optional []string
	for _, term := range terms {
		if strings.HasPrefix(term, "+") {
			required = append(required, strings.TrimPrefix(term, "+"))
		} else {
			optional = append(optional, term)
		}
	}

	var results []scored
	for _, td := range allTools {
		name := td.Name()
		parts := parseToolName(name)
		desc := strings.ToLower(td.Description())
		nameLower := strings.ToLower(name)

		score := 0
		requiredMet := true

		for _, term := range required {
			termScore := scoreTerm(term, parts, nameLower, desc)
			if termScore == 0 {
				requiredMet = false
				break
			}
			score += termScore
		}

		if !requiredMet {
			continue
		}

		for _, term := range optional {
			score += scoreTerm(term, parts, nameLower, desc)
		}

		if score > 0 {
			results = append(results, scored{name: name, score: score})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	var matches []string
	for i, r := range results {
		if i >= in.MaxResults {
			break
		}
		matches = append(matches, r.name)
	}

	return t.formatResult(matches, in.Query, len(allTools))
}

func (t *Tool) formatResult(matches []string, query string, totalTools int) (tool.InvokeResult, error) {
	result := struct {
		Matches    []string `json:"matches"`
		Query      string   `json:"query"`
		TotalTools int      `json:"total_tools"`
	}{
		Matches:    matches,
		Query:      query,
		TotalTools: totalTools,
	}
	if result.Matches == nil {
		result.Matches = []string{}
	}
	data, _ := json.Marshal(result)
	return tool.InvokeResult{Content: string(data)}, nil
}

// parseToolName splits a tool name into searchable parts.
// CamelCase → ["file", "read"], MCP names → split by "__" and "_".
func parseToolName(name string) []string {
	if strings.Contains(name, "__") {
		// MCP tool: mcp__server__action → split by __ and _
		raw := strings.Split(name, "__")
		var parts []string
		for _, r := range raw {
			for _, p := range strings.Split(r, "_") {
				if p != "" {
					parts = append(parts, strings.ToLower(p))
				}
			}
		}
		return parts
	}

	// CamelCase split
	var parts []string
	var current strings.Builder
	for _, r := range name {
		if unicode.IsUpper(r) && current.Len() > 0 {
			parts = append(parts, strings.ToLower(current.String()))
			current.Reset()
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		parts = append(parts, strings.ToLower(current.String()))
	}
	return parts
}

func scoreTerm(term string, parts []string, nameLower, desc string) int {
	score := 0

	// Exact part match (highest)
	for _, p := range parts {
		if p == term {
			score += 10
			break
		}
	}

	// Substring part match
	if score == 0 {
		for _, p := range parts {
			if strings.Contains(p, term) {
				score += 5
				break
			}
		}
	}

	// Full name fallback
	if score == 0 && strings.Contains(nameLower, term) {
		score += 3
	}

	// Description word-boundary match
	if strings.Contains(desc, term) {
		score += 2
	}

	return score
}
