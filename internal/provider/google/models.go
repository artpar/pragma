package google

import (
	"context"
	"sort"
	"strings"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"google.golang.org/genai"
)

// ModelInfo holds capabilities and pricing for a known Google model.
type ModelInfo struct {
	ID               string
	MaxContext       int
	MaxOutput        int
	Pricing          model.Pricing
	SupportsVision   bool
	SupportsToolUse  bool
	SupportsThinking bool
}

// Source: https://ai.google.dev/gemini-api/docs/pricing (2026-04-18)
// Source: https://ai.google.dev/gemini-api/docs/models (2026-04-18)
var registry = map[string]ModelInfo{

	"gemini-3.1-pro-preview": {
		ID:               "gemini-3.1-pro-preview",
		MaxContext:       1_048_576,
		MaxOutput:        65_536,
		Pricing:          model.Pricing{InputPerMToken: 2.00, OutputPerMToken: 12.00, CacheReadPerMToken: 0.20},
		SupportsVision:   true,
		SupportsToolUse:  true,
		SupportsThinking: true,
	},
	"gemini-3-flash-preview": {
		ID:               "gemini-3-flash-preview",
		MaxContext:       1_048_576,
		MaxOutput:        65_536,
		Pricing:          model.Pricing{InputPerMToken: 0.50, OutputPerMToken: 3.00, CacheReadPerMToken: 0.05},
		SupportsVision:   true,
		SupportsToolUse:  true,
		SupportsThinking: true,
	},
	"gemini-3.1-flash-lite-preview": {
		ID:               "gemini-3.1-flash-lite-preview",
		MaxContext:       1_048_576,
		MaxOutput:        65_536,
		Pricing:          model.Pricing{InputPerMToken: 0.25, OutputPerMToken: 1.50, CacheReadPerMToken: 0.025},
		SupportsVision:   true,
		SupportsToolUse:  true,
		SupportsThinking: true,
	},

	"gemini-2.5-pro": {
		ID:               "gemini-2.5-pro",
		MaxContext:       1_048_576,
		MaxOutput:        65_536,
		Pricing:          model.Pricing{InputPerMToken: 1.25, OutputPerMToken: 10.00, CacheReadPerMToken: 0.125},
		SupportsVision:   true,
		SupportsToolUse:  true,
		SupportsThinking: true,
	},
	"gemini-2.5-flash": {
		ID:               "gemini-2.5-flash",
		MaxContext:       1_048_576,
		MaxOutput:        65_536,
		Pricing:          model.Pricing{InputPerMToken: 0.30, OutputPerMToken: 2.50, CacheReadPerMToken: 0.03},
		SupportsVision:   true,
		SupportsToolUse:  true,
		SupportsThinking: true,
	},
	"gemini-2.5-flash-lite": {
		ID:               "gemini-2.5-flash-lite",
		MaxContext:       1_048_576,
		MaxOutput:        65_536,
		Pricing:          model.Pricing{InputPerMToken: 0.10, OutputPerMToken: 0.40, CacheReadPerMToken: 0.01},
		SupportsVision:   true,
		SupportsToolUse:  true,
		SupportsThinking: false,
	},
}

// ListModels returns the sorted IDs of all known Google models.
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

// FetchModels lists model IDs from the live Gemini Models API
// (GET /v1beta/models), keeping only models that support generateContent
// (chat) — the endpoint also returns embeddings, image, and video models
// that cannot serve a conversation. Resource names of the form
// "models/<id>" are stripped to the bare IDs the registry and requests use.
// An empty baseURL selects the SDK default.
func FetchModels(ctx context.Context, apiKey, baseURL string) ([]string, error) {
	observe.TraceCtx(ctx, "google", "FetchModels", "enter")
	defer observe.TraceCtx(ctx, "google", "FetchModels", "exit")
	clientConfig := &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	}
	if baseURL != "" {
		observe.TraceCtx(ctx, "google", "FetchModels", "if: baseURL != \"\"")
		clientConfig.HTTPOptions = genai.HTTPOptions{BaseURL: baseURL}
	}
	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		observe.TraceCtx(ctx, "google", "FetchModels", "if: err != nil")
		observe.TraceCtx(ctx, "google", "FetchModels", "return: nil, err")
		return nil, err
	}
	var ids []string
	for m, err := range client.Models.All(ctx) {
		observe.TraceCtx(ctx, "google", "FetchModels", "range client.Models.All(ctx)")
		if err != nil {
			observe.TraceCtx(ctx, "google", "FetchModels", "if: err != nil")
			observe.TraceCtx(ctx, "google", "FetchModels", "return: nil, err")
			return nil, err
		}
		if m == nil || !supportsGenerateContent(m.SupportedActions) {
			observe.TraceCtx(ctx, "google", "FetchModels", "if: m == nil || !supportsGenerateContent(m.SupportedActions)")
			continue
		}
		id := strings.TrimPrefix(m.Name, "models/")
		if id == "" {
			observe.TraceCtx(ctx, "google", "FetchModels", "if: id == \"\"")
			continue
		}
		if !containsString(ids, id) {
			observe.TraceCtx(ctx, "google", "FetchModels", "if: !containsString(ids, id)")
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	observe.TraceCtx(ctx, "google", "FetchModels", "return: ids, nil")
	return ids, nil
}

// supportsGenerateContent reports whether the model's supported actions
// include chat generation. The Gemini REST API reports this as
// supportedGenerationMethods; the SDK maps it to SupportedActions.
func supportsGenerateContent(actions []string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, action := range actions {
		observe.GlobalTrace("range actions")
		if action == "generateContent" {
			observe.GlobalTrace("if: action == \"generateContent\"")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func containsString(haystack []string, needle string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, s := range haystack {
		observe.GlobalTrace("range haystack")
		if s == needle {
			observe.GlobalTrace("if: s == needle")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
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
