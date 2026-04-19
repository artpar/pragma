package google

import (
	"sort"
	"strings"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// ModelInfo holds capabilities and pricing for a known Google model.
type ModelInfo struct {
	ID               string
	MaxContext       int
	MaxOutput        int
	Pricing          model.Pricing
	SupportsVision   bool
	SupportsToolUse  bool
	SupportsThinking bool
}

// Source: https://ai.google.dev/gemini-api/docs/pricing (2026-04-18)
// Source: https://ai.google.dev/gemini-api/docs/models (2026-04-18)
var registry = map[string]ModelInfo{

	"gemini-3.1-pro-preview": {
		ID:               "gemini-3.1-pro-preview",
		MaxContext:       1_048_576,
		MaxOutput:        65_536,
		Pricing:          model.Pricing{InputPerMToken: 2.00, OutputPerMToken: 12.00, CacheReadPerMToken: 0.20},
		SupportsVision:   true,
		SupportsToolUse:  true,
		SupportsThinking: true,
	},
	"gemini-3-flash-preview": {
		ID:               "gemini-3-flash-preview",
		MaxContext:       1_048_576,
		MaxOutput:        65_536,
		Pricing:          model.Pricing{InputPerMToken: 0.50, OutputPerMToken: 3.00, CacheReadPerMToken: 0.05},
		SupportsVision:   true,
		SupportsToolUse:  true,
		SupportsThinking: true,
	},
	"gemini-3.1-flash-lite-preview": {
		ID:               "gemini-3.1-flash-lite-preview",
		MaxContext:       1_048_576,
		MaxOutput:        65_536,
		Pricing:          model.Pricing{InputPerMToken: 0.25, OutputPerMToken: 1.50, CacheReadPerMToken: 0.025},
		SupportsVision:   true,
		SupportsToolUse:  true,
		SupportsThinking: true,
	},

	"gemini-2.5-pro": {
		ID:               "gemini-2.5-pro",
		MaxContext:       1_048_576,
		MaxOutput:        65_536,
		Pricing:          model.Pricing{InputPerMToken: 1.25, OutputPerMToken: 10.00, CacheReadPerMToken: 0.125},
		SupportsVision:   true,
		SupportsToolUse:  true,
		SupportsThinking: true,
	},
	"gemini-2.5-flash": {
		ID:               "gemini-2.5-flash",
		MaxContext:       1_048_576,
		MaxOutput:        65_536,
		Pricing:          model.Pricing{InputPerMToken: 0.30, OutputPerMToken: 2.50, CacheReadPerMToken: 0.03},
		SupportsVision:   true,
		SupportsToolUse:  true,
		SupportsThinking: true,
	},
	"gemini-2.5-flash-lite": {
		ID:               "gemini-2.5-flash-lite",
		MaxContext:       1_048_576,
		MaxOutput:        65_536,
		Pricing:          model.Pricing{InputPerMToken: 0.10, OutputPerMToken: 0.40, CacheReadPerMToken: 0.01},
		SupportsVision:   true,
		SupportsToolUse:  true,
		SupportsThinking: false,
	},
}

// ListModels returns the sorted IDs of all known Google models.
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
