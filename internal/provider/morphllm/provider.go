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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if apiKey == "" {
		observe.GlobalTrace("if: apiKey == \"\"")
		apiKey = os.Getenv("MORPH_API_KEY")
	}
	if baseURL == "" {
		observe.GlobalTrace("if: baseURL == \"\"")
		baseURL = DefaultBaseURL
	}
	inner, err := oaiprov.New(apiKey, bus,
		oaiprov.WithBaseURL(baseURL),
		oaiprov.WithChatCompletionRequestTransform(morphRequestTransform),
	)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}
	client, ok := rawcapture.HTTPClientFromEnv(10 * time.Minute)
	if !ok {
		observe.GlobalTrace("if: !ok")
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	wireClient := oaisdk.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
		option.WithHTTPClient(client),
	)
	observe.GlobalTrace("return: &Provider{Provider: inner, wireClient: wireClient, bus: bus}, nil")
	return &Provider{Provider: inner, wireClient: wireClient, bus: bus}, nil
}

func (p *Provider) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"morphllm\"")
	return "morphllm"
}

func (p *Provider) SupportsFeature(feature provider.Feature) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if feature == provider.FeatureThinking {
		observe.GlobalTrace("if: feature == provider.FeatureThinking")
		observe.GlobalTrace("return: true")
		return true
	}
	observe.GlobalTrace("return: p.Provider.SupportsFeature(feature)")
	return p.Provider.SupportsFeature(feature)
}

func (p *Provider) Pricing(modelID string) (model.Pricing, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if modelID != DefaultModel {
		observe.GlobalTrace("if: modelID != DefaultModel")
		observe.GlobalTrace("return: model.Pricing{}, false")
		return model.Pricing{}, false
	}
	observe.GlobalTrace("return: model.Pricing{InputPerMToken: 1.25, OutputPerMToken: 4.40, CacheReadPerMToken...")
	return model.Pricing{InputPerMToken: 1.25, OutputPerMToken: 4.40, CacheReadPerMToken: 0.26}, true
}

func (p *Provider) ContextWindow(modelID string) (int, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if modelID == DefaultModel {
		observe.GlobalTrace("if: modelID == DefaultModel")
		observe.GlobalTrace("return: ContextWindow, true")
		return ContextWindow, true
	}
	observe.GlobalTrace("return: p.Provider.ContextWindow(modelID)")
	return p.Provider.ContextWindow(modelID)
}

func (p *Provider) ListModels() []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: []string{DefaultModel}")
	return []string{DefaultModel}
}

var morphClassify = shared.ClassifyByStatusCodes([]string{"429", "500", "502", "503", "504"})

// Morph documents max_tokens on its OpenAI-compatible endpoint. The shared
// OpenAI adapter otherwise emits OpenAI's newer max_completion_tokens field.
func morphRequestTransform(req *oaisdk.ChatCompletionNewParams) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if req.MaxCompletionTokens.Valid() {
		observe.GlobalTrace("if: req.MaxCompletionTokens.Valid()")
		req.MaxTokens = oaisdk.Int(req.MaxCompletionTokens.Value)
	}
	req.MaxCompletionTokens = param.Opt[int64]{}
}
