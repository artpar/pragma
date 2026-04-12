package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/artpar/gogent/internal/observe"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// braveResponse is the top-level response from the Brave Search API.
type braveResponse struct {
	Web braveWebResults `json:"web"`
}

type braveWebResults struct {
	Results []braveResult `json:"results"`
}

type braveResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// searchBrave calls the Brave Search API and returns results.
// apiKey is the Brave Search API subscription token.
// query is the search query string. count is the max number of results (1-20).
func searchBrave(ctx context.Context, apiKey, query string, count int) ([]braveResult, error) {
	observe.TraceCtx(ctx, "websearch", "searchBrave", "enter")
	defer observe.TraceCtx(ctx, "websearch", "searchBrave", "exit")
	params := url.Values{}
	params.Set("q", query)
	params.Set("count", strconv.Itoa(count))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.search.brave.com/res/v1/web/search?"+params.Encode(), nil)
	if err != nil {
		observe.TraceCtx(ctx, "websearch", "searchBrave", "if: err != nil")
		observe.TraceCtx(ctx, "websearch", "searchBrave", "return: nil, fmt.Errorf(\"build request: %w\", err)")
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		observe.TraceCtx(ctx, "websearch", "searchBrave", "if: err != nil")
		observe.TraceCtx(ctx, "websearch", "searchBrave", "return: nil, fmt.Errorf(\"search request: %w\", err)")
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		observe.TraceCtx(ctx, "websearch", "searchBrave", "if: resp.StatusCode != http.StatusOK")
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		observe.TraceCtx(ctx, "websearch", "searchBrave", "return: nil, fmt.Errorf(\"search API returned %d: %s\", resp.StatusCode, string(body))")
		return nil, fmt.Errorf("search API returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		observe.TraceCtx(ctx, "websearch", "searchBrave", "if: err != nil")
		observe.TraceCtx(ctx, "websearch", "searchBrave", "return: nil, fmt.Errorf(\"read response: %w\", err)")
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result braveResponse
	if err := json.Unmarshal(body, &result); err != nil {
		observe.TraceCtx(ctx, "websearch", "searchBrave", "if: err != nil")
		observe.TraceCtx(ctx, "websearch", "searchBrave", "return: nil, fmt.Errorf(\"decode response: %w\", err)")
		return nil, fmt.Errorf("decode response: %w", err)
	}
	observe.TraceCtx(ctx, "websearch", "searchBrave", "return: result.Web.Results, nil")

	return result.Web.Results, nil
}
