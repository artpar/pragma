package model

import (
	"sync"
	"time"
)

// TokenUsage reports token consumption from a single LLM request.
// Cache fields are zero for providers that don't support caching.
type TokenUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// Pricing defines per-million-token costs for a model.
type Pricing struct {
	InputPerMToken       float64 `json:"input_per_m_token"`
	OutputPerMToken      float64 `json:"output_per_m_token"`
	CacheCreatePerMToken float64 `json:"cache_create_per_m_token,omitempty"`
	CacheReadPerMToken   float64 `json:"cache_read_per_m_token,omitempty"`
}

// CostEntry records the cost of a single LLM request.
type CostEntry struct {
	Timestamp time.Time  `json:"timestamp"`
	Model     string     `json:"model"`
	Provider  string     `json:"provider"`
	Usage     TokenUsage `json:"usage"`
	CostUSD   float64    `json:"cost_usd"`
}

// CostTracker accumulates costs across multiple LLM requests.
// Thread-safe via RWMutex.
type CostTracker struct {
	mu       sync.RWMutex
	entries  []CostEntry
	totalUSD float64
}

func NewCostTracker() *CostTracker {
	return &CostTracker{}
}

// Record adds a cost entry calculated from usage and pricing.
func (ct *CostTracker) Record(model string, provider string, usage TokenUsage, pricing Pricing) {
	cost := float64(usage.InputTokens)*pricing.InputPerMToken/1_000_000 +
		float64(usage.OutputTokens)*pricing.OutputPerMToken/1_000_000 +
		float64(usage.CacheCreationInputTokens)*pricing.CacheCreatePerMToken/1_000_000 +
		float64(usage.CacheReadInputTokens)*pricing.CacheReadPerMToken/1_000_000

	entry := CostEntry{
		Timestamp: time.Now(),
		Model:     model,
		Provider:  provider,
		Usage:     usage,
		CostUSD:   cost,
	}

	ct.mu.Lock()
	ct.entries = append(ct.entries, entry)
	ct.totalUSD += cost
	ct.mu.Unlock()
}

func (ct *CostTracker) TotalUSD() float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.totalUSD
}

// Snapshot returns a copy of all cost entries.
func (ct *CostTracker) Snapshot() []CostEntry {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	out := make([]CostEntry, len(ct.entries))
	copy(out, ct.entries)
	return out
}
