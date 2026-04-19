package webfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/tool"
)

const (
	fetchTimeout     = 60 * time.Second
	maxBodyBytes     = 10 * 1024 * 1024 // 10 MB
	maxMarkdownChars = 100_000          // truncate markdown to this
	cacheMaxSize     = 50 * 1024 * 1024 // 50 MB
	cacheTTL         = 15 * time.Minute
)

type WebFetchInput struct {
	URL    string `json:"url" desc:"URL to fetch"`
	Prompt string `json:"prompt" desc:"What to extract from the page"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
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
	Provider       provider.Provider
	Bus            *observe.EventBus
	SecondaryModel string // model for summarization; provider-specific
	cache          *urlCache
	cacheOnce      sync.Once
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"WebFetch\"")
	return "WebFetch"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Fetches content from a specified URL and processes it using an AI model...\"")
	observe.GlobalTrace("return: webFetchDescription")
	return webFetchDescription
}

const webFetchDescription = `Fetches content from a specified URL and processes it using an AI model.
- Takes a URL and a prompt as input
- Fetches the URL content, converts HTML to markdown
- Processes the content with the prompt using a small, fast model
- Returns the model's response about the content
- Use this tool when you need to retrieve and analyze web content

Usage notes:
  - The URL must be a fully-formed valid URL
  - HTTP URLs will be automatically upgraded to HTTPS
  - The prompt should describe what information you want to extract from the page
  - This tool is read-only and does not modify any files
  - Results may be summarized if the content is very large
  - Includes a self-cleaning 15-minute cache for faster responses when repeatedly accessing same URL
  - When a URL redirects to a different host, will inform you with redirect URL
  - For GitHub URLs, prefer using the gh CLI via Bash instead (gh pr view, gh issue view, gh api)`

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
	observe.TraceCtx(ctx, "webfetch", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "webfetch", "Tool.CheckPerm", "exit")
	var in struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "webfetch", "Tool.CheckPerm", "if: err != nil")
		observe.TraceCtx(ctx, "webfetch", "Tool.CheckPerm", "return: checker.Check(ctx, \"WebFetch\", \"\")")
		return checker.Check(ctx, "WebFetch", "")
	}
	u, err := url.Parse(in.URL)
	if err != nil || u.Hostname() == "" {
		observe.TraceCtx(ctx, "webfetch", "Tool.CheckPerm", "if: err != nil || u.Hostname() == \"\"")
		observe.TraceCtx(ctx, "webfetch", "Tool.CheckPerm", "return: checker.Check(ctx, \"WebFetch\", \"\")")
		return checker.Check(ctx, "WebFetch", "")
	}
	observe.TraceCtx(ctx, "webfetch", "Tool.CheckPerm", "return: checker.Check(ctx, \"WebFetch\", \"domain:\"+u.Hostname())")
	return checker.Check(ctx, "WebFetch", "domain:"+u.Hostname())
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "exit")
	var in WebFetchInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.URL == "" {
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "if: in.URL == \"\"")
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"url is required\")")
		return tool.InvokeResult{}, fmt.Errorf("url is required")
	}
	if in.Prompt == "" {
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "if: in.Prompt == \"\"")
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"prompt is required\")")
		return tool.InvokeResult{}, fmt.Errorf("prompt is required")
	}

	u, err := url.Parse(in.URL)
	if err != nil {
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Failed to parse URL: %v\", err)}, nil")
		return tool.InvokeResult{Content: fmt.Sprintf("Failed to parse URL: %v", err)}, nil
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "if: u.Scheme != \"http\" && u.Scheme != \"https\"")
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "return: tool.InvokeResult{Content: \"URL must use http or https scheme\"}, nil")
		return tool.InvokeResult{Content: "URL must use http or https scheme"}, nil
	}
	if u.User != nil {
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "if: u.User != nil")
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "return: tool.InvokeResult{Content: \"URL must not contain credentials\"}, nil")
		return tool.InvokeResult{Content: "URL must not contain credentials"}, nil
	}

	if u.Scheme == "http" {
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "if: u.Scheme == \"http\"")
		u.Scheme = "https"
	}
	fetchURL := u.String()

	pinnedAddr, err := resolveAndCheckSSRF(u.Hostname())
	if err != nil {
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "return: tool.InvokeResult{Content: err.Error()}, nil")
		return tool.InvokeResult{Content: err.Error()}, nil
	}

	t.cacheOnce.Do(func() {
		t.cache = newURLCache(cacheMaxSize, cacheTTL)
	})

	start := time.Now()

	if cached, ok := t.cache.Get(fetchURL); ok {
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "if: ok")
		result, err := t.summarize(ctx, cached, in.Prompt)
		if err != nil {
			observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "if: err != nil")
			observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Failed to process content: %v\", err)}...")
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
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "return: marshalResult(fr), nil")
		return marshalResult(fr), nil
	}

	httpCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(httpCtx, http.MethodGet, fetchURL, nil)
	if err != nil {
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Failed to create request: %v\", err)},...")
		return tool.InvokeResult{Content: fmt.Sprintf("Failed to create request: %v", err)}, nil
	}
	req.Header.Set("User-Agent", "gogent/1.0 (AI coding assistant)")

	client := pinnedHTTPClient(u.Hostname(), pinnedAddr)
	resp, err := client.Do(req)
	if err != nil {
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Failed to fetch URL: %v\", err)}, nil")
		return tool.InvokeResult{Content: fmt.Sprintf("Failed to fetch URL: %v", err)}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Failed to read response: %v\", err)}, nil")
		return tool.InvokeResult{Content: fmt.Sprintf("Failed to read response: %v", err)}, nil
	}

	contentType := resp.Header.Get("Content-Type")
	var markdown string

	switch {
	case strings.Contains(contentType, "text/html"):
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "case: strings.Contains(contentType, \"text/html\")")
		markdown, err = htmltomarkdown.ConvertString(string(body))
		if err != nil {

			markdown = string(body)
		}
	case strings.Contains(contentType, "text/"):
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "case: strings.Contains(contentType, \"text/\")")
		markdown = string(body)
	default:
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "default")
		markdown = fmt.Sprintf("[Binary content: %s, %d bytes]", contentType, len(body))
	}

	// Truncate (deduct marker length so result stays within limit)
	const truncMarker = "\n\n[Content truncated]"
	if len(markdown) > maxMarkdownChars {
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "if: len(markdown) > maxMarkdownChars")
		markdown = markdown[:maxMarkdownChars-len(truncMarker)] + truncMarker
	}

	t.cache.Set(fetchURL, markdown)

	result, err := t.summarize(ctx, markdown, in.Prompt)
	if err != nil {
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Failed to process content: %v\", err)}...")
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
	observe.TraceCtx(ctx, "webfetch", "Tool.Invoke", "return: marshalResult(fr), nil")
	return marshalResult(fr), nil
}

// summarize calls the provider with a secondary prompt to process fetched content.
func (t *Tool) summarize(ctx context.Context, content, userPrompt string) (string, error) {
	observe.TraceCtx(ctx, "webfetch", "Tool.summarize", "enter")
	defer observe.TraceCtx(ctx, "webfetch", "Tool.summarize", "exit")
	if t.Provider == nil {
		observe.TraceCtx(ctx, "webfetch", "Tool.summarize", "if: t.Provider == nil")

		if len(content) > 2000 {
			observe.TraceCtx(ctx, "webfetch", "Tool.summarize", "if: len(content) > 2000")
			observe.TraceCtx(ctx, "webfetch", "Tool.summarize", "return: content[:2000] + \"\\n[truncated]\", nil")
			return content[:2000] + "\n[truncated]", nil
		}
		observe.TraceCtx(ctx, "webfetch", "Tool.summarize", "return: content, nil")
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
		Model:     t.SecondaryModel,
		MaxTokens: 4096,
		Messages:  messages,
		System: model.SystemPrompt{
			Blocks: []model.SystemBlock{{
				Text: "You are a helpful assistant that answers questions based on provided web content. Be concise and accurate.",
			}},
		},
	})
	if err != nil {
		observe.TraceCtx(ctx, "webfetch", "Tool.summarize", "if: err != nil")
		observe.TraceCtx(ctx, "webfetch", "Tool.summarize", "return: \"\", fmt.Errorf(\"secondary model call: %w\", err)")
		return "", fmt.Errorf("secondary model call: %w", err)
	}

	// Extract text from response
	var result strings.Builder
	for _, part := range resp.Content {
		observe.TraceCtx(ctx, "webfetch", "Tool.summarize", "range resp.Content")
		if tp, ok := part.(model.TextPart); ok {
			observe.TraceCtx(ctx, "webfetch", "Tool.summarize", "if: ok")
			result.WriteString(tp.Text)
		}
	}
	observe.TraceCtx(ctx, "webfetch", "Tool.summarize", "return: result.String(), nil")
	return result.String(), nil
}

func marshalResult(fr fetchResult) tool.InvokeResult {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := json.Marshal(fr)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{Content: fmt.Sprintf(\"Fetch completed but failed to marshal...")
		return tool.InvokeResult{Content: fmt.Sprintf("Fetch completed but failed to marshal result: %v", err)}
	}
	observe.GlobalTrace("return: tool.InvokeResult{Content: string(data)}")
	return tool.InvokeResult{Content: string(data)}
}

// resolveAndCheckSSRF resolves the hostname, rejects private/loopback/metadata IPs,
// and returns the first valid IP for connection pinning (prevents DNS rebinding).
func resolveAndCheckSSRF(hostname string) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	ips, err := net.LookupHost(hostname)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"DNS resolution failed for %s: %v\", hostname, err)")
		return "", fmt.Errorf("DNS resolution failed for %s: %w", hostname, err)
	}
	var firstValid string
	for _, ipStr := range ips {
		observe.GlobalTrace("range ips")
		ip := net.ParseIP(ipStr)
		if ip == nil {
			observe.GlobalTrace("if: ip == nil")
			continue
		}
		if isPrivateIP(ip) {
			observe.GlobalTrace("if: isPrivateIP(ip)")
			observe.GlobalTrace("return: \"\", fmt.Errorf(\"URL resolves to private/reserved IP address (%s) — request ...")
			return "", fmt.Errorf("URL resolves to private/reserved IP address (%s) — request blocked for security", ipStr)
		}
		if firstValid == "" {
			observe.GlobalTrace("if: firstValid == \"\"")
			firstValid = ipStr
		}
	}
	if firstValid == "" {
		observe.GlobalTrace("if: firstValid == \"\"")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"DNS resolution returned no usable addresses for %s\", hostname)")
		return "", fmt.Errorf("DNS resolution returned no usable addresses for %s", hostname)
	}
	observe.GlobalTrace("return: firstValid, nil")
	return firstValid, nil
}

// maxRedirects is the maximum number of HTTP redirects to follow.
const maxRedirects = 5

// pinnedHTTPClient returns an HTTP client whose transport resolves the given
// hostname to pinnedIP, preventing DNS rebinding between SSRF check and fetch.
// Every redirect target is validated against SSRF rules before following.
func pinnedHTTPClient(hostname, pinnedIP string) *http.Client {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	dialer := &net.Dialer{Timeout: 10 * time.Second}

	pinned := map[string]string{hostname: pinnedIP}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return dialer.DialContext(ctx, network, addr)
			}
			if ip, ok := pinned[host]; ok {
				addr = net.JoinHostPort(ip, port)
			}
			return dialer.DialContext(ctx, network, addr)
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		MaxIdleConns:          1,
		IdleConnTimeout:       30 * time.Second,
	}
	observe.GlobalTrace("return: &http.Client{\n\tTransport:\ttransport,\n\tCheckRedirect: func(req *http.Request, ...")

	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			redirectHost := req.URL.Hostname()
			if _, ok := pinned[redirectHost]; ok {
				return nil
			}
			validIP, err := resolveAndCheckSSRF(redirectHost)
			if err != nil {
				return fmt.Errorf("redirect to %s blocked: %w", redirectHost, err)
			}
			pinned[redirectHost] = validIP
			return nil
		},
	}
}

// isPrivateIP checks if an IP is loopback, private, link-local, or a cloud metadata endpoint.
func isPrivateIP(ip net.IP) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		observe.GlobalTrace("if: ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLoca...")
		observe.GlobalTrace("return: true")
		return true
	}

	metadataIP := net.ParseIP("169.254.169.254")
	observe.GlobalTrace("return: ip.Equal(metadataIP)")
	return ip.Equal(metadataIP)
}
