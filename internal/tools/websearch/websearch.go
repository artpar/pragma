package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// ToolName is the model-facing name of the web search tool.
const ToolName = "WebSearch"

// DefaultBaseURL is the real Brave Search API endpoint. Overridden via
// BRAVE_SEARCH_BASE_URL (mirroring OPENAI_BASE_URL) for hermetic tests.
const DefaultBaseURL = "https://api.search.brave.com"

const searchResultCount = 10
const searchTimeout = 10 * time.Second
const minQueryLen = 2

// Input defines the parameters for the WebSearch tool.
type Input struct {
	Query          string   `json:"query"`
	AllowedDomains []string `json:"allowed_domains,omitempty"`
	BlockedDomains []string `json:"blocked_domains,omitempty"`
}

// ToolDef returns the model tool definition (WEB-001, ported from the
// worktree branch's WebSearch tool).
func ToolDef() model.ToolDef {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: model.ToolDef{\n\tName:\tToolName,\n\tDescription: `Search the web for current inf...")
	return model.ToolDef{
		Name: ToolName,
		Description: `Search the web for current information.

Provides up-to-date information via web search (Brave Search API). Returns search results with titles, URLs, and descriptions.

When using search results, you MUST include a "Sources:" section at the end of your response with markdown hyperlinks to all sources used.`,
		InputSchema: json.RawMessage(`{
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
}`),
	}
}

// Execute runs one web search and returns formatted results for the model.
// apiKey is the Brave subscription token; baseURL may be empty for the
// default endpoint.
func Execute(ctx context.Context, apiKey, baseURL string, input json.RawMessage) (string, error) {
	observe.TraceCtx(ctx, "websearch", "Execute", "enter")
	defer observe.TraceCtx(ctx, "websearch", "Execute", "exit")
	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "websearch", "Execute", "if: err != nil")
		observe.TraceCtx(ctx, "websearch", "Execute", "return: \"\", fmt.Errorf(\"invalid WebSearch input: %w\", err)")
		return "", fmt.Errorf("invalid WebSearch input: %w", err)
	}

	in.Query = strings.TrimSpace(in.Query)
	if len(in.Query) < minQueryLen {
		observe.TraceCtx(ctx, "websearch", "Execute", "if: len(in.Query) < minQueryLen")
		observe.TraceCtx(ctx, "websearch", "Execute", "return: \"\", fmt.Errorf(\"query must be at least %d characters\", minQueryLen)")
		return "", fmt.Errorf("query must be at least %d characters", minQueryLen)
	}
	if len(in.AllowedDomains) > 0 && len(in.BlockedDomains) > 0 {
		observe.TraceCtx(ctx, "websearch", "Execute", "if: len(in.AllowedDomains) > 0 && len(in.BlockedDomains) > 0")
		observe.TraceCtx(ctx, "websearch", "Execute", "return: \"\", fmt.Errorf(\"cannot specify both allowed_domains and blocked_domains\")")
		return "", fmt.Errorf("cannot specify both allowed_domains and blocked_domains")
	}

	query := in.Query
	if len(in.AllowedDomains) > 0 {
		observe.TraceCtx(ctx, "websearch", "Execute", "if: len(in.AllowedDomains) > 0")
		siteParts := make([]string, len(in.AllowedDomains))
		for i, d := range in.AllowedDomains {
			observe.TraceCtx(ctx, "websearch", "Execute", "range in.AllowedDomains")
			siteParts[i] = "site:" + d
		}
		query = query + " " + strings.Join(siteParts, " OR ")
	}

	searchCtx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()

	start := time.Now()
	results, err := searchBrave(searchCtx, apiKey, baseURL, query, searchResultCount)
	duration := time.Since(start)
	if err != nil {
		observe.TraceCtx(ctx, "websearch", "Execute", "if: err != nil")
		observe.TraceCtx(ctx, "websearch", "Execute", "return: \"\", fmt.Errorf(\"web search failed: %w\", err)")
		return "", fmt.Errorf("web search failed: %w", err)
	}

	if len(in.BlockedDomains) > 0 {
		observe.TraceCtx(ctx, "websearch", "Execute", "if: len(in.BlockedDomains) > 0")
		results = filterBlockedDomains(results, in.BlockedDomains)
	}
	observe.TraceCtx(ctx, "websearch", "Execute", "return: FormatResults(in.Query, results, duration), nil")

	return FormatResults(in.Query, results, duration), nil
}

// filterBlockedDomains removes results whose URL hostname matches any
// blocked domain (exact or subdomain).
func filterBlockedDomains(results []Result, blocked []string) []Result {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	blockedSet := make(map[string]bool, len(blocked))
	for _, d := range blocked {
		observe.GlobalTrace("range blocked")
		blockedSet[strings.ToLower(d)] = true
	}

	filtered := make([]Result, 0, len(results))
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

// FormatResults builds the text content returned to the model.
func FormatResults(query string, results []Result, duration time.Duration) string {
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
