// Package openrouter implements provider.Provider for OpenRouter's
// OpenAI-compatible chat completions API.
package openrouter

import (
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	oaiprov "github.com/artpar/pragma/internal/provider/openai"
)

const (
	DefaultBaseURL = "https://openrouter.ai/api/v1"
	DefaultModel   = "z-ai/glm-5.3-flash"
)

// Provider reuses the OpenAI-compatible wire adapter while reporting the
// correct provider identity to sessions and runtime diagnostics.
type Provider struct {
	*oaiprov.Provider
}

func New(apiKey string, bus *observe.EventBus, baseURL string) (*Provider, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	inner, err := oaiprov.New(apiKey, bus, oaiprov.WithBaseURL(baseURL))
	if err != nil {
		return nil, err
	}
	return &Provider{Provider: inner}, nil
}

func (p *Provider) Name() string { return "openrouter" }

func (p *Provider) SupportsFeature(feature provider.Feature) bool {
	if feature == provider.FeatureThinking {
		return true
	}
	return p.Provider.SupportsFeature(feature)
}

func (p *Provider) Pricing(modelID string) (model.Pricing, bool) {
	// OpenRouter pricing is model- and route-dependent. Avoid attributing the
	// embedded OpenAI adapter's prices to OpenRouter models.
	return model.Pricing{}, false
}

func (p *Provider) ContextWindow(modelID string) (int, bool) {
	if modelID == DefaultModel || modelID == "stealth/ox-alpha" {
		return 1_048_576, true
	}
	return p.Provider.ContextWindow(modelID)
}

func (p *Provider) ListModels() []string { return []string{DefaultModel} }
