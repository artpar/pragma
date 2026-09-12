package openai

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	oaisdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// ModelInfo holds capabilities and pricing for a known OpenAI model.
type ModelInfo struct {
	ID                string
	MaxContext        int
	MaxOutput         int
	Pricing           model.Pricing
	SupportsVision    bool
	SupportsToolUse   bool
	SupportsReasoning bool
	ParallelTools     bool
	ReasoningEfforts  []string // valid reasoning_effort values for this model
}

var registry = map[string]ModelInfo{
	"gpt-4o": {
		ID:              "gpt-4o",
		MaxContext:      128000,
		MaxOutput:       16384,
		Pricing:         model.Pricing{InputPerMToken: 2.50, OutputPerMToken: 10.00, CacheReadPerMToken: 1.25},
		SupportsVision:  true,
		SupportsToolUse: true,
		ParallelTools:   true,
	},
	"gpt-4o-mini": {
		ID:              "gpt-4o-mini",
		MaxContext:      128000,
		MaxOutput:       16384,
		Pricing:         model.Pricing{InputPerMToken: 0.15, OutputPerMToken: 0.60, CacheReadPerMToken: 0.075},
		SupportsVision:  true,
		SupportsToolUse: true,
		ParallelTools:   true,
	},
	"gpt-4-turbo": {
		ID:              "gpt-4-turbo",
		MaxContext:      128000,
		MaxOutput:       4096,
		Pricing:         model.Pricing{InputPerMToken: 10.00, OutputPerMToken: 30.00, CacheReadPerMToken: 5.00},
		SupportsVision:  true,
		SupportsToolUse: true,
		ParallelTools:   true,
	},
}

// ListModels returns the sorted IDs of all known OpenAI models.
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

// FetchModels lists model IDs from an OpenAI-compatible models endpoint
// (GET {baseURL}/models). This one function serves the OpenAI, Groq,
// Morph, and OpenRouter adapters because they all implement that contract.
// No filtering is applied here; the caller decides what belongs in a
// picker. baseURL is required.
func FetchModels(ctx context.Context, apiKey, baseURL string) ([]string, error) {
	observe.TraceCtx(ctx, "openai", "FetchModels", "enter")
	defer observe.TraceCtx(ctx, "openai", "FetchModels", "exit")
	if baseURL == "" {
		observe.TraceCtx(ctx, "openai", "FetchModels", "if: baseURL == \"\"")
		observe.TraceCtx(ctx, "openai", "FetchModels", "return: nil, fmt.Errorf(\"models fetch: base URL is required\")")
		return nil, fmt.Errorf("models fetch: base URL is required")
	}
	client := oaisdk.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
	)
	pager := client.Models.ListAutoPaging(ctx)
	var ids []string
	for pager.Next() {
		observe.TraceCtx(ctx, "openai", "FetchModels", "for: pager.Next()")
		if id := strings.TrimSpace(pager.Current().ID); id != "" {
			observe.TraceCtx(ctx, "openai", "FetchModels", "if: id != \"\"")
			ids = append(ids, id)
		}
	}
	if err := pager.Err(); err != nil {
		observe.TraceCtx(ctx, "openai", "FetchModels", "if: err != nil")
		observe.TraceCtx(ctx, "openai", "FetchModels", "return: nil, err")
		return nil, err
	}
	sort.Strings(ids)
	observe.TraceCtx(ctx, "openai", "FetchModels", "return: ids, nil")
	return ids, nil
}

// LookupModel finds a model by exact ID or prefix match.
// Prefix match is sorted alphabetically for determinism.
func LookupModel(modelID string) (ModelInfo, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if info, ok := registry[modelID]; ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: info, true")
		return info, true
	}
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
