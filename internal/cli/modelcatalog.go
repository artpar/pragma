package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider/anthropic"
	googleprov "github.com/artpar/pragma/internal/provider/google"
	groqprov "github.com/artpar/pragma/internal/provider/groq"
	morphprov "github.com/artpar/pragma/internal/provider/morphllm"
	oaiprov "github.com/artpar/pragma/internal/provider/openai"
	openrouterprov "github.com/artpar/pragma/internal/provider/openrouter"
)

// modelCatalogTTL bounds how long a fetched model list is reused before a
// background refresh is triggered. Model catalogs change slowly, and the
// /models picker must never block on the network, so refreshes are always
// asynchronous.
const modelCatalogTTL = 10 * time.Minute

// modelFetchTimeout bounds one provider's /models request. A slow provider
// must not hold the whole refresh.
const modelFetchTimeout = 5 * time.Second

// providerModelFetcher fetches the live model list for one provider.
// It is a field on ModelCatalog so tests can substitute it.
type providerModelFetcher func(ctx context.Context, providerName, apiKey, baseURL string) ([]string, error)

// ModelCatalog aggregates the models of every provider that has credentials,
// combining live /models listings with the static per-provider registries.
// It is the data source for the /models picker and for cross-provider
// /model switching.
//
// Picker entries are qualified "provider/model" IDs so identical model IDs
// from different providers cannot collide (for example
// openrouter/z-ai/glm-5.3 versus morphllm/morph-glm53-744b). A bare model
// ID always refers to the active provider.
type ModelCatalog struct {
	creds config.Credentials
	fetch providerModelFetcher
	ttl   time.Duration

	mu              sync.Mutex
	remote          map[string][]string // provider -> fetched model IDs
	fetchErrors     map[string]error    // provider -> last fetch error
	fetchedAt       time.Time
	refreshInFlight bool
}

// NewModelCatalog creates a catalog backed by the given credentials.
// Refreshing is lazy: construction performs no I/O.
func NewModelCatalog(creds config.Credentials) *ModelCatalog {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &ModelCatalog{...}")
	observe.GlobalTrace("return: &ModelCatalog{\n\tcreds:\tcreds,\n\tfetch:\tfetchProviderModels,\n\tttl:\tmodelCatalog...")
	return &ModelCatalog{
		creds: creds,
		fetch: fetchProviderModels,
		ttl:   modelCatalogTTL,
	}
}

// QualifiedModelIDs returns "provider/model" entries for every provider
// with credentials, with the active provider's models first. It never
// blocks on the network: providers without a cached remote listing fall
// back to their static registry, and a stale cache only triggers an
// asynchronous refresh.
func (c *ModelCatalog) QualifiedModelIDs(activeProvider, activeAPIKey string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if c == nil {
		observe.GlobalTrace("if: c == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	c.MaybeRefreshAsync(activeProvider, activeAPIKey)

	c.mu.Lock()
	defer c.mu.Unlock()

	var ids []string
	for _, providerName := range c.providersLocked(activeProvider) {
		observe.GlobalTrace("range c.providersLocked(activeProvider)")
		for _, m := range c.modelsForLocked(providerName) {
			observe.GlobalTrace("range c.modelsForLocked(providerName)")
			ids = append(ids, providerName+"/"+m)
		}
	}
	observe.GlobalTrace("return: ids")
	return ids
}

// ModelsFor returns the model IDs of one provider — the live listing when a
// fetch has succeeded, otherwise the static registry. Used to validate
// /model targets without blocking on the network.
func (c *ModelCatalog) ModelsFor(providerName string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if c == nil {
		observe.GlobalTrace("if: c == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := append([]string(nil), c.modelsForLocked(providerName)...)
	observe.GlobalTrace("return: out")
	return out
}

// Knows reports whether modelID is listed (live or static) for the provider.
func (c *ModelCatalog) Knows(providerName, modelID string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, m := range c.ModelsFor(providerName) {
		observe.GlobalTrace("range c.ModelsFor(providerName)")
		if m == modelID {
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

// FetchErrors returns the last fetch error per provider, for diagnostics
// when a picker unexpectedly shows only the static fallback list.
func (c *ModelCatalog) FetchErrors() map[string]string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if c == nil {
		observe.GlobalTrace("return: nil")
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]string, len(c.fetchErrors))
	for name, err := range c.fetchErrors {
		observe.GlobalTrace("range c.fetchErrors")
		out[name] = err.Error()
	}
	observe.GlobalTrace("return: out")
	return out
}

// MaybeRefreshAsync starts a background refresh when the cache is older
// than the TTL and no refresh is already running. It returns immediately.
func (c *ModelCatalog) MaybeRefreshAsync(activeProvider, activeAPIKey string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if c == nil {
		observe.GlobalTrace("return")
		return
	}
	c.mu.Lock()
	stale := time.Since(c.fetchedAt) > c.ttl && !c.refreshInFlight
	c.mu.Unlock()
	if stale {
		observe.GlobalTrace("if: stale — kick background refresh")
		go c.Refresh(context.Background(), activeProvider, activeAPIKey)
	}
}

// Refresh fetches live model lists for every provider with credentials,
// concurrently, and replaces the cache. Per-provider failures are recorded
// (see FetchErrors) and leave that provider on its static fallback list.
// Only one refresh runs at a time; additional calls while one is in flight
// return immediately.
func (c *ModelCatalog) Refresh(ctx context.Context, activeProvider, activeAPIKey string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if c == nil {
		observe.GlobalTrace("return")
		return
	}
	c.mu.Lock()
	if c.refreshInFlight {
		observe.GlobalTrace("if: c.refreshInFlight")
		c.mu.Unlock()
		observe.GlobalTrace("return")
		return
	}
	c.refreshInFlight = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.refreshInFlight = false
		c.mu.Unlock()
	}()

	fetched := make(map[string][]string)
	errs := make(map[string]error)
	var resultMu sync.Mutex
	var wg sync.WaitGroup
	for _, providerName := range c.eligibleProviders(activeProvider, activeAPIKey) {
		observe.GlobalTrace("range c.eligibleProviders(activeProvider, activeAPIKey)")
		if providerName == "google-vertex" {
			observe.GlobalTrace("if: providerName == \"google-vertex\" — no key-based models endpoint")
			continue
		}
		apiKey := c.apiKeyFor(providerName, activeProvider, activeAPIKey)
		baseURL := c.baseURLFor(providerName)
		wg.Add(1)
		go func(name, key, url string) {
			defer wg.Done()
			fetchCtx, cancel := context.WithTimeout(ctx, modelFetchTimeout)
			defer cancel()
			models, err := c.fetch(fetchCtx, name, key, url)
			resultMu.Lock()
			defer resultMu.Unlock()
			if err != nil {
				observe.TraceCtx(ctx, "cli", "ModelCatalog.Refresh", "if: err != nil")
				errs[name] = err
				return
			}
			fetched[name] = filterPickerModels(name, models)
		}(providerName, apiKey, baseURL)
	}
	wg.Wait()

	c.mu.Lock()
	c.remote = fetched
	c.fetchErrors = errs
	c.fetchedAt = time.Now()
	c.mu.Unlock()
}

// providersLocked returns the eligible provider names, the active provider
// first and the rest alphabetically. Caller holds c.mu.
func (c *ModelCatalog) providersLocked(activeProvider string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var out []string
	seen := map[string]bool{}
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	add(activeProvider)
	var rest []string
	for _, name := range knownProviders {
		observe.GlobalTrace("range knownProviders")
		if name == activeProvider {
			observe.GlobalTrace("if: name == activeProvider")
			continue
		}
		if c.hasCredentials(name) {
			observe.GlobalTrace("if: c.hasCredentials(name)")
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	for _, name := range rest {
		observe.GlobalTrace("range rest")
		add(name)
	}
	observe.GlobalTrace("return: out")
	return out
}

// eligibleProviders returns every provider that can be listed: the active
// provider plus every known provider with credentials.
func (c *ModelCatalog) eligibleProviders(activeProvider, _ string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.mu.Lock()
	defer c.mu.Unlock()
	observe.GlobalTrace("return: c.providersLocked(activeProvider)")
	return c.providersLocked(activeProvider)
}

// modelsForLocked returns the live listing when available, otherwise the
// static registry. Caller holds c.mu.
func (c *ModelCatalog) modelsForLocked(providerName string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if models, ok := c.remote[providerName]; ok && len(models) > 0 {
		observe.GlobalTrace("if: ok && len(models) > 0")
		observe.GlobalTrace("return: models")
		return models
	}
	observe.GlobalTrace("return: staticModelList(providerName)")
	return staticModelList(providerName)
}

// apiKeyFor resolves the API key for a provider the same way startup does:
// the active config's key, then credentials.yml, then the provider's
// environment variable.
func (c *ModelCatalog) apiKeyFor(providerName, activeProvider, activeAPIKey string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if providerName == activeProvider && activeAPIKey != "" {
		observe.GlobalTrace("if: providerName == activeProvider && activeAPIKey != \"\"")
		observe.GlobalTrace("return: activeAPIKey")
		return activeAPIKey
	}
	if key := c.creds.CredentialFor(providerName).APIKey; key != "" {
		observe.GlobalTrace("if: key != \"\"")
		observe.GlobalTrace("return: key")
		return key
	}
	observe.GlobalTrace("return: os.Getenv(envVarForProvider(providerName))")
	return os.Getenv(envVarForProvider(providerName))
}

// hasCredentials reports whether a non-active provider has a stored key.
func (c *ModelCatalog) hasCredentials(providerName string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if c.creds.CredentialFor(providerName).APIKey != "" {
		observe.GlobalTrace("if: c.creds.CredentialFor(providerName).APIKey != \"\"")
		observe.GlobalTrace("return: true")
		return true
	}
	observe.GlobalTrace("return: os.Getenv(envVarForProvider(providerName)) != \"\"")
	return os.Getenv(envVarForProvider(providerName)) != ""
}

// baseURLFor returns the models-endpoint base URL for OpenAI-compatible
// providers. Anthropic and Google use their SDK defaults, matching what
// their chat adapters actually use.
func (c *ModelCatalog) baseURLFor(providerName string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch providerName {
	case "openai", "groq", "morphllm", "openrouter":
		observe.GlobalTrace("case: \"openai\", \"groq\", \"morphllm\", \"openrouter\"")
		return ProviderBaseURL(providerName, c.creds)
	default:
		observe.GlobalTrace("default")
		return ""
	}
}

// fetchProviderModels dispatches a live model listing to the right adapter.
func fetchProviderModels(ctx context.Context, providerName, apiKey, baseURL string) ([]string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch providerName {
	case "anthropic":
		observe.GlobalTrace("case: \"anthropic\"")
		return anthropic.FetchModels(ctx, apiKey, baseURL)
	case "google":
		observe.GlobalTrace("case: \"google\"")
		return googleprov.FetchModels(ctx, apiKey, baseURL)
	case "openai", "groq", "morphllm", "openrouter":
		observe.GlobalTrace("case: openai-compatible")
		return oaiprov.FetchModels(ctx, apiKey, baseURL)
	default:
		observe.GlobalTrace("default")
		return nil, fmt.Errorf("no models endpoint for provider %q", providerName)
	}
}

// staticModelList returns the compiled-in fallback model IDs for a
// provider. google-vertex reuses the Gemini registry as an approximation:
// Vertex serves the same undated Gemini model IDs and has no static
// registry of its own.
func staticModelList(providerName string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch providerName {
	case "anthropic":
		observe.GlobalTrace("case: \"anthropic\"")
		return anthropic.ListModels()
	case "google", "google-vertex":
		observe.GlobalTrace("case: \"google\", \"google-vertex\"")
		return googleprov.ListModels()
	case "groq":
		observe.GlobalTrace("case: \"groq\"")
		return groqprov.ListModels()
	case "openai":
		observe.GlobalTrace("case: \"openai\"")
		return oaiprov.ListModels()
	case "morphllm":
		observe.GlobalTrace("case: \"morphllm\"")
		return []string{morphprov.DefaultModel}
	case "openrouter":
		observe.GlobalTrace("case: \"openrouter\"")
		return []string{openrouterprov.DefaultModel, openrouterprov.FlashModel}
	default:
		observe.GlobalTrace("default")
		return nil
	}
}

// filterPickerModels removes non-chat entries from providers whose /models
// endpoint returns the account's entire model inventory (embeddings, TTS,
// moderation, transcription). Providers with curated lists pass through
// unchanged.
func filterPickerModels(providerName string, models []string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var out []string
	for _, id := range models {
		observe.GlobalTrace("range models")
		if includeInPicker(providerName, id) {
			observe.GlobalTrace("if: includeInPicker(providerName, id)")
			out = append(out, id)
		}
	}
	observe.GlobalTrace("return: out")
	return out
}

// includeInPicker decides whether a fetched model ID belongs in the picker.
func includeInPicker(providerName, modelID string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch providerName {
	case "openai":

		observe.GlobalTrace("case: \"openai\"")
		for _, prefix := range []string{"gpt", "o1", "o3", "o4", "o5", "chatgpt"} {
			observe.GlobalTrace("range prefixes")
			if strings.HasPrefix(modelID, prefix) {
				observe.GlobalTrace("return: true")
				return true
			}
		}
		observe.GlobalTrace("return: false")
		return false
	case "groq":

		observe.GlobalTrace("case: \"groq\"")
		for _, prefix := range []string{"whisper", "guard"} {
			observe.GlobalTrace("range prefixes")
			if strings.HasPrefix(modelID, prefix) {
				observe.GlobalTrace("return: false")
				return false
			}
		}
		observe.GlobalTrace("return: true")
		return true
	default:
		observe.GlobalTrace("default")
		return true
	}
}
