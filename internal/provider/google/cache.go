package google

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/artpar/pragma/internal/observe"
	"google.golang.org/genai"
)

// minCacheTokens is the minimum token count for context caching.
// Gemini requires 1024 tokens for Flash models, 4096 for Pro models.
// We use the lower bound; the API will reject if too small for a given model.
const minCacheTokens = 1024

// cacheTTL is the time-to-live for cached content on the Gemini server.
const cacheTTL = 5 * time.Minute

// cacheManager manages a single active Gemini context cache for the provider.
// It tracks the hash of the cached prefix and creates/deletes caches as the
// prefix changes across turns.
type cacheManager struct {
	client  *genai.Client
	bus     *observe.EventBus
	mu      sync.Mutex
	current *cacheEntry
}

type cacheEntry struct {
	name      string   // genai cache resource name
	hash      [32]byte // SHA-256 of cached content
	expiresAt time.Time
}

func newCacheManager(client *genai.Client, bus *observe.EventBus) *cacheManager {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &cacheManager{client: client, bus: bus}")
	return &cacheManager{client: client, bus: bus}
}

// getOrCreateCache checks if the stable prefix matches the current cache.
// Returns the cache resource name if a cache is active, or empty string if
// caching should be skipped (prefix too small, error creating, etc.).
//
// stableContents: all messages except the last user message.
// sysInstruction: the system prompt content (may be nil).
// tools: the genai tools list (may be nil).
// model: the model ID for cache creation.
// prefixTokens: the token count of the stable prefix (from CountTokens).
func (cm *cacheManager) getOrCreateCache(
	ctx context.Context,
	model string,
	stableContents []*genai.Content,
	sysInstruction *genai.Content,
	tools []*genai.Tool,
	prefixTokens int,
) (string, error) {
	observe.TraceCtx(ctx, "google", "cacheManager.getOrCreateCache", "enter")
	defer observe.TraceCtx(ctx, "google", "cacheManager.getOrCreateCache", "exit")

	if prefixTokens < minCacheTokens {
		observe.TraceCtx(ctx, "google", "cacheManager.getOrCreateCache", fmt.Sprintf("skip: prefix %d tokens < min %d", prefixTokens, minCacheTokens))
		observe.TraceCtx(ctx, "google", "cacheManager.getOrCreateCache", "return: \"\", nil")
		return "", nil
	}

	hash := hashPrefix(stableContents, sysInstruction, tools)

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.current != nil && cm.current.hash == hash && time.Now().Before(cm.current.expiresAt) {
		observe.TraceCtx(ctx, "google", "cacheManager.getOrCreateCache", fmt.Sprintf("cache hit: %s (%d tokens)", cm.current.name, prefixTokens))
		observe.TraceCtx(ctx, "google", "cacheManager.getOrCreateCache", "return: cm.current.name, nil")
		return cm.current.name, nil
	}

	if cm.current != nil {
		observe.TraceCtx(ctx, "google", "cacheManager.getOrCreateCache", "cache miss, deleting old")
		deleteCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, delErr := cm.client.Caches.Delete(deleteCtx, cm.current.name, nil)
		cancel()
		if delErr != nil {
			observe.TraceCtx(ctx, "google", "cacheManager.getOrCreateCache", fmt.Sprintf("delete old cache error: %v", delErr))
		}
		cm.current = nil
	}

	observe.TraceCtx(ctx, "google", "cacheManager.getOrCreateCache", fmt.Sprintf("creating cache for %d tokens", prefixTokens))
	cached, err := cm.client.Caches.Create(ctx, model, &genai.CreateCachedContentConfig{
		Contents:          stableContents,
		SystemInstruction: sysInstruction,
		Tools:             tools,
		TTL:               cacheTTL,
		DisplayName:       "gogent-session-cache",
	})
	if err != nil {
		observe.TraceCtx(ctx, "google", "cacheManager.getOrCreateCache", fmt.Sprintf("create cache error: %v", err))
		observe.TraceCtx(ctx, "google", "cacheManager.getOrCreateCache", "return: \"\", fmt.Errorf(\"google: create cache: %w\", err)")
		return "", fmt.Errorf("google: create cache: %w", err)
	}

	cm.current = &cacheEntry{
		name:      cached.Name,
		hash:      hash,
		expiresAt: time.Now().Add(cacheTTL),
	}

	observe.TraceCtx(ctx, "google", "cacheManager.getOrCreateCache", fmt.Sprintf("cache created: %s (%d tokens, TTL %s)", cached.Name, prefixTokens, cacheTTL))
	observe.TraceCtx(ctx, "google", "cacheManager.getOrCreateCache", "return: cached.Name, nil")
	return cached.Name, nil
}

// hashPrefix computes a SHA-256 hash of the stable prefix for cache invalidation.
func hashPrefix(contents []*genai.Content, sysInstruction *genai.Content, tools []*genai.Tool) [32]byte {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	h := sha256.New()

	if sysInstruction != nil {
		observe.GlobalTrace("if: sysInstruction != nil")
		sysBytes, _ := json.Marshal(sysInstruction)
		h.Write(sysBytes)
	}

	for _, c := range contents {
		observe.GlobalTrace("range contents")
		cBytes, _ := json.Marshal(c)
		h.Write(cBytes)
	}

	if len(tools) > 0 {
		observe.GlobalTrace("if: len(tools) > 0")
		tBytes, _ := json.Marshal(tools)
		h.Write(tBytes)
	}

	var result [32]byte
	copy(result[:], h.Sum(nil))
	observe.GlobalTrace("return: result")
	return result
}

// splitStablePrefix separates the "stable" prefix (all but last user message)
// from the "tail" (last user message). The stable prefix is what gets cached.
func splitStablePrefix(contents []*genai.Content) (stable []*genai.Content, tail []*genai.Content) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(contents) == 0 {
		observe.GlobalTrace("if: len(contents) == 0")
		observe.GlobalTrace("return: nil, nil")
		return nil, nil
	}

	lastUserIdx := -1
	for i := len(contents) - 1; i >= 0; i-- {
		observe.GlobalTrace("for: i >= 0")
		if contents[i].Role == "user" {
			observe.GlobalTrace("if: contents[i].Role == \"user\"")
			lastUserIdx = i
			break
		}
	}

	if lastUserIdx <= 0 {

		observe.GlobalTrace("return: nil, contents")
		return nil, contents
	}

	observe.GlobalTrace("return: contents[:lastUserIdx], contents[lastUserIdx:]")
	return contents[:lastUserIdx], contents[lastUserIdx:]
}
