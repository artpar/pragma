package groq

import (
	"sort"
	"strings"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// ModelInfo holds capabilities and pricing for a known Groq model.
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
	"llama-3.3-70b-versatile": {
		ID:              "llama-3.3-70b-versatile",
		MaxContext:      131072,
		MaxOutput:       32768,
		Pricing:         model.Pricing{InputPerMToken: 0.59, OutputPerMToken: 0.79},
		SupportsToolUse: true,
		ParallelTools:   true,
	},
	"llama-3.1-8b-instant": {
		ID:              "llama-3.1-8b-instant",
		MaxContext:      131072,
		MaxOutput:       131072,
		Pricing:         model.Pricing{InputPerMToken: 0.05, OutputPerMToken: 0.08},
		SupportsToolUse: true,
		ParallelTools:   true,
	},
	"meta-llama/llama-4-scout-17b-16e-instruct": {
		ID:              "meta-llama/llama-4-scout-17b-16e-instruct",
		MaxContext:      131072,
		MaxOutput:       8192,
		Pricing:         model.Pricing{InputPerMToken: 0.11, OutputPerMToken: 0.34},
		SupportsVision:  true,
		SupportsToolUse: true,
		ParallelTools:   true,
	},
	"openai/gpt-oss-20b": {
		ID:                "openai/gpt-oss-20b",
		MaxContext:        131072,
		MaxOutput:         65536,
		Pricing:           model.Pricing{InputPerMToken: 0.075, OutputPerMToken: 0.30, CacheReadPerMToken: 0.0375},
		SupportsToolUse:   true,
		SupportsReasoning: true,
		ReasoningEfforts:  []string{"low", "medium", "high"},
	},
	"openai/gpt-oss-120b": {
		ID:                "openai/gpt-oss-120b",
		MaxContext:        131072,
		MaxOutput:         65536,
		Pricing:           model.Pricing{InputPerMToken: 0.15, OutputPerMToken: 0.60, CacheReadPerMToken: 0.075},
		SupportsToolUse:   true,
		SupportsReasoning: true,
		ReasoningEfforts:  []string{"low", "medium", "high"},
	},
	"qwen/qwen3-32b": {
		ID:                "qwen/qwen3-32b",
		MaxContext:        131072,
		MaxOutput:         40960,
		Pricing:           model.Pricing{InputPerMToken: 0.29, OutputPerMToken: 0.59},
		SupportsToolUse:   true,
		SupportsReasoning: true,
		ParallelTools:     true,
		ReasoningEfforts:  []string{"none", "default"},
	},
}

// ListModels returns the sorted IDs of all known Groq models.
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
