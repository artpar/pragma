// Package openrouter implements provider.Provider for OpenRouter's
// OpenAI-compatible chat completions API.
package openrouter

import (
	"errors"
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	if apiKey == "" {
		observe.GlobalTrace("if: apiKey == \"\"")
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if baseURL == "" {
		observe.GlobalTrace("if: baseURL == \"\"")
		baseURL = DefaultBaseURL
	}
	inner, err := oaiprov.New(
		apiKey,
		bus,
		oaiprov.WithBaseURL(baseURL),
		oaiprov.WithErrorClassifier(openrouterClassify),
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
	wireClient := oaisdk.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(baseURL), option.WithHTTPClient(client))
	observe.GlobalTrace("return: &Provider{Provider: inner, wireClient: wireClient, bus: bus}, nil")
	return &Provider{Provider: inner, wireClient: wireClient, bus: bus}, nil
}

func (p *Provider) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"openrouter\"")
	return "openrouter"
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
	observe.GlobalTrace("return: model.Pricing{}, false")

	return model.Pricing{}, false
}

func (p *Provider) ContextWindow(modelID string) (int, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if modelID == DefaultModel || modelID == FlashModel || modelID == "stealth/ox-alpha" {
		observe.GlobalTrace("if: modelID == DefaultModel || modelID == FlashModel || modelID == \"stealth/ox-al...")
		observe.GlobalTrace("return: 1_310_720, true")
		return 1_310_720, true
	}
	observe.GlobalTrace("return: p.Provider.ContextWindow(modelID)")
	return p.Provider.ContextWindow(modelID)
}

func (p *Provider) ListModels() []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: []string{DefaultModel, FlashModel}")
	return []string{DefaultModel, FlashModel}
}

var openrouterFallbackClassify = shared.ClassifyByStatusCodes([]string{"429", "500", "502", "503", "504"})

func openrouterClassify(err error) shared.ErrorClassification {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	msg := err.Error()
	if strings.Contains(msg, `"reason":"in_flight_budget_exhausted"`) {
		observe.GlobalTrace("if: strings.Contains(msg, `\"reason\":\"in_flight_budget_exhausted\"`)")
		observe.GlobalTrace("return: shared.ErrorClassification{\n\tWrapped:\terr,\n\tRetryable:\ttrue,\n\tErrorType:\t\"rat...")
		return shared.ErrorClassification{
			Wrapped:    err,
			Retryable:  true,
			ErrorType:  "rate_limit",
			RetryAfter: openrouterRetryAfter(msg),
		}
	}
	// Structured status classification: the openai-go SDK error renders as
	// `POST "<request URL>": <status> <statusText> <body>`, so substring
	// matching sees URL ports and body text. Ephemeral ports or error bodies
	// containing code-like digits ("127.0.0.1:25003" contains "500") must
	// not turn a permanent 4xx into a ten-attempt retry storm.
	var apiErr *oaisdk.Error
	if errors.As(err, &apiErr) {
		observe.GlobalTrace("if: errors.As(err, &apiErr)")
		switch status := apiErr.StatusCode; status {
		case 429:
			observe.GlobalTrace("case 429")
			observe.GlobalTrace("return: shared.ErrorClassification{\n\tWrapped:\terr,\n\tRetryable:\ttrue,\n\tErrorType:\t\"rat...")
			return shared.ErrorClassification{
				Wrapped:    err,
				Retryable:  true,
				ErrorType:  "rate_limit",
				RetryAfter: openrouterRetryAfter(msg),
			}
		case 500, 502, 503, 504:
			observe.GlobalTrace("case 500, 502, 503, 504")
			observe.GlobalTrace("return: shared.ErrorClassification{\n\tWrapped:\terr,\n\tRetryable:\ttrue,\n\tErrorType:\t\"ser...")
			return shared.ErrorClassification{
				Wrapped:   err,
				Retryable: true,
				ErrorType: "server_error",
			}
		default:
			observe.GlobalTrace("default")
			observe.GlobalTrace("return: shared.ErrorClassification{\n\tWrapped:\terr,\n\tRetryable:\tfalse,\n\tErrorType:\t\"req...")
			return shared.ErrorClassification{
				Wrapped:   err,
				Retryable: false,
				ErrorType: "request_failed",
			}
		}
	}
	observe.GlobalTrace("return: openrouterFallbackClassify(err)")
	return openrouterFallbackClassify(err)
}

func openrouterRetryAfter(msg string) time.Duration {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	const marker = `"Retry-After":"`
	start := strings.Index(msg, marker)
	if start < 0 {
		observe.GlobalTrace("if: start < 0")
		observe.GlobalTrace("return: 0")
		return 0
	}
	start += len(marker)
	end := strings.IndexByte(msg[start:], '"')
	if end < 0 {
		observe.GlobalTrace("if: end < 0")
		observe.GlobalTrace("return: 0")
		return 0
	}
	seconds, err := strconv.Atoi(msg[start : start+end])
	if err != nil || seconds < 0 {
		observe.GlobalTrace("if: err != nil || seconds < 0")
		observe.GlobalTrace("return: 0")
		return 0
	}
	observe.GlobalTrace("return: time.Duration(seconds) * time.Second")
	return time.Duration(seconds) * time.Second
}
