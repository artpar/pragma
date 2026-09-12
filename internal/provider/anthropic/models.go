package anthropic

import (
	"context"
	"sort"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// ModelInfo describes an Anthropic model's capabilities and pricing.
type ModelInfo struct {
	ID               string
	ContextWindow    int // total context window in tokens (default 200_000)
	DefaultMaxOutput int
	UpperMaxOutput   int
	MaxThinking      int
	Pricing          model.Pricing
	SupportsThinking bool
	ThinkingType     string // "adaptive" or "enabled"
}

// aliases maps short model names to their dated equivalents.
var aliases = map[string]string{
	"claude-opus-4-6":   "claude-opus-4-6-20250610",
	"claude-sonnet-4-6": "claude-sonnet-4-6-20250514",
	"claude-opus-4-5":   "claude-opus-4-5-20250220",
	"claude-sonnet-4":   "claude-sonnet-4-20250514",
	"claude-haiku-4-5":  "claude-haiku-4-5-20251001",
}

// registry holds the known model configurations.
var registry = map[string]ModelInfo{
	"claude-opus-4-6-20250610": {
		ID:               "claude-opus-4-6-20250610",
		ContextWindow:    200_000,
		DefaultMaxOutput: 64000,
		UpperMaxOutput:   128000,
		MaxThinking:      127999,
		Pricing: model.Pricing{
			InputPerMToken:       5.0,
			OutputPerMToken:      25.0,
			CacheCreatePerMToken: 6.25,
			CacheReadPerMToken:   0.5,
		},
		SupportsThinking: true,
		ThinkingType:     "adaptive",
	},
	"claude-sonnet-4-6-20250514": {
		ID:               "claude-sonnet-4-6-20250514",
		ContextWindow:    200_000,
		DefaultMaxOutput: 32000,
		UpperMaxOutput:   128000,
		MaxThinking:      127999,
		Pricing: model.Pricing{
			InputPerMToken:       3.0,
			OutputPerMToken:      15.0,
			CacheCreatePerMToken: 3.75,
			CacheReadPerMToken:   0.3,
		},
		SupportsThinking: true,
		ThinkingType:     "adaptive",
	},
	"claude-opus-4-5-20250220": {
		ID:               "claude-opus-4-5-20250220",
		ContextWindow:    200_000,
		DefaultMaxOutput: 32000,
		UpperMaxOutput:   64000,
		MaxThinking:      63999,
		Pricing: model.Pricing{
			InputPerMToken:       5.0,
			OutputPerMToken:      25.0,
			CacheCreatePerMToken: 6.25,
			CacheReadPerMToken:   0.5,
		},
		SupportsThinking: true,
		ThinkingType:     "adaptive",
	},
	"claude-sonnet-4-20250514": {
		ID:               "claude-sonnet-4-20250514",
		ContextWindow:    200_000,
		DefaultMaxOutput: 32000,
		UpperMaxOutput:   64000,
		MaxThinking:      63999,
		Pricing: model.Pricing{
			InputPerMToken:       3.0,
			OutputPerMToken:      15.0,
			CacheCreatePerMToken: 3.75,
			CacheReadPerMToken:   0.3,
		},
		SupportsThinking: true,
		ThinkingType:     "enabled",
	},
	"claude-haiku-4-5-20251001": {
		ID:               "claude-haiku-4-5-20251001",
		ContextWindow:    200_000,
		DefaultMaxOutput: 32000,
		UpperMaxOutput:   64000,
		MaxThinking:      63999,
		Pricing: model.Pricing{
			InputPerMToken:       1.0,
			OutputPerMToken:      5.0,
			CacheCreatePerMToken: 1.25,
			CacheReadPerMToken:   0.1,
		},
		SupportsThinking: true,
		ThinkingType:     "enabled",
	},
}

// ListModels returns the sorted IDs of all known Anthropic models.
func ListModels() []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	ids := make([]string, 0, len(registry))
	for id := range registry {
		observe.GlobalTrace("range registry")
		ids = append(ids, id)
	}
	sort.Strings(ids)
	observe.GlobalTrace("return: ids")
	return ids
}

// FetchModels lists model IDs from the live Anthropic Models API
// (GET /v1/models), paging through all results. Used by the cross-provider
// model catalog behind /models. An empty baseURL selects the SDK default.
func FetchModels(ctx context.Context, apiKey, baseURL string) ([]string, error) {
	observe.TraceCtx(ctx, "anthropic", "FetchModels", "enter")
	defer observe.TraceCtx(ctx, "anthropic", "FetchModels", "exit")
	clientOpts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithMaxRetries(0),
	}
	if baseURL != "" {
		observe.TraceCtx(ctx, "anthropic", "FetchModels", "if: baseURL != \"\"")
		clientOpts = append(clientOpts, option.WithBaseURL(baseURL))
	}
	client := sdk.NewClient(clientOpts...)
	pager := client.Models.ListAutoPaging(ctx, sdk.ModelListParams{})
	var ids []string
	for pager.Next() {
		observe.TraceCtx(ctx, "anthropic", "FetchModels", "for: pager.Next()")
		if id := strings.TrimSpace(pager.Current().ID); id != "" {
			observe.TraceCtx(ctx, "anthropic", "FetchModels", "if: id != \"\"")
			ids = append(ids, id)
		}
	}
	if err := pager.Err(); err != nil {
		observe.TraceCtx(ctx, "anthropic", "FetchModels", "if: err != nil")
		observe.TraceCtx(ctx, "anthropic", "FetchModels", "return: nil, err")
		return nil, err
	}
	sort.Strings(ids)
	observe.TraceCtx(ctx, "anthropic", "FetchModels", "return: ids, nil")
	return ids, nil
}

// LookupModel returns the ModelInfo for a model ID, resolving aliases.
// Returns false if the model is not known.
func LookupModel(modelID string) (ModelInfo, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	if info, ok := registry[modelID]; ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: info, true")
		return info, true
	}

	if resolved, ok := aliases[modelID]; ok {
		observe.GlobalTrace("if: ok")
		if info, ok := registry[resolved]; ok {
			observe.GlobalTrace("if: ok")
			observe.GlobalTrace("return: info, true")
			return info, true
		}
	}
	// Try prefix match (e.g., "claude-sonnet-4-6" matches "claude-sonnet-4-6-20250514").
	// Collect all matches and sort alphabetically for deterministic results.
	var candidates []string
	for id := range registry {
		observe.GlobalTrace("range registry")
		if strings.HasPrefix(id, modelID) {
			observe.GlobalTrace("if: strings.HasPrefix(id, modelID)")
			candidates = append(candidates, id)
		}
	}
	if len(candidates) > 0 {
		observe.GlobalTrace("if: len(candidates) > 0")
		sort.Strings(candidates)
		observe.GlobalTrace("return: registry[candidates[0]], true")
		return registry[candidates[0]], true
	}
	observe.GlobalTrace("return: ModelInfo{}, false")
	return ModelInfo{}, false
}
