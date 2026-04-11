package search

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/artpar/gogent/internal/observe"
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

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"ToolSearch\"")
	return "ToolSearch"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Search for available tools by keyword or select specific tools by name.\"")
	return "Search for available tools by keyword or select specific tools by name."
}
func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "search", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "search", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "search", "Tool.CheckPerm", "return: checker.Check(ctx, \"ToolSearch\", \"\")")
	return checker.Check(ctx, "ToolSearch", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var in searchInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Query == "" {
		observe.GlobalTrace("if: in.Query == \"\"")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"query is required\")")
		return tool.InvokeResult{}, fmt.Errorf("query is required")
	}
	if in.MaxResults <= 0 {
		observe.GlobalTrace("if: in.MaxResults <= 0")
		in.MaxResults = 5
	}

	allTools := t.Registry.List()

	var matches []matchResult

	if strings.HasPrefix(in.Query, "select:") {
		observe.GlobalTrace("if: strings.HasPrefix(in.Query, \"select:\")")
		names := strings.Split(in.Query[len("select:"):], ",")
		matches = selectByNames(allTools, names)
	} else {
		observe.GlobalTrace("else: strings.HasPrefix(in.Query, \"select:\")")
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
		observe.GlobalTrace("range matches")
		items[i] = resultItem{Name: m.name, Description: m.description}
	}

	out, _ := json.Marshal(searchResult{
		Matches:    items,
		Query:      in.Query,
		TotalTools: len(allTools),
	})
	observe.GlobalTrace("return: tool.InvokeResult{Content: string(out)}, nil")
	return tool.InvokeResult{Content: string(out)}, nil
}

type matchResult struct {
	name        string
	description string
	score       int
}

// selectByNames returns tools matching the given names (case-insensitive).
func selectByNames(tools []tool.Descriptor, names []string) []matchResult {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		observe.GlobalTrace("range names")
		nameSet[strings.TrimSpace(strings.ToLower(n))] = true
	}
	var results []matchResult
	for _, td := range tools {
		observe.GlobalTrace("range tools")
		if nameSet[strings.ToLower(td.Name())] {
			observe.GlobalTrace("if: nameSet[strings.ToLower(td.Name())]")
			results = append(results, matchResult{
				name:        td.Name(),
				description: td.Description(),
			})
		}
	}
	observe.GlobalTrace("return: results")
	return results
}

// keywordSearch scores tools against query terms.
func keywordSearch(tools []tool.Descriptor, query string, maxResults int) []matchResult {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	terms := parseTerms(query)
	if len(terms) == 0 {
		observe.GlobalTrace("if: len(terms) == 0")
		observe.GlobalTrace("return: nil")
		return nil
	}

	// Separate required (+term) from optional
	var required, optional []string
	for _, t := range terms {
		observe.GlobalTrace("range terms")
		if strings.HasPrefix(t, "+") {
			observe.GlobalTrace("if: strings.HasPrefix(t, \"+\")")
			required = append(required, strings.ToLower(t[1:]))
		} else {
			observe.GlobalTrace("else: strings.HasPrefix(t, \"+\")")
			optional = append(optional, strings.ToLower(t))
		}
	}

	scoringTerms := optional
	if len(required) > 0 {
		observe.GlobalTrace("if: len(required) > 0")
		scoringTerms = append(required, optional...)
	}

	type scored struct {
		matchResult
	}

	var results []scored
	for _, td := range tools {
		observe.GlobalTrace("range tools")
		name := td.Name()
		desc := td.Description()
		nameLower := strings.ToLower(name)
		descLower := strings.ToLower(desc)
		parts := splitToolName(name)

		if len(required) > 0 {
			observe.GlobalTrace("if: len(required) > 0")
			allFound := true
			for _, req := range required {
				observe.GlobalTrace("range required")
				if !containsTerm(nameLower, descLower, parts, req) {
					observe.GlobalTrace("if: !containsTerm(nameLower, descLower, parts, req)")
					allFound = false
					break
				}
			}
			if !allFound {
				observe.GlobalTrace("if: !allFound")
				continue
			}
		}

		totalScore := 0
		for _, term := range scoringTerms {
			observe.GlobalTrace("range scoringTerms")
			totalScore += scoreTerm(nameLower, descLower, parts, term, strings.HasPrefix(name, "mcp__"))
		}

		if totalScore > 0 {
			observe.GlobalTrace("if: totalScore > 0")
			results = append(results, scored{matchResult{
				name:        name,
				description: desc,
				score:       totalScore,
			}})
		}
	}

	for i := 1; i < len(results); i++ {
		observe.GlobalTrace("for: i < len(results)")
		key := results[i]
		j := i - 1
		for j >= 0 && results[j].score < key.score {
			observe.GlobalTrace("for: j >= 0 && results[j].score < key.score")
			results[j+1] = results[j]
			j--
		}
		results[j+1] = key
	}

	if len(results) > maxResults {
		observe.GlobalTrace("if: len(results) > maxResults")
		results = results[:maxResults]
	}

	out := make([]matchResult, len(results))
	for i, r := range results {
		observe.GlobalTrace("range results")
		out[i] = r.matchResult
	}
	observe.GlobalTrace("return: out")
	return out
}

// containsTerm checks if a term appears anywhere in the name or description.
func containsTerm(nameLower, descLower string, parts []string, term string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if strings.Contains(nameLower, term) {
		observe.GlobalTrace("if: strings.Contains(nameLower, term)")
		observe.GlobalTrace("return: true")
		return true
	}
	if strings.Contains(descLower, term) {
		observe.GlobalTrace("if: strings.Contains(descLower, term)")
		observe.GlobalTrace("return: true")
		return true
	}
	for _, p := range parts {
		observe.GlobalTrace("range parts")
		if strings.Contains(strings.ToLower(p), term) {
			observe.GlobalTrace("if: strings.Contains(strings.ToLower(p), term)")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

// scoreTerm scores a single term against a tool's name and description.
// Matches TS scoring: exact part match +10/+12, partial part match +5/+6,
// full name match +3, description match +2.
func scoreTerm(nameLower, descLower string, parts []string, term string, isMCP bool) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	score := 0
	exactBonus := 10
	partialBonus := 5
	if isMCP {
		observe.GlobalTrace("if: isMCP")
		exactBonus = 12
		partialBonus = 6
	}

	for _, p := range parts {
		observe.GlobalTrace("range parts")
		pl := strings.ToLower(p)
		if pl == term {
			observe.GlobalTrace("if: pl == term")
			score += exactBonus
		} else if strings.Contains(pl, term) {
			observe.GlobalTrace("else-if: strings.Contains(pl, term)")
			score += partialBonus
		}
	}

	if score == 0 && strings.Contains(nameLower, term) {
		observe.GlobalTrace("if: score == 0 && strings.Contains(nameLower, term)")
		score += 3
	}

	if strings.Contains(descLower, term) {
		observe.GlobalTrace("if: strings.Contains(descLower, term)")
		score += 2
	}
	observe.GlobalTrace("return: score")

	return score
}

// splitToolName splits a tool name into parts.
// MCP tools: mcp__server__action → ["mcp", "server", "action"]
// CamelCase: FileReadTool → ["File", "Read", "Tool"]
func splitToolName(name string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	if strings.Contains(name, "__") {
		observe.GlobalTrace("if: strings.Contains(name, \"__\")")
		observe.GlobalTrace("return: strings.Split(name, \"__\")")
		return strings.Split(name, "__")
	}
	observe.GlobalTrace("return: splitCamelCase(name)")

	return splitCamelCase(name)
}

// splitCamelCase splits a CamelCase string into parts.
func splitCamelCase(s string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var parts []string
	var current strings.Builder
	for i, r := range s {
		observe.GlobalTrace("range s")
		if i > 0 && unicode.IsUpper(r) {
			observe.GlobalTrace("if: i > 0 && unicode.IsUpper(r)")
			if current.Len() > 0 {
				observe.GlobalTrace("if: current.Len() > 0")
				parts = append(parts, current.String())
				current.Reset()
			}
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		observe.GlobalTrace("if: current.Len() > 0")
		parts = append(parts, current.String())
	}
	observe.GlobalTrace("return: parts")
	return parts
}

// parseTerms splits a query into terms, preserving +prefix markers.
func parseTerms(query string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	fields := strings.Fields(query)
	var terms []string
	for _, f := range fields {
		observe.GlobalTrace("range fields")
		f = strings.TrimSpace(f)
		if f != "" {
			observe.GlobalTrace("if: f != \"\"")
			terms = append(terms, f)
		}
	}
	observe.GlobalTrace("return: terms")
	return terms
}
