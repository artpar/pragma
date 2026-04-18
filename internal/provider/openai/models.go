package openai

import (
	"sort"
	"strings"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
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
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
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
