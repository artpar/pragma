// Package openrouter implements provider.Provider for OpenRouter's
// OpenAI-compatible chat completions API.
package openrouter

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	oaiprov "github.com/artpar/pragma/internal/provider/openai"
	"github.com/artpar/pragma/internal/provider/rawcapture"
	"github.com/artpar/pragma/internal/provider/shared"
	oaisdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const (
	DefaultBaseURL = "https://openrouter.ai/api/v1"
	DefaultModel   = "z-ai/glm-5.3"
	FlashModel     = "z-ai/glm-5.3-flash"
)

// Provider reuses the OpenAI-compatible wire adapter while reporting the
// correct provider identity to sessions and runtime diagnostics.
type Provider struct {
	*oaiprov.Provider
	wireClient oaisdk.Client
	bus        *observe.EventBus
}

func New(apiKey string, bus *observe.EventBus, baseURL string) (*Provider, error) {
	// Match the embedded OpenAI-compatible adapter's existing key fallback so
	// streaming and nonstreaming requests cannot use different credentials.
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	inner, err := oaiprov.New(
		apiKey,
		bus,
		oaiprov.WithBaseURL(baseURL),
		oaiprov.WithErrorClassifier(openrouterClassify),
	)
	if err != nil {
		return nil, err
	}
	client, ok := rawcapture.HTTPClientFromEnv(10 * time.Minute)
	if !ok {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	wireClient := oaisdk.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(baseURL), option.WithHTTPClient(client))
	return &Provider{Provider: inner, wireClient: wireClient, bus: bus}, nil
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
	if modelID == DefaultModel || modelID == FlashModel || modelID == "stealth/ox-alpha" {
		return 1_310_720, true
	}
	return p.Provider.ContextWindow(modelID)
}

func (p *Provider) ListModels() []string { return []string{DefaultModel, FlashModel} }

var openrouterFallbackClassify = shared.ClassifyByStatusCodes([]string{"429", "500", "502", "503", "504"})

func openrouterClassify(err error) shared.ErrorClassification {
	msg := err.Error()
	if strings.Contains(msg, `"reason":"in_flight_budget_exhausted"`) {
		return shared.ErrorClassification{
			Wrapped:    err,
			Retryable:  true,
			ErrorType:  "rate_limit",
			RetryAfter: openrouterRetryAfter(msg),
		}
	}
	return openrouterFallbackClassify(err)
}

func openrouterRetryAfter(msg string) time.Duration {
	const marker = `"Retry-After":"`
	start := strings.Index(msg, marker)
	if start < 0 {
		return 0
	}
	start += len(marker)
	end := strings.IndexByte(msg[start:], '"')
	if end < 0 {
		return 0
	}
	seconds, err := strconv.Atoi(msg[start : start+end])
	if err != nil || seconds < 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
