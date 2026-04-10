package anthropic

import (
	"strings"

	"github.com/artpar/gogent/internal/model"
)

// ModelInfo describes an Anthropic model's capabilities and pricing.
type ModelInfo struct {
	ID               string
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
		DefaultMaxOutput: 64000,
		UpperMaxOutput:   128000,
		MaxThinking:      127999,
		Pricing: model.Pricing{
			InputPerMToken:       15.0,
			OutputPerMToken:      75.0,
			CacheCreatePerMToken: 18.75,
			CacheReadPerMToken:   1.5,
		},
		SupportsThinking: true,
		ThinkingType:     "adaptive",
	},
	"claude-sonnet-4-6-20250514": {
		ID:               "claude-sonnet-4-6-20250514",
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
		DefaultMaxOutput: 32000,
		UpperMaxOutput:   64000,
		MaxThinking:      63999,
		Pricing: model.Pricing{
			InputPerMToken:       15.0,
			OutputPerMToken:      75.0,
			CacheCreatePerMToken: 18.75,
			CacheReadPerMToken:   1.5,
		},
		SupportsThinking: true,
		ThinkingType:     "adaptive",
	},
	"claude-sonnet-4-20250514": {
		ID:               "claude-sonnet-4-20250514",
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
		DefaultMaxOutput: 32000,
		UpperMaxOutput:   64000,
		MaxThinking:      63999,
		Pricing: model.Pricing{
			InputPerMToken:       0.8,
			OutputPerMToken:      4.0,
			CacheCreatePerMToken: 1.0,
			CacheReadPerMToken:   0.08,
		},
		SupportsThinking: true,
		ThinkingType:     "enabled",
	},
}

// LookupModel returns the ModelInfo for a model ID, resolving aliases.
// Returns false if the model is not known.
func LookupModel(modelID string) (ModelInfo, bool) {
	// Try direct lookup
	if info, ok := registry[modelID]; ok {
		return info, true
	}
	// Try alias resolution
	if resolved, ok := aliases[modelID]; ok {
		if info, ok := registry[resolved]; ok {
			return info, true
		}
	}
	// Try prefix match (e.g., "claude-sonnet-4-6" matches "claude-sonnet-4-6-20250514")
	for id, info := range registry {
		if strings.HasPrefix(id, modelID) {
			return info, true
		}
	}
	return ModelInfo{}, false
}
