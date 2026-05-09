package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
)

// WebSearchInput defines the parameters for the WebSearch tool.
type WebSearchInput struct {
	Query          string   `json:"query" desc:"The web search query. Use natural language."`
	AllowedDomains []string `json:"allowed_domains,omitempty" desc:"Only include results from these domains"`
	BlockedDomains []string `json:"blocked_domains,omitempty" desc:"Exclude results from these domains"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
	"required": ["query"],
	"properties": {
		"query": {
			"type": "string",
			"description": "The web search query. Use natural language."
		},
		"allowed_domains": {
			"type": "array",
			"items": {"type": "string"},
			"description": "Only include results from these domains"
		},
		"blocked_domains": {
			"type": "array",
			"items": {"type": "string"},
			"description": "Exclude results from these domains"
		}
	}
}`)

// Tool implements the WebSearch tool using the Brave Search API.
type Tool struct {
	Token string // Optional Brave Search API key (overrides env var)
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"WebSearch\"")
	return "WebSearch"
}

func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: `Search the web for current information.\n\nProvides up-to-date information via...")
	return `Search the web for current information.

Provides up-to-date information via web search. Returns search results with titles, URLs, and descriptions.

Requires BRAVE_SEARCH_API_KEY environment variable or brave.api_key in ~/.pragma/credentials.yml.

When using search results, you MUST include a "Sources:" section at the end of your response with markdown hyperlinks to all sources used.`
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

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "websearch", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "websearch", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "websearch", "Tool.CheckPerm", "return: checker.Check(ctx, \"WebSearch\", \"\")")
	return checker.Check(ctx, "WebSearch", "")
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "websearch", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "websearch", "Tool.Invoke", "exit")

	var in WebSearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "websearch", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "websearch", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Invalid input: %v\", err)}, nil")
		return tool.InvokeResult{Content: fmt.Sprintf("Invalid input: %v", err)}, nil
	}

	in.Query = strings.TrimSpace(in.Query)
	if len(in.Query) < 2 {
		observe.TraceCtx(ctx, "websearch", "Tool.Invoke", "if: len(in.Query) < 2")
		observe.TraceCtx(ctx, "websearch", "Tool.Invoke", "return: tool.InvokeResult{Content: \"Query must be at least 2 characters.\"}, nil")
		return tool.InvokeResult{Content: "Query must be at least 2 characters."}, nil
	}
	if len(in.AllowedDomains) > 0 && len(in.BlockedDomains) > 0 {
		observe.TraceCtx(ctx, "websearch", "Tool.Invoke", "if: len(in.AllowedDomains) > 0 && len(in.BlockedDomains) > 0")
		observe.TraceCtx(ctx, "websearch", "Tool.Invoke", "return: tool.InvokeResult{Content: \"Cannot specify both allowed_domains and blocked_d...")
		return tool.InvokeResult{Content: "Cannot specify both allowed_domains and blocked_domains."}, nil
	}

	apiKey := t.Token
	if apiKey == "" {
		apiKey = os.Getenv("BRAVE_SEARCH_API_KEY")
	}
	if apiKey == "" {
		observe.TraceCtx(ctx, "websearch", "Tool.Invoke", "if: apiKey == \"\"")
		observe.TraceCtx(ctx, "websearch", "Tool.Invoke", "return: tool.InvokeResult{Content: \"BRAVE_SEARCH_API_KEY environment variable is not ...")
		return tool.InvokeResult{Content: "BRAVE_SEARCH_API_KEY environment variable or brave.api_key in ~/.pragma/credentials.yml is required. Web search is unavailable."}, nil
	}

	query := in.Query
	if len(in.AllowedDomains) > 0 {
		observe.TraceCtx(ctx, "websearch", "Tool.Invoke", "appending site: filters")
		siteParts := make([]string, len(in.AllowedDomains))
		for i, d := range in.AllowedDomains {
			observe.TraceCtx(ctx, "websearch", "Tool.Invoke", "range in.AllowedDomains")
			siteParts[i] = "site:" + d
		}
		query = query + " " + strings.Join(siteParts, " OR ")
	}

	searchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	start := time.Now()
	results, err := searchBrave(searchCtx, apiKey, query, 10)
	duration := time.Since(start)

	if err != nil {
		observe.TraceCtx(ctx, "websearch", "Tool.Invoke", "search error: "+err.Error())
		observe.TraceCtx(ctx, "websearch", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Web search failed: %v\", err)}, nil")
		return tool.InvokeResult{Content: fmt.Sprintf("Web search failed: %v", err)}, nil
	}

	if len(in.BlockedDomains) > 0 {
		observe.TraceCtx(ctx, "websearch", "Tool.Invoke", "filtering blocked domains")
		results = filterBlockedDomains(results, in.BlockedDomains)
	}

	content := formatResults(in.Query, results, duration)
	observe.TraceCtx(ctx, "websearch", "Tool.Invoke", "return: tool.InvokeResult{Content: content}, nil")
	return tool.InvokeResult{Content: content}, nil
}

// filterBlockedDomains removes results whose URL hostname matches any blocked domain.
func filterBlockedDomains(results []braveResult, blocked []string) []braveResult {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	blockedSet := make(map[string]bool, len(blocked))
	for _, d := range blocked {
		observe.GlobalTrace("range blocked")
		blockedSet[strings.ToLower(d)] = true
	}

	filtered := make([]braveResult, 0, len(results))
	for _, r := range results {
		observe.GlobalTrace("range results")
		parsed, err := url.Parse(r.URL)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			continue
		}
		host := strings.ToLower(parsed.Hostname())
		if blockedSet[host] {
			observe.GlobalTrace("if: blockedSet[host]")
			continue
		}

		skip := false
		for d := range blockedSet {
			observe.GlobalTrace("range blockedSet")
			if strings.HasSuffix(host, "."+d) {
				observe.GlobalTrace("if: strings.HasSuffix(host, \".\"+d)")
				skip = true
				break
			}
		}
		if !skip {
			observe.GlobalTrace("if: !skip")
			filtered = append(filtered, r)
		}
	}
	observe.GlobalTrace("return: filtered")
	return filtered
}

// formatResults builds the text content returned to the LLM.
func formatResults(query string, results []braveResult, duration time.Duration) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(results) == 0 {
		observe.GlobalTrace("if: len(results) == 0")
		observe.GlobalTrace("return: fmt.Sprintf(\"No web search results found for query: %q\", query)")
		return fmt.Sprintf("No web search results found for query: %q", query)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Web search results for query: %q (%.1fs)\n\n", query, duration.Seconds())

	for i, r := range results {
		observe.GlobalTrace("range results")
		fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, r.Title, r.URL)
		if r.Description != "" {
			observe.GlobalTrace("if: r.Description != \"\"")
			fmt.Fprintf(&b, "   %s\n", r.Description)
		}
		b.WriteString("\n")
	}

	b.WriteString("REMINDER: You MUST cite the sources above in your response using markdown hyperlinks.")
	observe.GlobalTrace("return: b.String()")
	return b.String()
}
