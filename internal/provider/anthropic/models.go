package anthropic

import (
	"sort"
	"strings"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
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

// LookupModel returns the ModelInfo for a model ID, resolving aliases.
// Returns false if the model is not known.
func LookupModel(modelID string) (ModelInfo, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	if info, ok := registry[modelID]; ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: info, true")
		observe.GlobalTrace("return: info, true")
		observe.GlobalTrace("return: info, true")
		return info, true
	}

	if resolved, ok := aliases[modelID]; ok {
		observe.GlobalTrace("if: ok")
		if info, ok := registry[resolved]; ok {
			observe.GlobalTrace("if: ok")
			observe.GlobalTrace("return: info, true")
			observe.GlobalTrace("return: info, true")
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
		observe.GlobalTrace("return: registry[candidates[0]], true")
		observe.GlobalTrace("return: registry[candidates[0]], true")
		return registry[candidates[0]], true
	}
	observe.GlobalTrace("return: ModelInfo{}, false")
	observe.GlobalTrace("return: ModelInfo{}, false")
	observe.GlobalTrace("return: ModelInfo{}, false")
	return ModelInfo{}, false
}
