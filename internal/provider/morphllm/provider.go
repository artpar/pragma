// Package morphllm implements provider.Provider for Morph's OpenAI-compatible
// chat completions API.
package morphllm

import (
	"net/http"
	"os"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	oaiprov "github.com/artpar/pragma/internal/provider/openai"
	"github.com/artpar/pragma/internal/provider/rawcapture"
	"github.com/artpar/pragma/internal/provider/shared"
	oaisdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
)

const (
	DefaultBaseURL = "https://api.morphllm.com/v1"
	DefaultModel   = "morph-glm53-744b"
	// Morph's public catalog advertises a 1M-token context without a more
	// precise value. Keep the local budget conservative until a live model
	// response provides an exact integer.
	ContextWindow = 1_000_000
)

// Provider reuses Pragma's OpenAI-compatible wire adapter while retaining the
// Morph identity and model metadata in sessions, accounting, and diagnostics.
type Provider struct {
	*oaiprov.Provider
	wireClient oaisdk.Client
	bus        *observe.EventBus
}

func New(apiKey string, bus *observe.EventBus, baseURL string) (*Provider, error) {
	if apiKey == "" {
		apiKey = os.Getenv("MORPH_API_KEY")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	inner, err := oaiprov.New(apiKey, bus,
		oaiprov.WithBaseURL(baseURL),
		oaiprov.WithChatCompletionRequestTransform(morphRequestTransform),
	)
	if err != nil {
		return nil, err
	}
	client, ok := rawcapture.HTTPClientFromEnv(10 * time.Minute)
	if !ok {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	wireClient := oaisdk.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
		option.WithHTTPClient(client),
	)
	return &Provider{Provider: inner, wireClient: wireClient, bus: bus}, nil
}

func (p *Provider) Name() string { return "morphllm" }

func (p *Provider) SupportsFeature(feature provider.Feature) bool {
	if feature == provider.FeatureThinking {
		return true
	}
	return p.Provider.SupportsFeature(feature)
}

func (p *Provider) Pricing(modelID string) (model.Pricing, bool) {
	if modelID != DefaultModel {
		return model.Pricing{}, false
	}
	return model.Pricing{InputPerMToken: 1.25, OutputPerMToken: 4.40, CacheReadPerMToken: 0.26}, true
}

func (p *Provider) ContextWindow(modelID string) (int, bool) {
	if modelID == DefaultModel {
		return ContextWindow, true
	}
	return p.Provider.ContextWindow(modelID)
}

func (p *Provider) ListModels() []string { return []string{DefaultModel} }

var morphClassify = shared.ClassifyByStatusCodes([]string{"429", "500", "502", "503", "504"})

// Morph documents max_tokens on its OpenAI-compatible endpoint. The shared
// OpenAI adapter otherwise emits OpenAI's newer max_completion_tokens field.
func morphRequestTransform(req *oaisdk.ChatCompletionNewParams) {
	if req.MaxCompletionTokens.Valid() {
		req.MaxTokens = oaisdk.Int(req.MaxCompletionTokens.Value)
	}
	req.MaxCompletionTokens = param.Opt[int64]{}
}
