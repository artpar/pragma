package webfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/provider"
	"github.com/artpar/gogent/internal/tool"
)

const (
	fetchTimeout     = 60 * time.Second
	maxBodyBytes     = 10 * 1024 * 1024  // 10 MB
	maxMarkdownChars = 100_000           // truncate markdown to this
	cacheMaxSize     = 50 * 1024 * 1024  // 50 MB
	cacheTTL         = 15 * time.Minute
	// Use a fast, cheap model for summarization (not the parent's model)
	secondaryModel = "claude-haiku-4-5-20251001"
)

type WebFetchInput struct {
	URL    string `json:"url" desc:"URL to fetch"`
	Prompt string `json:"prompt" desc:"What to extract from the page"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["url", "prompt"],
	"properties": {
		"url": {
			"type": "string",
			"description": "The URL to fetch content from"
		},
		"prompt": {
			"type": "string",
			"description": "The prompt to run on the fetched content"
		}
	}
}`)

type fetchResult struct {
	URL        string `json:"url"`
	Code       int    `json:"code"`
	CodeText   string `json:"code_text"`
	Bytes      int    `json:"bytes"`
	Result     string `json:"result"`
	DurationMs int64  `json:"duration_ms"`
}

// Tool implements the WebFetch tool for fetching and processing web content.
type Tool struct {
	Provider  provider.Provider
	Bus       *observe.EventBus
	cache     *urlCache
	cacheOnce sync.Once
}

func (t *Tool) Name() string                { return "WebFetch" }
func (t *Tool) Description() string          { return "Fetch a URL and process the content with a prompt." }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	var in struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return checker.Check(ctx, "WebFetch", "")
	}
	u, err := url.Parse(in.URL)
	if err != nil || u.Hostname() == "" {
		return checker.Check(ctx, "WebFetch", "")
	}
	return checker.Check(ctx, "WebFetch", "domain:"+u.Hostname())
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in WebFetchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.URL == "" {
		return tool.InvokeResult{}, fmt.Errorf("url is required")
	}
	if in.Prompt == "" {
		return tool.InvokeResult{}, fmt.Errorf("prompt is required")
	}

	// Validate URL
	u, err := url.Parse(in.URL)
	if err != nil {
		return tool.InvokeResult{Content: fmt.Sprintf("Failed to parse URL: %v", err)}, nil
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return tool.InvokeResult{Content: "URL must use http or https scheme"}, nil
	}
	if u.User != nil {
		return tool.InvokeResult{Content: "URL must not contain credentials"}, nil
	}

	// Upgrade HTTP to HTTPS
	if u.Scheme == "http" {
		u.Scheme = "https"
	}
	fetchURL := u.String()

	// Init cache lazily (thread-safe)
	t.cacheOnce.Do(func() {
		t.cache = newURLCache(cacheMaxSize, cacheTTL)
	})

	start := time.Now()

	// Check cache
	if cached, ok := t.cache.Get(fetchURL); ok {
		result, err := t.summarize(ctx, cached, in.Prompt)
		if err != nil {
			return tool.InvokeResult{Content: fmt.Sprintf("Failed to process content: %v", err)}, nil
		}
		fr := fetchResult{
			URL:        fetchURL,
			Code:       200,
			CodeText:   "OK (cached)",
			Bytes:      len(cached),
			Result:     result,
			DurationMs: time.Since(start).Milliseconds(),
		}
		return marshalResult(fr), nil
	}

	// Fetch
	httpCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(httpCtx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return tool.InvokeResult{Content: fmt.Sprintf("Failed to create request: %v", err)}, nil
	}
	req.Header.Set("User-Agent", "gogent/1.0 (AI coding assistant)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return tool.InvokeResult{Content: fmt.Sprintf("Failed to fetch URL: %v", err)}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return tool.InvokeResult{Content: fmt.Sprintf("Failed to read response: %v", err)}, nil
	}

	contentType := resp.Header.Get("Content-Type")
	var markdown string

	switch {
	case strings.Contains(contentType, "text/html"):
		markdown, err = htmltomarkdown.ConvertString(string(body))
		if err != nil {
			// Fallback to raw text
			markdown = string(body)
		}
	case strings.Contains(contentType, "text/"):
		markdown = string(body)
	default:
		markdown = fmt.Sprintf("[Binary content: %s, %d bytes]", contentType, len(body))
	}

	// Truncate
	if len(markdown) > maxMarkdownChars {
		markdown = markdown[:maxMarkdownChars] + "\n\n[Content truncated]"
	}

	// Cache the markdown content
	t.cache.Set(fetchURL, markdown)

	// Summarize with secondary model
	result, err := t.summarize(ctx, markdown, in.Prompt)
	if err != nil {
		return tool.InvokeResult{Content: fmt.Sprintf("Failed to process content: %v", err)}, nil
	}

	fr := fetchResult{
		URL:        fetchURL,
		Code:       resp.StatusCode,
		CodeText:   resp.Status,
		Bytes:      len(body),
		Result:     result,
		DurationMs: time.Since(start).Milliseconds(),
	}
	return marshalResult(fr), nil
}

// summarize calls the provider with a secondary prompt to process fetched content.
func (t *Tool) summarize(ctx context.Context, content, userPrompt string) (string, error) {
	if t.Provider == nil {
		// No provider available — return raw content excerpt
		if len(content) > 2000 {
			return content[:2000] + "\n[truncated]", nil
		}
		return content, nil
	}

	messages := []model.Message{
		{
			ID:   model.NewUUID(),
			Role: model.RoleUser,
			Content: []model.ContentPart{model.TextPart{Text: fmt.Sprintf(
				"Here is web page content:\n\n%s\n\n---\nBased on the above content, answer: %s", content, userPrompt,
			)}},
		},
	}

	resp, err := t.Provider.Complete(ctx, provider.RequestParams{
		Model:     secondaryModel,
		MaxTokens: 4096,
		Messages:  messages,
		System: model.SystemPrompt{
			Blocks: []model.SystemBlock{{
				Text: "You are a helpful assistant that answers questions based on provided web content. Be concise and accurate.",
			}},
		},
	})
	if err != nil {
		return "", fmt.Errorf("secondary model call: %w", err)
	}

	// Extract text from response
	var result strings.Builder
	for _, part := range resp.Content {
		if tp, ok := part.(model.TextPart); ok {
			result.WriteString(tp.Text)
		}
	}
	return result.String(), nil
}

func marshalResult(fr fetchResult) tool.InvokeResult {
	data, _ := json.Marshal(fr)
	return tool.InvokeResult{Content: string(data)}
}
