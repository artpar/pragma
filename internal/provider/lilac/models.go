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

// Source: https://docs.getlilac.com/inference/models (2026-04-15)
var registry = map[string]ModelInfo{
	"zai-org/glm-5.1": {
		ID:                "zai-org/glm-5.1",
		MaxContext:        202800,
		MaxOutput:         16384,
		Pricing:           model.Pricing{InputPerMToken: 0.90, OutputPerMToken: 3.00},
		SupportsToolUse:   true,
		SupportsReasoning: true,
	},
	"moonshotai/kimi-k2.5": {
		ID:                "moonshotai/kimi-k2.5",
		MaxContext:        262144,
		MaxOutput:         16384,
		Pricing:           model.Pricing{InputPerMToken: 0.40, OutputPerMToken: 2.00},
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
