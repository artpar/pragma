package model

import (
	"math"
	"sync"
	"testing"
)

func TestCostTrackerRecord(t *testing.T) {
	ct := NewCostTracker(0)
	usage := TokenUsage{InputTokens: 1000, OutputTokens: 500}
	pricing := Pricing{InputPerMToken: 3.0, OutputPerMToken: 15.0}

	ct.Record("claude-sonnet-4-20250514", "anthropic", usage, pricing)

	expectedCost := 1000*3.0/1_000_000 + 500*15.0/1_000_000
	got := ct.TotalUSD()
	if math.Abs(got-expectedCost) > 1e-12 {
		t.Errorf("TotalUSD: got %v, want %v", got, expectedCost)
	}

	entries := ct.Snapshot()
	if len(entries) != 1 {
		t.Fatalf("Snapshot: got %d entries, want 1", len(entries))
	}
	if entries[0].Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model: got %q", entries[0].Model)
	}
}

func TestCostTrackerWithCacheTokens(t *testing.T) {
	ct := NewCostTracker(0)
	usage := TokenUsage{
		InputTokens:              1000,
		OutputTokens:             500,
		CacheCreationInputTokens: 200,
		CacheReadInputTokens:     300,
	}
	pricing := Pricing{
		InputPerMToken:       3.0,
		OutputPerMToken:      15.0,
		CacheCreatePerMToken: 3.75,
		CacheReadPerMToken:   0.30,
	}

	ct.Record("model", "prov", usage, pricing)

	expected := 1000*3.0/1_000_000 +
		500*15.0/1_000_000 +
		200*3.75/1_000_000 +
		300*0.30/1_000_000

	got := ct.TotalUSD()
	if math.Abs(got-expected) > 1e-12 {
		t.Errorf("TotalUSD with cache: got %v, want %v", got, expected)
	}
}

func TestCostTrackerConcurrent(t *testing.T) {
	ct := NewCostTracker(0)
	usage := TokenUsage{InputTokens: 100, OutputTokens: 50}
	pricing := Pricing{InputPerMToken: 1.0, OutputPerMToken: 1.0}

	var wg sync.WaitGroup
	n := 100
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			ct.Record("model", "prov", usage, pricing)
		}()
	}
	wg.Wait()

	entries := ct.Snapshot()
	if len(entries) != n {
		t.Errorf("Snapshot: got %d entries, want %d", len(entries), n)
	}

	singleCost := 100*1.0/1_000_000 + 50*1.0/1_000_000
	expectedTotal := float64(n) * singleCost
	got := ct.TotalUSD()
	if math.Abs(got-expectedTotal) > 1e-10 {
		t.Errorf("TotalUSD: got %v, want %v", got, expectedTotal)
	}
}

func TestCostTrackerSnapshotIsCopy(t *testing.T) {
	ct := NewCostTracker(0)
	usage := TokenUsage{InputTokens: 100, OutputTokens: 50}
	pricing := Pricing{InputPerMToken: 1.0, OutputPerMToken: 1.0}

	ct.Record("model", "prov", usage, pricing)
	snap := ct.Snapshot()

	// Modify the snapshot — should not affect the tracker
	snap[0].CostUSD = 999.0

	entries := ct.Snapshot()
	if entries[0].CostUSD == 999.0 {
		t.Error("Snapshot returned a reference, not a copy")
	}
}
