package toolsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/artpar/gogent/internal/observe"
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

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"ToolSearch\"")
	observe.GlobalTrace("return: \"ToolSearch\"")
	observe.GlobalTrace("return: \"ToolSearch\"")
	return "ToolSearch"
}
func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	observe.GlobalTrace("return: inputSchema")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Fetches full schema definitions for deferred tools so they can be called...\"")
	observe.GlobalTrace("return: toolSearchDescription")
	observe.GlobalTrace("return: toolSearchDescription")
	return toolSearchDescription
}

const toolSearchDescription = `Fetches full schema definitions for deferred tools so they can be called.

Deferred tools appear by name in <system-reminder> messages. Until fetched, only the name is known — there is no parameter schema, so the tool cannot be invoked. This tool takes a query, matches it against the deferred tool list, and returns the matched tools' complete JSONSchema definitions inside a <functions> block. Once a tool's schema appears in that result, it is callable exactly like any tool defined at the top of the prompt.

Result format: each matched tool appears as one <function>{"description": "...", "name": "...", "parameters": {...}}</function> line inside the <functions> block — the same encoding as the tool list at the top of this prompt.

Query forms:
- "select:Read,Edit,Grep" — fetch these exact tools by name
- "notebook jupyter" — keyword search, up to max_results best matches
- "+slack send" — require "slack" in the name, rank by remaining terms`

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "toolsearch", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "toolsearch", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "toolsearch", "Tool.CheckPerm", "return: checker.Check(ctx, \"ToolSearch\", \"\")")
	observe.TraceCtx(ctx, "toolsearch", "Tool.CheckPerm", "return: checker.Check(ctx, \"ToolSearch\", \"\")")
	observe.TraceCtx(ctx, "toolsearch", "Tool.CheckPerm", "return: checker.Check(ctx, \"ToolSearch\", \"\")")
	return checker.Check(ctx, "ToolSearch", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var in toolSearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Query == "" {
		observe.GlobalTrace("if: in.Query == \"\"")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"query is required\")")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"query is required\")")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"query is required\")")
		return tool.InvokeResult{}, fmt.Errorf("query is required")
	}
	if in.MaxResults <= 0 {
		observe.GlobalTrace("if: in.MaxResults <= 0")
		in.MaxResults = 5
	}

	allTools := t.Registry.List()

	if strings.HasPrefix(in.Query, "select:") {
		observe.GlobalTrace("if: strings.HasPrefix(in.Query, \"select:\")")
		names := strings.Split(strings.TrimPrefix(in.Query, "select:"), ",")
		var matches []string
		for _, name := range names {
			observe.GlobalTrace("range names")
			name = strings.TrimSpace(name)
			if name == "" {
				observe.GlobalTrace("if: name == \"\"")
				continue
			}
			if _, ok := t.Registry.Get(name); ok {
				observe.GlobalTrace("if: ok")
				matches = append(matches, name)
			} else {
				observe.GlobalTrace("else: ok")

				suffix := "__" + name
				for _, td := range allTools {
					observe.GlobalTrace("range allTools")
					if strings.HasSuffix(td.Name(), suffix) {
						observe.GlobalTrace("if: strings.HasSuffix(td.Name(), suffix)")
						matches = append(matches, td.Name())
					}
				}
			}
		}
		observe.GlobalTrace("return: t.formatResult(matches, in.Query, len(allTools))")
		observe.GlobalTrace("return: t.formatResult(matches, in.Query, len(allTools))")
		observe.GlobalTrace("return: t.formatResult(matches, in.Query, len(allTools))")
		return t.formatResult(matches, in.Query, len(allTools))
	}

	if _, ok := t.Registry.Get(in.Query); ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: t.formatResult([]string{in.Query}, in.Query, len(allTools))")
		observe.GlobalTrace("return: t.formatResult([]string{in.Query}, in.Query, len(allTools))")
		observe.GlobalTrace("return: t.formatResult([]string{in.Query}, in.Query, len(allTools))")
		return t.formatResult([]string{in.Query}, in.Query, len(allTools))
	}

	if strings.HasPrefix(in.Query, "mcp__") || strings.HasPrefix(in.Query, "+mcp__") {
		observe.GlobalTrace("if: strings.HasPrefix(in.Query, \"mcp__\") || strings.HasPrefix(in.Query, \"+mcp__\")")
		prefix := strings.TrimPrefix(in.Query, "+")
		var matches []string
		for _, td := range allTools {
			observe.GlobalTrace("range allTools")
			if strings.HasPrefix(td.Name(), prefix) {
				observe.GlobalTrace("if: strings.HasPrefix(td.Name(), prefix)")
				matches = append(matches, td.Name())
			}
		}
		if len(matches) > in.MaxResults {
			observe.GlobalTrace("if: len(matches) > in.MaxResults")
			matches = matches[:in.MaxResults]
		}
		observe.GlobalTrace("return: t.formatResult(matches, in.Query, len(allTools))")
		observe.GlobalTrace("return: t.formatResult(matches, in.Query, len(allTools))")
		observe.GlobalTrace("return: t.formatResult(matches, in.Query, len(allTools))")
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
		observe.GlobalTrace("range terms")
		if strings.HasPrefix(term, "+") {
			observe.GlobalTrace("if: strings.HasPrefix(term, \"+\")")
			required = append(required, strings.TrimPrefix(term, "+"))
		} else {
			observe.GlobalTrace("else: strings.HasPrefix(term, \"+\")")
			optional = append(optional, term)
		}
	}

	var results []scored
	for _, td := range allTools {
		observe.GlobalTrace("range allTools")
		name := td.Name()
		parts := parseToolName(name)
		desc := strings.ToLower(td.Description())
		nameLower := strings.ToLower(name)

		score := 0
		requiredMet := true

		for _, term := range required {
			observe.GlobalTrace("range required")
			termScore := scoreTerm(term, parts, nameLower, desc)
			if termScore == 0 {
				observe.GlobalTrace("if: termScore == 0")
				requiredMet = false
				break
			}
			score += termScore
		}

		if !requiredMet {
			observe.GlobalTrace("if: !requiredMet")
			continue
		}

		for _, term := range optional {
			observe.GlobalTrace("range optional")
			score += scoreTerm(term, parts, nameLower, desc)
		}

		if score > 0 {
			observe.GlobalTrace("if: score > 0")
			results = append(results, scored{name: name, score: score})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	var matches []string
	for i, r := range results {
		observe.GlobalTrace("range results")
		if i >= in.MaxResults {
			observe.GlobalTrace("if: i >= in.MaxResults")
			break
		}
		matches = append(matches, r.name)
	}
	observe.GlobalTrace("return: t.formatResult(matches, in.Query, len(allTools))")
	observe.GlobalTrace("return: t.formatResult(matches, in.Query, len(allTools))")
	observe.GlobalTrace("return: t.formatResult(matches, in.Query, len(allTools))")

	return t.formatResult(matches, in.Query, len(allTools))
}

func (t *Tool) formatResult(matches []string, query string, totalTools int) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
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
		observe.GlobalTrace("if: result.Matches == nil")
		result.Matches = []string{}
	}
	data, _ := json.Marshal(result)
	observe.GlobalTrace("return: tool.InvokeResult{Content: string(data)}, nil")
	observe.GlobalTrace("return: tool.InvokeResult{Content: string(data)}, nil")
	observe.GlobalTrace("return: tool.InvokeResult{Content: string(data)}, nil")
	return tool.InvokeResult{Content: string(data)}, nil
}

// parseToolName splits a tool name into searchable parts.
// CamelCase → ["file", "read"], MCP names → split by "__" and "_".
func parseToolName(name string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if strings.Contains(name, "__") {
		observe.GlobalTrace("if: strings.Contains(name, \"__\")")

		raw := strings.Split(name, "__")
		var parts []string
		for _, r := range raw {
			observe.GlobalTrace("range raw")
			for _, p := range strings.Split(r, "_") {
				observe.GlobalTrace("range strings.Split(r, \"_\")")
				if p != "" {
					observe.GlobalTrace("if: p != \"\"")
					parts = append(parts, strings.ToLower(p))
				}
			}
		}
		observe.GlobalTrace("return: parts")
		observe.GlobalTrace("return: parts")
		observe.GlobalTrace("return: parts")
		return parts
	}

	// CamelCase split
	var parts []string
	var current strings.Builder
	for _, r := range name {
		observe.GlobalTrace("range name")
		if unicode.IsUpper(r) && current.Len() > 0 {
			observe.GlobalTrace("if: unicode.IsUpper(r) && current.Len() > 0")
			parts = append(parts, strings.ToLower(current.String()))
			current.Reset()
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		observe.GlobalTrace("if: current.Len() > 0")
		parts = append(parts, strings.ToLower(current.String()))
	}
	observe.GlobalTrace("return: parts")
	observe.GlobalTrace("return: parts")
	observe.GlobalTrace("return: parts")
	return parts
}

func scoreTerm(term string, parts []string, nameLower, desc string) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	score := 0

	for _, p := range parts {
		observe.GlobalTrace("range parts")
		if p == term {
			observe.GlobalTrace("if: p == term")
			score += 10
			break
		}
	}

	if score == 0 {
		observe.GlobalTrace("if: score == 0")
		for _, p := range parts {
			observe.GlobalTrace("range parts")
			if strings.Contains(p, term) {
				observe.GlobalTrace("if: strings.Contains(p, term)")
				score += 5
				break
			}
		}
	}

	if score == 0 && strings.Contains(nameLower, term) {
		observe.GlobalTrace("if: score == 0 && strings.Contains(nameLower, term)")
		score += 3
	}

	if strings.Contains(desc, term) {
		observe.GlobalTrace("if: strings.Contains(desc, term)")
		score += 2
	}
	observe.GlobalTrace("return: score")
	observe.GlobalTrace("return: score")
	observe.GlobalTrace("return: score")

	return score
}
