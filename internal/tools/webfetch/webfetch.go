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
	Provider       provider.Provider
	Bus            *observe.EventBus
	SecondaryModel string // model for summarization; provider-specific
	cache          *urlCache
	cacheOnce      sync.Once
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

	// SSRF protection: resolve hostname, block private/loopback IPs, and pin
	// the resolved address so the HTTP client cannot be DNS-rebinded to a
	// different (private) IP between our check and the actual connection.
	pinnedAddr, err := resolveAndCheckSSRF(u.Hostname())
	if err != nil {
		return tool.InvokeResult{Content: err.Error()}, nil
	}

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

	// Fetch using a client that pins DNS to the already-validated IP
	httpCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(httpCtx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return tool.InvokeResult{Content: fmt.Sprintf("Failed to create request: %v", err)}, nil
	}
	req.Header.Set("User-Agent", "gogent/1.0 (AI coding assistant)")

	client := pinnedHTTPClient(u.Hostname(), pinnedAddr)
	resp, err := client.Do(req)
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

	// Truncate (deduct marker length so result stays within limit)
	const truncMarker = "\n\n[Content truncated]"
	if len(markdown) > maxMarkdownChars {
		markdown = markdown[:maxMarkdownChars-len(truncMarker)] + truncMarker
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
	data, err := json.Marshal(fr)
	if err != nil {
		return tool.InvokeResult{Content: fmt.Sprintf("Fetch completed but failed to marshal result: %v", err)}
	}
	return tool.InvokeResult{Content: string(data)}
}

// resolveAndCheckSSRF resolves the hostname, rejects private/loopback/metadata IPs,
// and returns the first valid IP for connection pinning (prevents DNS rebinding).
func resolveAndCheckSSRF(hostname string) (string, error) {
	ips, err := net.LookupHost(hostname)
	if err != nil {
		return "", fmt.Errorf("DNS resolution failed for %s: %v", hostname, err)
	}
	var firstValid string
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if isPrivateIP(ip) {
			return "", fmt.Errorf("URL resolves to private/reserved IP address (%s) — request blocked for security", ipStr)
		}
		if firstValid == "" {
			firstValid = ipStr
		}
	}
	if firstValid == "" {
		return "", fmt.Errorf("DNS resolution returned no usable addresses for %s", hostname)
	}
	return firstValid, nil
}

// maxRedirects is the maximum number of HTTP redirects to follow.
const maxRedirects = 5

// pinnedHTTPClient returns an HTTP client whose transport resolves the given
// hostname to pinnedIP, preventing DNS rebinding between SSRF check and fetch.
// Every redirect target is validated against SSRF rules before following.
func pinnedHTTPClient(hostname, pinnedIP string) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}

	// pinned tracks hostname→IP mappings. Starts with the initial hostname.
	// Redirect targets are validated and added on follow.
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
		TLSHandshakeTimeout:  10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		MaxIdleConns:          1,
		IdleConnTimeout:       30 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			redirectHost := req.URL.Hostname()
			if _, ok := pinned[redirectHost]; ok {
				return nil // already validated
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
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	// Cloud metadata endpoint: 169.254.169.254
	metadataIP := net.ParseIP("169.254.169.254")
	return ip.Equal(metadataIP)
}
