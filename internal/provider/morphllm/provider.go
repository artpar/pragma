// Package morphllm implements provider.Provider for Morph's OpenAI-compatible
// chat completions API.
package morphllm

import (
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	oaiprov "github.com/artpar/pragma/internal/provider/openai"
	"github.com/artpar/pragma/internal/provider/rawcapture"
	"github.com/artpar/pragma/internal/provider/shared"
	llmerrors "github.com/mozilla-ai/any-llm-go/errors"
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

var morphFallbackClassify = shared.ClassifyByStatusCodes([]string{"429", "500", "502", "503", "504"})

func morphClassify(err error) shared.ErrorClassification {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	// Structured classification (RTY-002): the substring fallback matches
	// code-like digit substrings anywhere in the error message, and the
	// openai-go SDK error embeds the request URL and response body. A
	// validation 400 whose URL or body carries such digits ("127.0.0.1:25003"
	// contains "500") must not burn the ten-attempt retry budget.
	var rate *llmerrors.RateLimitError
	if errors.As(err, &rate) {
		observe.GlobalTrace("if: errors.As(err, &rate)")
		observe.GlobalTrace("return: shared.ErrorClassification{\n\tWrapped:\terr,\n\tRetryable:\ttrue,\n\tErrorType:\t\"rat...")
		return shared.ErrorClassification{
			Wrapped:    err,
			Retryable:  true,
			ErrorType:  "rate_limit",
			RetryAfter: time.Duration(rate.RetryAfter) * time.Second,
		}
	}
	var apiErr *oaisdk.Error
	if errors.As(err, &apiErr) {
		observe.GlobalTrace("if: errors.As(err, &apiErr)")
		switch status := apiErr.StatusCode; status {
		case 429:
			observe.GlobalTrace("case 429")
			observe.GlobalTrace("return: shared.ErrorClassification{\n\tWrapped:\terr,\n\tRetryable:\ttrue,\n\tErrorType:\t\"rat...")
			return shared.ErrorClassification{
				Wrapped:   err,
				Retryable: true,
				ErrorType: "rate_limit",
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
	observe.GlobalTrace("return: morphFallbackClassify(err)")
	return morphFallbackClassify(err)
}

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
