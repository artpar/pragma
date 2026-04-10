package search

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

type searchInput struct {
	Query      string `json:"query" desc:"Search query or select:Name,Name2 for direct selection"`
	MaxResults int    `json:"max_results" desc:"Maximum results to return (default 5)"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["query"],
	"properties": {
		"query": {
			"type": "string",
			"description": "Search query. Use 'select:ToolA,ToolB' for direct selection, or keywords to search. Prefix a term with '+' to require it."
		},
		"max_results": {
			"type": "integer",
			"description": "Maximum number of results to return (default 5)",
			"default": 5
		}
	}
}`)

// Tool searches registered tools by name and description.
type Tool struct {
	Registry *tool.Registry
}

func (t *Tool) Name() string                { return "ToolSearch" }
func (t *Tool) Description() string          { return "Search for available tools by keyword or select specific tools by name." }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, "ToolSearch", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in searchInput
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

	var matches []matchResult

	// Fast path: select:Name,Name2
	if strings.HasPrefix(in.Query, "select:") {
		names := strings.Split(in.Query[len("select:"):], ",")
		matches = selectByNames(allTools, names)
	} else {
		matches = keywordSearch(allTools, in.Query, in.MaxResults)
	}

	type resultItem struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	type searchResult struct {
		Matches    []resultItem `json:"matches"`
		Query      string       `json:"query"`
		TotalTools int          `json:"total_tools"`
	}

	items := make([]resultItem, len(matches))
	for i, m := range matches {
		items[i] = resultItem{Name: m.name, Description: m.description}
	}

	out, _ := json.Marshal(searchResult{
		Matches:    items,
		Query:      in.Query,
		TotalTools: len(allTools),
	})
	return tool.InvokeResult{Content: string(out)}, nil
}

type matchResult struct {
	name        string
	description string
	score       int
}

// selectByNames returns tools matching the given names (case-insensitive).
func selectByNames(tools []tool.Descriptor, names []string) []matchResult {
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[strings.TrimSpace(strings.ToLower(n))] = true
	}
	var results []matchResult
	for _, td := range tools {
		if nameSet[strings.ToLower(td.Name())] {
			results = append(results, matchResult{
				name:        td.Name(),
				description: td.Description(),
			})
		}
	}
	return results
}

// keywordSearch scores tools against query terms.
func keywordSearch(tools []tool.Descriptor, query string, maxResults int) []matchResult {
	terms := parseTerms(query)
	if len(terms) == 0 {
		return nil
	}

	// Separate required (+term) from optional
	var required, optional []string
	for _, t := range terms {
		if strings.HasPrefix(t, "+") {
			required = append(required, strings.ToLower(t[1:]))
		} else {
			optional = append(optional, strings.ToLower(t))
		}
	}

	// If no required terms, all terms participate in scoring
	scoringTerms := optional
	if len(required) > 0 {
		scoringTerms = append(required, optional...)
	}

	type scored struct {
		matchResult
	}

	var results []scored
	for _, td := range tools {
		name := td.Name()
		desc := td.Description()
		nameLower := strings.ToLower(name)
		descLower := strings.ToLower(desc)
		parts := splitToolName(name)

		// Check required terms
		if len(required) > 0 {
			allFound := true
			for _, req := range required {
				if !containsTerm(nameLower, descLower, parts, req) {
					allFound = false
					break
				}
			}
			if !allFound {
				continue
			}
		}

		// Score against all terms
		totalScore := 0
		for _, term := range scoringTerms {
			totalScore += scoreTerm(nameLower, descLower, parts, term, strings.HasPrefix(name, "mcp__"))
		}

		if totalScore > 0 {
			results = append(results, scored{matchResult{
				name:        name,
				description: desc,
				score:       totalScore,
			}})
		}
	}

	// Sort by score descending (insertion sort, small N)
	for i := 1; i < len(results); i++ {
		key := results[i]
		j := i - 1
		for j >= 0 && results[j].score < key.score {
			results[j+1] = results[j]
			j--
		}
		results[j+1] = key
	}

	if len(results) > maxResults {
		results = results[:maxResults]
	}

	out := make([]matchResult, len(results))
	for i, r := range results {
		out[i] = r.matchResult
	}
	return out
}

// containsTerm checks if a term appears anywhere in the name or description.
func containsTerm(nameLower, descLower string, parts []string, term string) bool {
	if strings.Contains(nameLower, term) {
		return true
	}
	if strings.Contains(descLower, term) {
		return true
	}
	for _, p := range parts {
		if strings.Contains(strings.ToLower(p), term) {
			return true
		}
	}
	return false
}

// scoreTerm scores a single term against a tool's name and description.
// Matches TS scoring: exact part match +10/+12, partial part match +5/+6,
// full name match +3, description match +2.
func scoreTerm(nameLower, descLower string, parts []string, term string, isMCP bool) int {
	score := 0
	exactBonus := 10
	partialBonus := 5
	if isMCP {
		exactBonus = 12
		partialBonus = 6
	}

	// Check parts
	for _, p := range parts {
		pl := strings.ToLower(p)
		if pl == term {
			score += exactBonus
		} else if strings.Contains(pl, term) {
			score += partialBonus
		}
	}

	// Full name contains term (but not already scored via parts)
	if score == 0 && strings.Contains(nameLower, term) {
		score += 3
	}

	// Description match
	if strings.Contains(descLower, term) {
		score += 2
	}

	return score
}

// splitToolName splits a tool name into parts.
// MCP tools: mcp__server__action → ["mcp", "server", "action"]
// CamelCase: FileReadTool → ["File", "Read", "Tool"]
func splitToolName(name string) []string {
	// MCP naming convention
	if strings.Contains(name, "__") {
		return strings.Split(name, "__")
	}
	// CamelCase splitting
	return splitCamelCase(name)
}

// splitCamelCase splits a CamelCase string into parts.
func splitCamelCase(s string) []string {
	var parts []string
	var current strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// parseTerms splits a query into terms, preserving +prefix markers.
func parseTerms(query string) []string {
	fields := strings.Fields(query)
	var terms []string
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			terms = append(terms, f)
		}
	}
	return terms
}
