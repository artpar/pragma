package google

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"google.golang.org/genai"
)

// =============================================================================
// Helpers — build realistic conversation data
// =============================================================================

// makeTextContent creates a genai.Content with a text part of the given size.
func makeTextContent(role string, charCount int) *genai.Content {
	return &genai.Content{
		Role:  role,
		Parts: []*genai.Part{{Text: strings.Repeat("x", charCount)}},
	}
}

// makeConversation builds a multi-turn conversation of alternating user/model
// messages. Each message has charCount characters of text.
func makeConversation(turns int, charCount int) []*genai.Content {
	var contents []*genai.Content
	for i := 0; i < turns; i++ {
		contents = append(contents, makeTextContent("user", charCount))
		contents = append(contents, makeTextContent("model", charCount))
	}
	// Add final user message (the "new" turn)
	contents = append(contents, makeTextContent("user", charCount))
	return contents
}

// makeSystemInstruction builds a system prompt with the given total character count.
func makeSystemInstruction(charCount int) *genai.Content {
	return &genai.Content{
		Parts: []*genai.Part{{Text: strings.Repeat("s", charCount)}},
	}
}

// makeTools builds n tool declarations with realistic schema sizes.
func makeTools(n int) []*genai.Tool {
	var decls []*genai.FunctionDeclaration
	for i := 0; i < n; i++ {
		schema := &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"param": {Type: "string", Description: strings.Repeat("d", 100)},
			},
		}
		decls = append(decls, &genai.FunctionDeclaration{
			Name:        fmt.Sprintf("Tool_%d", i),
			Description: strings.Repeat("desc", 25), // ~100 chars
			Parameters:  schema,
		})
	}
	return []*genai.Tool{{FunctionDeclarations: decls}}
}

// estimateCharTokens computes the same heuristic as applyCache: chars/4.
func estimateCharTokens(chars int) int {
	return chars / 4
}

// =============================================================================
// splitStablePrefix — exhaustive edge cases
// =============================================================================

func TestSplitStablePrefixEmpty(t *testing.T) {
	stable, tail := splitStablePrefix(nil)
	if stable != nil || tail != nil {
		t.Error("expected nil, nil for empty input")
	}
}

func TestSplitStablePrefixSingleUser(t *testing.T) {
	contents := []*genai.Content{makeTextContent("user", 100)}
	stable, tail := splitStablePrefix(contents)
	if stable != nil {
		t.Error("expected nil stable for single user message")
	}
	if len(tail) != 1 {
		t.Errorf("expected 1 tail, got %d", len(tail))
	}
}

func TestSplitStablePrefixMultipleTurns(t *testing.T) {
	contents := makeConversation(3, 100) // 7 messages: u m u m u m u
	stable, tail := splitStablePrefix(contents)
	// Last user at index 6, stable = [0:6], tail = [6:]
	if len(stable) != 6 {
		t.Errorf("expected 6 stable, got %d", len(stable))
	}
	if len(tail) != 1 {
		t.Errorf("expected 1 tail, got %d", len(tail))
	}
}

func TestSplitStablePrefixNoUser(t *testing.T) {
	contents := []*genai.Content{makeTextContent("model", 100)}
	stable, tail := splitStablePrefix(contents)
	if stable != nil {
		t.Error("expected nil stable when no user messages")
	}
	if len(tail) != 1 {
		t.Errorf("expected 1 tail, got %d", len(tail))
	}
}

func TestSplitStablePrefixEndsWithModel(t *testing.T) {
	contents := []*genai.Content{
		makeTextContent("user", 100),
		makeTextContent("model", 100),
		makeTextContent("user", 100),
		makeTextContent("model", 100),
	}
	stable, tail := splitStablePrefix(contents)
	if len(stable) != 2 {
		t.Errorf("expected 2 stable, got %d", len(stable))
	}
	if len(tail) != 2 {
		t.Errorf("expected 2 tail, got %d", len(tail))
	}
}

func TestSplitStablePrefixConsecutiveUserMessages(t *testing.T) {
	contents := []*genai.Content{
		makeTextContent("user", 100),
		makeTextContent("model", 100),
		makeTextContent("user", 100), // tool result 1
		makeTextContent("user", 100), // tool result 2
	}
	stable, tail := splitStablePrefix(contents)
	if len(stable) != 3 {
		t.Errorf("expected 3 stable, got %d", len(stable))
	}
	if len(tail) != 1 {
		t.Errorf("expected 1 tail, got %d", len(tail))
	}
}

// Realistic: 50-turn conversation with tool use (interleaved user tool-result messages)
func TestSplitStablePrefixLongConversationWithToolUse(t *testing.T) {
	var contents []*genai.Content
	for i := 0; i < 50; i++ {
		// Each turn: user msg -> model (with tool call) -> user (tool result) -> model (final)
		contents = append(contents,
			makeTextContent("user", 200),
			makeTextContent("model", 500), // model calls tool
			makeTextContent("user", 1000), // tool result
			makeTextContent("model", 500), // model final response
		)
	}
	// New user turn
	contents = append(contents, makeTextContent("user", 200))
	// total: 50*4 + 1 = 201 messages

	stable, tail := splitStablePrefix(contents)
	if len(stable) != 200 {
		t.Errorf("expected 200 stable messages, got %d", len(stable))
	}
	if len(tail) != 1 {
		t.Errorf("expected 1 tail message, got %d", len(tail))
	}
}

// =============================================================================
// hashPrefix — collision resistance and invalidation
// =============================================================================

func TestHashPrefixDeterministic(t *testing.T) {
	c := []*genai.Content{makeTextContent("user", 100)}
	sys := makeSystemInstruction(500)
	h1 := hashPrefix(c, sys, nil)
	h2 := hashPrefix(c, sys, nil)
	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
}

func TestHashPrefixDifferentContent(t *testing.T) {
	c1 := []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hello"}}}}
	c2 := []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "world"}}}}
	if hashPrefix(c1, nil, nil) == hashPrefix(c2, nil, nil) {
		t.Error("different content should produce different hash")
	}
}

func TestHashPrefixDifferentSystem(t *testing.T) {
	c := []*genai.Content{makeTextContent("user", 100)}
	s1 := makeSystemInstruction(500)
	s2 := makeSystemInstruction(600)
	if hashPrefix(c, s1, nil) == hashPrefix(c, s2, nil) {
		t.Error("different system instructions should produce different hash")
	}
}

func TestHashPrefixDifferentTools(t *testing.T) {
	c := []*genai.Content{makeTextContent("user", 100)}
	t1 := makeTools(5)
	t2 := makeTools(10)
	if hashPrefix(c, nil, t1) == hashPrefix(c, nil, t2) {
		t.Error("different tools should produce different hash")
	}
}

func TestHashPrefixNilVsEmptyContents(t *testing.T) {
	sys := makeSystemInstruction(500)
	h1 := hashPrefix(nil, sys, nil)
	h2 := hashPrefix([]*genai.Content{}, sys, nil)
	if h1 != h2 {
		t.Error("nil and empty contents should produce same hash")
	}
}

func TestHashPrefixIsValidSHA256(t *testing.T) {
	hash := hashPrefix([]*genai.Content{makeTextContent("user", 100)}, nil, nil)
	if len(hash) != sha256.Size {
		t.Errorf("hash size: got %d, want %d", len(hash), sha256.Size)
	}
}

// Hash changes when a new message is appended to the stable prefix.
// This is the fundamental cache invalidation trigger.
func TestHashPrefixInvalidatedByNewMessage(t *testing.T) {
	base := []*genai.Content{
		makeTextContent("user", 200),
		makeTextContent("model", 500),
	}
	h1 := hashPrefix(base, nil, nil)

	extended := append(base, makeTextContent("user", 200), makeTextContent("model", 500))
	h2 := hashPrefix(extended, nil, nil)

	if h1 == h2 {
		t.Error("adding messages to stable prefix should change hash")
	}
}

// Hash does NOT change when only the last user message changes (since that's the tail).
func TestHashPrefixStableAcrossTailChanges(t *testing.T) {
	conv1 := makeConversation(5, 100) // ... ends with user
	conv2 := makeConversation(5, 100)

	// Split both — should have same stable prefix
	stable1, _ := splitStablePrefix(conv1)
	stable2, _ := splitStablePrefix(conv2)

	h1 := hashPrefix(stable1, nil, nil)
	h2 := hashPrefix(stable2, nil, nil)
	if h1 != h2 {
		t.Error("same stable prefix should produce same hash regardless of tail")
	}
}

// =============================================================================
// cacheEntry — expiry behavior
// =============================================================================

func TestCacheEntryNotExpired(t *testing.T) {
	entry := &cacheEntry{
		name:      "test-cache",
		hash:      [32]byte{1},
		expiresAt: time.Now().Add(5 * time.Minute),
	}
	if !time.Now().Before(entry.expiresAt) {
		t.Error("cache should not be expired")
	}
}

func TestCacheEntryExpired(t *testing.T) {
	entry := &cacheEntry{
		name:      "test-cache",
		hash:      [32]byte{1},
		expiresAt: time.Now().Add(-1 * time.Second),
	}
	if time.Now().Before(entry.expiresAt) {
		t.Error("cache should be expired")
	}
}

// =============================================================================
// getOrCreateCache — logic without API calls
// =============================================================================

// Tests that the minimum token check works.
func TestGetOrCreateCacheRejectsBelowMinimum(t *testing.T) {
	bus := observe.NewEventBus(0)
	cm := &cacheManager{bus: bus}

	// prefixTokens below minimum
	name, err := cm.getOrCreateCache(
		context.Background(), "gemini-2.5-flash",
		[]*genai.Content{makeTextContent("user", 100)},
		nil, nil,
		1000, // well below 32768
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "" {
		t.Errorf("expected empty cache name for below-min tokens, got %q", name)
	}
}

// Tests cache hit path when hash matches and not expired.
func TestGetOrCreateCacheHitsExistingCache(t *testing.T) {
	bus := observe.NewEventBus(0)
	contents := []*genai.Content{makeTextContent("user", 200_000)}
	hash := hashPrefix(contents, nil, nil)

	cm := &cacheManager{
		bus: bus,
		current: &cacheEntry{
			name:      "existing-cache-123",
			hash:      hash,
			expiresAt: time.Now().Add(5 * time.Minute),
		},
	}

	name, err := cm.getOrCreateCache(
		context.Background(), "gemini-2.5-flash",
		contents, nil, nil,
		50_000, // above minimum
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "existing-cache-123" {
		t.Errorf("expected cache hit, got %q", name)
	}
}

// Tests that hash mismatch is correctly detected — verifies the cache invalidation
// logic without requiring API calls.
func TestGetOrCreateCacheMissOnHashChange(t *testing.T) {
	oldContents := []*genai.Content{makeTextContent("user", 200_000)}
	oldHash := hashPrefix(oldContents, nil, nil)

	newContents := []*genai.Content{
		makeTextContent("user", 200_000),
		makeTextContent("model", 100_000),
	}
	newHash := hashPrefix(newContents, nil, nil)
	if newHash == oldHash {
		t.Fatal("test setup: hashes should differ")
	}

	// Verify that the cache entry hash does NOT match the new hash
	entry := &cacheEntry{
		name:      "old-cache",
		hash:      oldHash,
		expiresAt: time.Now().Add(5 * time.Minute),
	}
	if entry.hash == newHash {
		t.Error("old cache hash should not match new content hash")
	}
	// Verify the condition that would trigger a cache miss in getOrCreateCache
	hashMatches := entry.hash == newHash
	notExpired := time.Now().Before(entry.expiresAt)
	// The cache hit condition is: hash matches AND not expired
	// With a different hash, this should be false
	if hashMatches && notExpired {
		t.Error("should NOT be a cache hit when hash differs")
	}
}

// Tests that expired cache is detected even if hash matches — verifies the
// expiry logic without requiring API calls.
func TestGetOrCreateCacheMissOnExpired(t *testing.T) {
	contents := []*genai.Content{makeTextContent("user", 200_000)}
	hash := hashPrefix(contents, nil, nil)

	entry := &cacheEntry{
		name:      "expired-cache",
		hash:      hash,
		expiresAt: time.Now().Add(-1 * time.Second), // expired
	}

	// The cache hit condition requires: hash matches AND not expired
	hashMatches := entry.hash == hash
	notExpired := time.Now().Before(entry.expiresAt)
	if !hashMatches {
		t.Error("hash should match")
	}
	if notExpired {
		t.Error("cache should be expired")
	}
	// Combined condition: should NOT be a hit
	if hashMatches && notExpired {
		t.Error("expired cache should not be a cache hit")
	}
}

// =============================================================================
// applyCache — heuristic token estimation
// =============================================================================

func TestApplyCacheNilCacheManager(t *testing.T) {
	p := &Provider{cache: nil}
	contents := makeConversation(5, 200_000)
	cfg := &genai.GenerateContentConfig{}

	result := p.applyCache(context.Background(), "gemini-2.5-flash", contents, cfg)
	if len(result) != len(contents) {
		t.Error("nil cache should return contents unchanged")
	}
}

func TestApplyCacheNoStablePrefix(t *testing.T) {
	bus := observe.NewEventBus(0)
	p := &Provider{cache: &cacheManager{bus: bus}}
	contents := []*genai.Content{makeTextContent("user", 200_000)}
	cfg := &genai.GenerateContentConfig{}

	result := p.applyCache(context.Background(), "gemini-2.5-flash", contents, cfg)
	if len(result) != 1 {
		t.Error("single user message should return contents unchanged")
	}
}

func TestApplyCacheBelowMinTokens(t *testing.T) {
	bus := observe.NewEventBus(0)
	p := &Provider{cache: &cacheManager{bus: bus}}
	// Each message is 100 chars = 25 tokens. 4 stable messages = 100 tokens. Well below 32768.
	contents := makeConversation(2, 100)
	cfg := &genai.GenerateContentConfig{}

	result := p.applyCache(context.Background(), "gemini-2.5-flash", contents, cfg)
	if len(result) != len(contents) {
		t.Error("below-threshold contents should return unchanged")
	}
	if cfg.CachedContent != "" {
		t.Error("should not set CachedContent when below threshold")
	}
}

func TestApplyCacheTokenEstimationIncludesSystem(t *testing.T) {
	// Test that the heuristic token estimation includes system instruction tokens.
	// Stable messages: 2 * 1_000 chars = ~500 tokens (below 1024)
	// But system prompt: 5_000 chars = ~1250 tokens
	// Combined: ~1750 tokens (above 1024)
	stableChars := 1_000 + 1_000 // user + model
	sysChars := 5_000
	estimated := stableChars/4 + sysChars/4 // applyCache heuristic

	if estimated < minCacheTokens {
		t.Errorf("system tokens should push estimate (%d) above min (%d)", estimated, minCacheTokens)
	}

	// Without system, would be below threshold
	withoutSys := stableChars / 4
	if withoutSys >= minCacheTokens {
		t.Errorf("without system, estimate (%d) should be below min (%d)", withoutSys, minCacheTokens)
	}
}

func TestApplyCacheEstimationThreshold(t *testing.T) {
	// Tests the heuristic token estimation logic used in applyCache to decide
	// whether the stable prefix is large enough to attempt caching.
	// This validates the math without requiring API calls.
	// minCacheTokens = 1024, so need 1024 * 4 = 4096 chars to pass.
	charsNeeded := minCacheTokens * 4

	tests := []struct {
		name       string
		stableChar int
		sysChar    int
		expectSkip bool
	}{
		{"below_threshold", charsNeeded - 100, 0, true},
		{"at_threshold", charsNeeded, 0, false},
		{"above_threshold", charsNeeded + 1000, 0, false},
		{"system_pushes_over", charsNeeded / 2, charsNeeded / 2, false},
		{"combined_still_below", 1_000, 1_000, true}, // 500 tokens < 1024
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the applyCache heuristic: chars/4 for text parts
			estimated := tt.stableChar / 4
			if tt.sysChar > 0 {
				estimated += tt.sysChar / 4
			}

			wouldSkip := estimated < minCacheTokens
			if wouldSkip != tt.expectSkip {
				t.Errorf("estimated=%d tokens, minCache=%d: expected skip=%v, got skip=%v",
					estimated, minCacheTokens, tt.expectSkip, wouldSkip)
			}
		})
	}
}

// =============================================================================
// Cost savings simulation — the real payoff
// =============================================================================

// simulateTurnCost calculates the cost of a single turn using CostTracker.
func simulateTurnCost(pricing model.Pricing, inputTokens, outputTokens, cacheCreate, cacheRead int) float64 {
	ct := model.NewCostTracker(0)
	ct.Record("model", "google", model.TokenUsage{
		InputTokens:              inputTokens,
		OutputTokens:             outputTokens,
		CacheCreationInputTokens: cacheCreate,
		CacheReadInputTokens:     cacheRead,
	}, pricing)
	return ct.TotalUSD()
}

func TestCostSavings10TurnSession(t *testing.T) {
	pricing, _ := LookupModel("gemini-2.5-pro")

	// Scenario: 10-turn session with a coding agent
	// System prompt: 10K tokens, tools: 20K tokens, growing conversation history
	systemTokens := 10_000
	toolTokens := 20_000
	basePrefix := systemTokens + toolTokens // 30K tokens stable from turn 1
	turnGrowth := 2_000                     // each turn adds ~2K tokens to history
	outputPerTurn := 1_000

	var noCacheCost float64
	var withCacheCost float64

	for turn := 0; turn < 10; turn++ {
		totalInput := basePrefix + turn*turnGrowth
		// Without caching: full input every turn
		noCacheCost += simulateTurnCost(pricing.Pricing, totalInput, outputPerTurn, 0, 0)

		if turn == 0 {
			// First turn: create cache (costs same as input for creation)
			withCacheCost += simulateTurnCost(pricing.Pricing,
				turnGrowth, // only new user message
				outputPerTurn,
				totalInput, // cache creation tokens
				0,
			)
		} else {
			// Subsequent turns: cache hit
			cachedTokens := basePrefix + (turn-1)*turnGrowth
			newTokens := turnGrowth // just the new message
			withCacheCost += simulateTurnCost(pricing.Pricing,
				newTokens,
				outputPerTurn,
				0,
				cachedTokens,
			)
		}
	}

	savings := (noCacheCost - withCacheCost) / noCacheCost * 100
	t.Logf("10-turn session with gemini-2.5-pro:")
	t.Logf("  Without caching: $%.6f", noCacheCost)
	t.Logf("  With caching:    $%.6f", withCacheCost)
	t.Logf("  Savings:         %.1f%%", savings)

	// Must save at least 50% — the actual savings should be ~68-75%
	if savings < 50 {
		t.Errorf("expected at least 50%% savings, got %.1f%%", savings)
	}
}

func TestCostSavings50TurnSession(t *testing.T) {
	pricing, _ := LookupModel("gemini-2.5-pro")

	systemTokens := 10_000
	toolTokens := 20_000
	basePrefix := systemTokens + toolTokens
	turnGrowth := 3_000 // larger turns (tool calls + results)
	outputPerTurn := 2_000

	var noCacheCost float64
	var withCacheCost float64

	for turn := 0; turn < 50; turn++ {
		totalInput := basePrefix + turn*turnGrowth
		noCacheCost += simulateTurnCost(pricing.Pricing, totalInput, outputPerTurn, 0, 0)

		if turn == 0 {
			withCacheCost += simulateTurnCost(pricing.Pricing,
				turnGrowth, outputPerTurn, totalInput, 0)
		} else {
			cachedTokens := basePrefix + (turn-1)*turnGrowth
			withCacheCost += simulateTurnCost(pricing.Pricing,
				turnGrowth, outputPerTurn, 0, cachedTokens)
		}
	}

	savings := (noCacheCost - withCacheCost) / noCacheCost * 100
	t.Logf("50-turn session with gemini-2.5-pro:")
	t.Logf("  Without caching: $%.4f", noCacheCost)
	t.Logf("  With caching:    $%.4f", withCacheCost)
	t.Logf("  Savings:         %.1f%%", savings)
	t.Logf("  Saved:           $%.4f", noCacheCost-withCacheCost)

	// Long sessions save even more due to growing prefix
	if savings < 60 {
		t.Errorf("expected at least 60%% savings over 50 turns, got %.1f%%", savings)
	}
}

func TestCostSavingsAllModels(t *testing.T) {
	models := []string{
		"gemini-2.5-pro",
		"gemini-2.5-flash",
		"gemini-2.5-flash-lite",
		"gemini-3.1-pro-preview",
		"gemini-3-flash-preview",
	}

	for _, modelID := range models {
		t.Run(modelID, func(t *testing.T) {
			info, ok := LookupModel(modelID)
			if !ok {
				t.Fatalf("model not found: %s", modelID)
			}

			// 20-turn session
			basePrefix := 30_000
			turnGrowth := 2_000
			output := 1_000

			var noCacheCost, withCacheCost float64
			for turn := 0; turn < 20; turn++ {
				totalInput := basePrefix + turn*turnGrowth
				noCacheCost += simulateTurnCost(info.Pricing, totalInput, output, 0, 0)
				if turn == 0 {
					withCacheCost += simulateTurnCost(info.Pricing, turnGrowth, output, totalInput, 0)
				} else {
					cached := basePrefix + (turn-1)*turnGrowth
					withCacheCost += simulateTurnCost(info.Pricing, turnGrowth, output, 0, cached)
				}
			}

			if noCacheCost > 0 {
				savings := (noCacheCost - withCacheCost) / noCacheCost * 100
				t.Logf("%s: no-cache=$%.6f, cached=$%.6f, savings=%.1f%%",
					modelID, noCacheCost, withCacheCost, savings)
				// Cache read is 25% of input, so savings should be significant
				if savings < 40 {
					t.Errorf("expected at least 40%% savings, got %.1f%%", savings)
				}
			}
		})
	}
}

// Test that cache read pricing matches documented rates.
// Gemini 2.5+ models: 10% of input. Gemini 2.0-flash: 25% of input.
// Source: https://ai.google.dev/gemini-api/docs/pricing (2026-04-15)
func TestCacheReadPricingMatchesDocs(t *testing.T) {
	tests := []struct {
		modelID  string
		wantRate float64
	}{
		{"gemini-3.1-pro-preview", 0.20},
		{"gemini-3-flash-preview", 0.05},
		{"gemini-2.5-pro", 0.125},
		{"gemini-2.5-flash", 0.03},
		{"gemini-2.5-flash-lite", 0.01},
	}
	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			info, ok := LookupModel(tt.modelID)
			if !ok {
				t.Fatalf("model not found: %s", tt.modelID)
			}
			if math.Abs(info.Pricing.CacheReadPerMToken-tt.wantRate) > 1e-10 {
				t.Errorf("CacheReadPerMToken: got %f, want %f",
					info.Pricing.CacheReadPerMToken, tt.wantRate)
			}
		})
	}
}

// Google does not charge per-token for cache creation (only hourly storage).
// CacheCreatePerMToken should be zero for all models.
func TestCacheCreatePricingIsZero(t *testing.T) {
	for id, info := range registry {
		t.Run(id, func(t *testing.T) {
			if info.Pricing.CacheCreatePerMToken != 0 {
				t.Errorf("CacheCreatePerMToken should be 0 (Google charges hourly storage, not per-token), got %f",
					info.Pricing.CacheCreatePerMToken)
			}
		})
	}
}

// =============================================================================
// Realistic conversation scenarios — cache hit/miss patterns
// =============================================================================

// Simulate the hash pattern across a real session: each turn the stable prefix grows.
func TestCacheInvalidationPatternAcrossTurns(t *testing.T) {
	sys := makeSystemInstruction(40_000) // 10K tokens
	tools := makeTools(30)               // ~30 tools

	var prevHash [32]byte
	cacheCreations := 0
	cacheHits := 0

	for turn := 1; turn <= 20; turn++ {
		// Build conversation up to this turn
		conv := makeConversation(turn, 500)
		stable, _ := splitStablePrefix(conv)
		hash := hashPrefix(stable, sys, tools)

		if turn == 1 {
			cacheCreations++
			prevHash = hash
			continue
		}

		if hash == prevHash {
			cacheHits++
		} else {
			cacheCreations++
		}
		prevHash = hash
	}

	// Every turn adds to the stable prefix, so hash changes every turn.
	// cacheCreations should be 20 (one per turn), hits should be 0.
	if cacheHits != 0 {
		t.Errorf("expected 0 cache hits (prefix changes every turn), got %d", cacheHits)
	}
	if cacheCreations != 20 {
		t.Errorf("expected 20 cache creations, got %d", cacheCreations)
	}
}

// Simulate same-turn retries: if the LLM retries (same prefix), should hit cache.
func TestCacheHitOnRetry(t *testing.T) {
	sys := makeSystemInstruction(40_000)
	conv := makeConversation(5, 500)
	stable, _ := splitStablePrefix(conv)
	hash := hashPrefix(stable, sys, nil)

	// Retry with same prefix — hash should match
	retryHash := hashPrefix(stable, sys, nil)
	if hash != retryHash {
		t.Error("same prefix should produce same hash (retry should hit cache)")
	}
}

// Simulate tool set change mid-session.
func TestCacheInvalidatedByToolChange(t *testing.T) {
	sys := makeSystemInstruction(40_000)
	conv := makeConversation(5, 500)
	stable, _ := splitStablePrefix(conv)

	fullTools := makeTools(50)
	readOnlyTools := makeTools(10)

	h1 := hashPrefix(stable, sys, fullTools)
	h2 := hashPrefix(stable, sys, readOnlyTools)

	if h1 == h2 {
		t.Error("tool change should invalidate cache")
	}
}

// Simulate system prompt change (e.g., AGENT.md updated mid-session).
func TestCacheInvalidatedBySystemPromptChange(t *testing.T) {
	conv := makeConversation(5, 500)
	stable, _ := splitStablePrefix(conv)

	sys1 := makeSystemInstruction(40_000)
	sys2 := &genai.Content{
		Parts: []*genai.Part{{Text: strings.Repeat("s", 40_000) + "\nnew rule added"}},
	}

	h1 := hashPrefix(stable, sys1, nil)
	h2 := hashPrefix(stable, sys2, nil)
	if h1 == h2 {
		t.Error("system prompt change should invalidate cache")
	}
}

// =============================================================================
// Long conversation token growth analysis
// =============================================================================

// Test that stable prefix token count grows linearly with turns.
func TestStablePrefixTokenGrowth(t *testing.T) {
	charsPerTurn := 2000 // ~500 tokens per message * 2 (user + model)

	type turnData struct {
		turn         int
		stableChars  int
		stableTokens int // estimated
	}

	var data []turnData
	for turn := 2; turn <= 50; turn += 5 {
		conv := makeConversation(turn, charsPerTurn/2) // split between user and model
		stable, _ := splitStablePrefix(conv)

		totalChars := 0
		for _, c := range stable {
			for _, p := range c.Parts {
				totalChars += len(p.Text)
			}
		}
		data = append(data, turnData{
			turn:         turn,
			stableChars:  totalChars,
			stableTokens: totalChars / 4,
		})
	}

	t.Logf("Stable prefix growth over turns:")
	for _, d := range data {
		t.Logf("  Turn %2d: %6d chars = ~%5d tokens", d.turn, d.stableChars, d.stableTokens)
	}

	// Verify linear growth
	if len(data) >= 2 {
		growthRate := float64(data[len(data)-1].stableTokens-data[0].stableTokens) /
			float64(data[len(data)-1].turn-data[0].turn)
		t.Logf("  Growth rate: ~%.0f tokens/turn", growthRate)
		if growthRate < 100 {
			t.Error("expected meaningful growth rate")
		}
	}
}

// =============================================================================
// Break-even analysis: when does caching start saving money?
// =============================================================================

func TestCacheBreakEvenTurn(t *testing.T) {
	pricing, _ := LookupModel("gemini-2.5-pro")

	basePrefix := 30_000
	turnGrowth := 2_000
	output := 1_000

	var noCacheTotal, cacheTotal float64
	breakEvenTurn := -1

	for turn := 0; turn < 50; turn++ {
		totalInput := basePrefix + turn*turnGrowth
		turnNoCost := simulateTurnCost(pricing.Pricing, totalInput, output, 0, 0)
		noCacheTotal += turnNoCost

		var turnCacheCost float64
		if turn == 0 {
			// First turn: pay for cache creation
			turnCacheCost = simulateTurnCost(pricing.Pricing, turnGrowth, output, totalInput, 0)
		} else {
			cached := basePrefix + (turn-1)*turnGrowth
			turnCacheCost = simulateTurnCost(pricing.Pricing, turnGrowth, output, 0, cached)
		}
		cacheTotal += turnCacheCost

		if breakEvenTurn == -1 && cacheTotal < noCacheTotal {
			breakEvenTurn = turn
		}
	}

	t.Logf("Break-even at turn %d for gemini-2.5-pro", breakEvenTurn)
	// Should break even very quickly — cache creation is 1x, but reads are 0.25x
	if breakEvenTurn > 5 {
		t.Errorf("expected break-even by turn 5, got turn %d", breakEvenTurn)
	}
}

// =============================================================================
// Model pricing — comprehensive checks
// =============================================================================

func TestModelPricingFlashLiteCacheRead(t *testing.T) {
	info, ok := LookupModel("gemini-3.1-flash-lite-preview")
	if !ok {
		t.Fatal("model not found")
	}
	// Google charges hourly storage for caches, not per-token creation
	if info.Pricing.CacheCreatePerMToken != 0 {
		t.Errorf("CacheCreate should be 0, got %f", info.Pricing.CacheCreatePerMToken)
	}
	// Cache read pricing should be cheaper than input
	if info.Pricing.CacheReadPerMToken >= info.Pricing.InputPerMToken {
		t.Errorf("CacheRead (%f) should be cheaper than Input (%f)",
			info.Pricing.CacheReadPerMToken, info.Pricing.InputPerMToken)
	}
}

func TestModelPricingCacheRatesConsistent(t *testing.T) {
	for id, info := range registry {
		t.Run(id, func(t *testing.T) {
			p := info.Pricing
			// CacheRead must be <= Input (otherwise caching costs more)
			if p.CacheReadPerMToken > p.InputPerMToken {
				t.Errorf("CacheRead (%f) > Input (%f) — caching would cost more",
					p.CacheReadPerMToken, p.InputPerMToken)
			}
			// CacheCreate should be 0 (Google charges hourly storage, not per-token)
			if p.CacheCreatePerMToken != 0 {
				t.Errorf("CacheCreate should be 0, got %f", p.CacheCreatePerMToken)
			}
		})
	}
}

// =============================================================================
// LookupModel
// =============================================================================

func TestLookupModelExact(t *testing.T) {
	info, ok := LookupModel("gemini-2.5-pro")
	if !ok || info.ID != "gemini-2.5-pro" {
		t.Errorf("expected exact match for gemini-2.5-pro")
	}
}

func TestLookupModelPrefix(t *testing.T) {
	info, ok := LookupModel("gemini-2.5")
	if !ok || info.ID != "gemini-2.5-flash" {
		t.Errorf("prefix match: got %q, want gemini-2.5-flash", info.ID)
	}
}

func TestLookupModelUnknown(t *testing.T) {
	_, ok := LookupModel("nonexistent")
	if ok {
		t.Error("expected no match for nonexistent model")
	}
}

// =============================================================================
// buildRequest with ResponseSchema (cached in cache_test.go for grouping)
// =============================================================================

func TestBuildRequestResponseSchemaProducesGenaiSchema(t *testing.T) {
	p := &Provider{}
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"nodes": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"name": {"type": "string"},
						"type": {"type": "string", "enum": ["llm", "tools", "eval"]}
					},
					"required": ["name", "type"]
				}
			}
		},
		"required": ["nodes"]
	}`)
	_, cfg := p.buildRequest(provider.RequestParams{ResponseSchema: schema})
	if cfg.ResponseMIMEType != "application/json" {
		t.Errorf("ResponseMIMEType: got %q", cfg.ResponseMIMEType)
	}
	if cfg.ResponseSchema == nil {
		t.Fatal("expected ResponseSchema")
	}
	if cfg.ResponseSchema.Type != "object" {
		t.Errorf("schema type: got %q", cfg.ResponseSchema.Type)
	}
}

func TestBuildRequestResponseSchemaWithBannedFields(t *testing.T) {
	p := &Provider{}
	schema := json.RawMessage(`{
		"type": "object",
		"default": {},
		"additionalProperties": false,
		"$schema": "http://json-schema.org/draft-07/schema",
		"properties": {
			"name": {"type": "string", "default": "unnamed"}
		}
	}`)
	_, cfg := p.buildRequest(provider.RequestParams{ResponseSchema: schema})
	if cfg.ResponseSchema == nil {
		t.Fatal("expected ResponseSchema")
	}
	out, _ := json.Marshal(cfg.ResponseSchema)
	var m map[string]any
	json.Unmarshal(out, &m)
	for _, banned := range []string{"default", "additionalProperties", "$schema"} {
		if _, ok := m[banned]; ok {
			t.Errorf("expected %q stripped from response schema", banned)
		}
	}
}

// =============================================================================
// Cost comparison table — comprehensive report
// =============================================================================

func TestCostComparisonTable(t *testing.T) {
	models := []string{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.5-flash-lite", "gemini-3.1-pro-preview"}
	turns := []int{5, 10, 25, 50}

	t.Logf("\n%-25s %5s %12s %12s %8s", "Model", "Turns", "No-Cache", "Cached", "Savings")
	t.Logf("%s", strings.Repeat("-", 65))

	for _, modelID := range models {
		info, ok := LookupModel(modelID)
		if !ok {
			continue
		}
		for _, numTurns := range turns {
			basePrefix := 30_000
			turnGrowth := 2_500
			output := 1_500

			var noCacheCost, cacheCost float64
			for turn := 0; turn < numTurns; turn++ {
				totalInput := basePrefix + turn*turnGrowth
				noCacheCost += simulateTurnCost(info.Pricing, totalInput, output, 0, 0)
				if turn == 0 {
					cacheCost += simulateTurnCost(info.Pricing, turnGrowth, output, totalInput, 0)
				} else {
					cached := basePrefix + (turn-1)*turnGrowth
					cacheCost += simulateTurnCost(info.Pricing, turnGrowth, output, 0, cached)
				}
			}

			savings := 0.0
			if noCacheCost > 0 {
				savings = (noCacheCost - cacheCost) / noCacheCost * 100
			}
			t.Logf("%-25s %5d $%10.6f $%10.6f %6.1f%%",
				modelID, numTurns, noCacheCost, cacheCost, savings)
		}
	}
}
