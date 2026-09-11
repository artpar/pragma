package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider/rawcapture"
)

// Response is the top-level response from the Brave Search API.
type Response struct {
	Web WebResults `json:"web"`
}

// WebResults holds the web search result list.
type WebResults struct {
	Results []Result `json:"results"`
}

// Result is one web search result.
type Result struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// searchBrave calls the Brave Search API and returns results.
// apiKey is the subscription token sent as X-Subscription-Token; baseURL
// may be empty for the default endpoint.
func searchBrave(ctx context.Context, apiKey, baseURL, query string, count int) ([]Result, error) {
	observe.TraceCtx(ctx, "websearch", "searchBrave", "enter")
	defer observe.TraceCtx(ctx, "websearch", "searchBrave", "exit")
	if baseURL == "" {
		observe.TraceCtx(ctx, "websearch", "searchBrave", "if: baseURL == \"\"")
		baseURL = DefaultBaseURL
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("count", strconv.Itoa(count))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		baseURL+"/res/v1/web/search?"+params.Encode(), nil)
	if err != nil {
		observe.TraceCtx(ctx, "websearch", "searchBrave", "if: err != nil")
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	client := http.DefaultClient
	if tr, ok := rawCaptureTransport(); ok {
		observe.TraceCtx(ctx, "websearch", "searchBrave", "if: tr, ok := rawCaptureTransport()")
		client = &http.Client{Transport: tr}
	}
	resp, err := client.Do(req)
	if err != nil {
		observe.TraceCtx(ctx, "websearch", "searchBrave", "if: err != nil")
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		observe.TraceCtx(ctx, "websearch", "searchBrave", "if: resp.StatusCode != http.StatusOK")
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("search API returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		observe.TraceCtx(ctx, "websearch", "searchBrave", "if: err != nil")
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result Response
	if err := json.Unmarshal(body, &result); err != nil {
		observe.TraceCtx(ctx, "websearch", "searchBrave", "if: err := json.Unmarshal(...)")
		return nil, fmt.Errorf("decode response: %w", err)
	}
	observe.TraceCtx(ctx, "websearch", "searchBrave", "return: result.Web.Results, nil")
	return result.Web.Results, nil
}

// rawCaptureTransport returns a capturing transport when
// PRAGMA_RAW_HTTP_CAPTURE_DIR is set, so the search wire lands in the same
// evidence directory as the model requests.
func rawCaptureTransport() (http.RoundTripper, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	dir := rawcapture.EnabledDirFromEnv()
	if dir == "" {
		observe.GlobalTrace("if: dir == \"\"")
		observe.GlobalTrace("return: nil, false")
		return nil, false
	}
	observe.GlobalTrace("return: rawcapture.NewTransport(dir, http.DefaultTransport), true")
	return rawcapture.NewTransport(dir, http.DefaultTransport), true
}
