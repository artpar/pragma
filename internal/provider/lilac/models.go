package lilac

import (
	"sort"
	"strings"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// ModelInfo holds capabilities and pricing for a known Lilac-hosted model.
type ModelInfo struct {
	ID                string
	MaxContext        int
	MaxOutput         int
	Pricing           model.Pricing
	SupportsVision    bool
	SupportsToolUse   bool
	SupportsReasoning bool
}

// Sources:
//
//	GLM-5.1:  HuggingFace zai-org/GLM-5.1 config.json (max_position_embeddings: 202752)
//	          OpenRouter: max output 131,072
//	Kimi K2.5: HuggingFace moonshotai/Kimi-K2.5 config.json (max_position_embeddings: 262144)
//	           OpenRouter: max output 65,535
//	Kimi K2.6: Lilac docs (2026-05-08) — 262K context, image input, tools, reasoning
//	MiniMax M2.7: Lilac model details (2026-05-19) — 204.8K context, text input/output, tools, reasoning
//	              Max output selected as 131,072 from provider-reported usage notes.
//	Gemma 4:   HuggingFace google/gemma-4-31b-it config.json (text_config.max_position_embeddings: 262144)
//	           Google AI docs: 256K context. No official max output stated.
//	Pricing:   https://docs.getlilac.com/inference/models (2026-05-08 for K2.6)
var registry = map[string]ModelInfo{
	"zai-org/glm-5.1": {
		ID:                "zai-org/glm-5.1",
		MaxContext:        202752,
		MaxOutput:         131072,
		Pricing:           model.Pricing{InputPerMToken: 0.90, OutputPerMToken: 3.00},
		SupportsToolUse:   true,
		SupportsReasoning: true,
	},
	"moonshotai/kimi-k2.5": {
		ID:                "moonshotai/kimi-k2.5",
		MaxContext:        262144,
		MaxOutput:         65535,
		Pricing:           model.Pricing{InputPerMToken: 0.40, OutputPerMToken: 2.00},
		SupportsVision:    true,
		SupportsToolUse:   true,
		SupportsReasoning: true,
	},
	"moonshotai/kimi-k2.6": {
		ID:                "moonshotai/kimi-k2.6",
		MaxContext:        262144,
		MaxOutput:         65535,
		Pricing:           model.Pricing{InputPerMToken: 0.70, OutputPerMToken: 3.50, CacheReadPerMToken: 0.20},
		SupportsVision:    true,
		SupportsToolUse:   true,
		SupportsReasoning: true,
	},
	"minimaxai/minimax-m2.7": {
		ID:                "minimaxai/minimax-m2.7",
		MaxContext:        204800,
		MaxOutput:         131072,
		Pricing:           model.Pricing{InputPerMToken: 0.30, OutputPerMToken: 1.20, CacheReadPerMToken: 0.055},
		SupportsToolUse:   true,
		SupportsReasoning: true,
	},
	"google/gemma-4-31b-it": {
		ID:                "google/gemma-4-31b-it",
		MaxContext:        262144,
		MaxOutput:         16384,
		Pricing:           model.Pricing{InputPerMToken: 0.11, OutputPerMToken: 0.35},
		SupportsVision:    true,
		SupportsToolUse:   true,
		SupportsReasoning: true,
	},
}

// ListModels returns the sorted IDs of all known Lilac models.
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
