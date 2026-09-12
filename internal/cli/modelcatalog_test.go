package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/config"
)

// clearProviderEnv removes developer-machine provider env vars so catalog
// eligibility tests do not depend on the local environment.
func clearProviderEnv(t *testing.T) {
	t.Helper()
	for _, v := range []string{
		"MORPH_API_KEY", "OPENROUTER_API_KEY", "ANTHROPIC_API_KEY",
		"GOOGLE_API_KEY", "OPENAI_API_KEY", "GROQ_API_KEY",
		"OPENAI_BASE_URL", "OPENROUTER_BASE_URL", "MORPH_BASE_URL", "GOOGLE_BASE_URL",
	} {
		t.Setenv(v, "")
	}
}

func catalogCreds(providers ...string) config.Credentials {
	creds := config.Credentials{Providers: map[string]config.ProviderCredential{}}
	for _, p := range providers {
		creds.Providers[p] = config.ProviderCredential{APIKey: "key-" + p}
	}
	return creds
}

func TestQualifiedModelIDsStaticFallbackActiveFirst(t *testing.T) {
	clearProviderEnv(t)
	catalog := NewModelCatalog(catalogCreds("groq"))

	ids := catalog.QualifiedModelIDs("anthropic", "active-key")
	if len(ids) == 0 {
		t.Fatal("expected non-empty catalog from static registries")
	}
	firstAnthropic := false
	for _, id := range ids {
		if strings.HasPrefix(id, "anthropic/") {
			firstAnthropic = true
			break
		}
		if strings.HasPrefix(id, "groq/") {
			t.Fatalf("groq models listed before active provider anthropic: %v", ids)
		}
	}
	if !firstAnthropic {
		t.Fatalf("no anthropic models in catalog: %v", ids)
	}
	// groq has credentials, so its static list must be present.
	foundGroq := false
	for _, id := range ids {
		if id == "groq/llama-3.3-70b-versatile" {
			foundGroq = true
		}
	}
	if !foundGroq {
		t.Errorf("groq static fallback model missing: %v", ids)
	}
	// morphllm has no credentials, so it must not appear.
	for _, id := range ids {
		if strings.HasPrefix(id, "morphllm/") {
			t.Errorf("morphllm listed without credentials: %v", ids)
		}
	}
}

func TestRefreshPopulatesRemoteAndRecordsErrors(t *testing.T) {
	clearProviderEnv(t)
	catalog := NewModelCatalog(catalogCreds("anthropic", "groq"))
	var fetchMu sync.Mutex
	fetched := map[string][]string{}
	catalog.fetch = func(ctx context.Context, providerName, apiKey, baseURL string) ([]string, error) {
		if apiKey == "" {
			t.Errorf("provider %s fetched without an API key", providerName)
		}
		if providerName == "groq" {
			return nil, errors.New("connection refused")
		}
		fetchMu.Lock()
		fetched[providerName] = []string{"remote-a", "remote-b"}
		fetchMu.Unlock()
		return []string{"remote-a", "remote-b"}, nil
	}

	catalog.Refresh(context.Background(), "anthropic", "active-key")

	if got := catalog.ModelsFor("anthropic"); len(got) != 2 || got[0] != "remote-a" || got[1] != "remote-b" {
		t.Errorf("anthropic remote models = %v, want [remote-a remote-b]", got)
	}
	errs := catalog.FetchErrors()
	if errs["groq"] == "" {
		t.Errorf("groq fetch error not recorded: %v", errs)
	}
	if got := catalog.ModelsFor("groq"); len(got) == 0 {
		t.Error("groq should fall back to its static list after fetch error")
	}
}

func TestRefreshFiltersNonChatModelsFromOpenAI(t *testing.T) {
	clearProviderEnv(t)
	catalog := NewModelCatalog(catalogCreds("openai"))
	catalog.fetch = func(ctx context.Context, providerName, apiKey, baseURL string) ([]string, error) {
		return []string{"gpt-4o", "gpt-4.1", "whisper-1", "tts-1", "text-embedding-3-small", "babbage-002"}, nil
	}

	catalog.Refresh(context.Background(), "openai", "key")

	got := catalog.ModelsFor("openai")
	if len(got) != 2 {
		t.Fatalf("openai models = %v, want only chat models", got)
	}
	// The filter preserves the fetched order; sorting is the adapter's job.
	if got[0] != "gpt-4o" || got[1] != "gpt-4.1" {
		t.Errorf("openai models = %v, want [gpt-4o gpt-4.1]", got)
	}
}

func TestRefreshFiltersGroqNonChatModels(t *testing.T) {
	clearProviderEnv(t)
	catalog := NewModelCatalog(catalogCreds("groq"))
	catalog.fetch = func(ctx context.Context, providerName, apiKey, baseURL string) ([]string, error) {
		return []string{"llama-3.3-70b-versatile", "whisper-large-v3", "openai-gpt-oss-20b"}, nil
	}

	catalog.Refresh(context.Background(), "groq", "key")

	got := catalog.ModelsFor("groq")
	if len(got) != 2 {
		t.Fatalf("groq models = %v, want whisper filtered out", got)
	}
}

func TestRefreshDoesNotFilterCuratedProviders(t *testing.T) {
	clearProviderEnv(t)
	catalog := NewModelCatalog(catalogCreds("morphllm"))
	catalog.fetch = func(ctx context.Context, providerName, apiKey, baseURL string) ([]string, error) {
		return []string{"morph-glm53-744b", "morph-other"}, nil
	}

	catalog.Refresh(context.Background(), "morphllm", "key")

	got := catalog.ModelsFor("morphllm")
	if len(got) != 2 {
		t.Errorf("morphllm models = %v, want the full fetched list", got)
	}
}

func TestVertexNeverFetchedButStaticWhenActive(t *testing.T) {
	clearProviderEnv(t)
	catalog := NewModelCatalog(config.Credentials{})
	var calls int32
	catalog.fetch = func(ctx context.Context, providerName, apiKey, baseURL string) ([]string, error) {
		atomic.AddInt32(&calls, 1)
		return []string{"should-not-happen"}, nil
	}

	catalog.Refresh(context.Background(), "google-vertex", "")
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("google-vertex fetched %d times, want 0 (ADC-based provider)", n)
	}

	ids := catalog.QualifiedModelIDs("google-vertex", "")
	if len(ids) == 0 {
		t.Fatal("active google-vertex should still list its static fallback")
	}
	for _, id := range ids {
		if !strings.HasPrefix(id, "google-vertex/") {
			t.Fatalf("unexpected catalog entry %q", id)
		}
	}
}

func TestKnowsStaticAndRemote(t *testing.T) {
	clearProviderEnv(t)
	catalog := NewModelCatalog(catalogCreds("morphllm"))

	if !catalog.Knows("morphllm", "morph-glm53-744b") {
		t.Error("morphllm static model should be known before any fetch")
	}
	if catalog.Knows("morphllm", "morph-does-not-exist") {
		t.Error("unknown morphllm model should not be known")
	}

	catalog.fetch = func(ctx context.Context, providerName, apiKey, baseURL string) ([]string, error) {
		return []string{"morph-live-model"}, nil
	}
	catalog.Refresh(context.Background(), "morphllm", "key")
	if !catalog.Knows("morphllm", "morph-live-model") {
		t.Error("remote-listed model should be known after refresh")
	}
}

func TestMaybeRefreshAsyncRefreshesStaleCache(t *testing.T) {
	clearProviderEnv(t)
	catalog := NewModelCatalog(catalogCreds("morphllm"))
	var calls int32
	catalog.fetch = func(ctx context.Context, providerName, apiKey, baseURL string) ([]string, error) {
		atomic.AddInt32(&calls, 1)
		return []string{"morph-live-model"}, nil
	}

	// A fresh catalog has never fetched, so it is stale immediately.
	catalog.MaybeRefreshAsync("morphllm", "key")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&calls) > 0 && catalog.Knows("morphllm", "morph-live-model") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("MaybeRefreshAsync did not populate the cache within 2s")
}

func TestQualifiedModelIDsUsesRemoteListings(t *testing.T) {
	clearProviderEnv(t)
	catalog := NewModelCatalog(catalogCreds("anthropic", "groq"))
	catalog.fetch = func(ctx context.Context, providerName, apiKey, baseURL string) ([]string, error) {
		return []string{fmt.Sprintf("%s-live-model", providerName)}, nil
	}
	catalog.Refresh(context.Background(), "anthropic", "key")

	ids := catalog.QualifiedModelIDs("anthropic", "key")
	wantFirst := "anthropic/anthropic-live-model"
	if len(ids) == 0 || ids[0] != wantFirst {
		t.Fatalf("first catalog entry = %v, want %q", ids, wantFirst)
	}
	found := false
	for _, id := range ids {
		if id == "groq/groq-live-model" {
			found = true
		}
	}
	if !found {
		t.Errorf("groq remote model missing from catalog: %v", ids)
	}
}

func TestParseModelTarget(t *testing.T) {
	cases := []struct {
		input          string
		activeProvider string
		wantProvider   string
		wantModel      string
	}{
		{"google/gemini-2.5-flash", "morphllm", "google", "gemini-2.5-flash"},
		{"anthropic/claude-sonnet-4-6-20250514", "anthropic", "anthropic", "claude-sonnet-4-6-20250514"},
		{"z-ai/glm-5.3", "openrouter", "openrouter", "z-ai/glm-5.3"},
		{"morph-glm53-744b", "morphllm", "morphllm", "morph-glm53-744b"},
		{"", "morphllm", "morphllm", ""},
	}
	for _, tc := range cases {
		gotProvider, gotModel := parseModelTarget(tc.input, tc.activeProvider)
		if gotProvider != tc.wantProvider || gotModel != tc.wantModel {
			t.Errorf("parseModelTarget(%q, %q) = (%q, %q), want (%q, %q)",
				tc.input, tc.activeProvider, gotProvider, gotModel, tc.wantProvider, tc.wantModel)
		}
	}
}

func TestIsKnownProvider(t *testing.T) {
	if !isKnownProvider("morphllm") || !isKnownProvider("google-vertex") {
		t.Error("expected known providers to be recognized")
	}
	if isKnownProvider("z-ai") {
		t.Error("z-ai is a vendor prefix inside OpenRouter model IDs, not a provider")
	}
}
