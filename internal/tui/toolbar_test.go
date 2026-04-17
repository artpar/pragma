package tui

import (
	"strings"
	"testing"
	"time"
)

func TestToolbarView(t *testing.T) {
	tb := newToolbar("claude-sonnet-4", "anthropic", "/home/user/pragma")

	view := tb.View(80)
	if !strings.Contains(view, "claude-sonnet-4") {
		t.Error("expected model name in toolbar")
	}
	if !strings.Contains(view, "anthropic") {
		t.Error("expected provider in toolbar")
	}
	if !strings.Contains(view, "·") {
		t.Error("expected middle dot separator")
	}
	if !strings.Contains(view, "ready") {
		t.Error("expected status in toolbar")
	}
	// Workspace should be basename
	if strings.Contains(view, "/home/user") {
		t.Error("should show basename not full path")
	}
}

func TestToolbarUpdates(t *testing.T) {
	tb := newToolbar("llama-3.3", "groq", "/tmp/myproject")

	tb.UpdateCost(0.0042)
	tb.SetStatus("streaming...")

	view := tb.View(100)
	if !strings.Contains(view, "$0.0042") {
		t.Errorf("expected cost $0.0042, got %q", view)
	}
	if !strings.Contains(view, "streaming...") {
		t.Error("expected streaming status")
	}
}

func TestToolbarTokenDisplay(t *testing.T) {
	tb := newToolbar("test-model", "test", "/workspace")
	tb.UpdateTokens(1500, 2300, 0, 200000)

	view := tb.View(120)
	if !strings.Contains(view, "1.5k in") {
		t.Errorf("expected '1.5k in' in token display, got %q", view)
	}
	if !strings.Contains(view, "2.3k out") {
		t.Errorf("expected '2.3k out' in token display, got %q", view)
	}
	// No cache segment when cache is 0
	if strings.Contains(view, "cache") {
		t.Error("should not show cache when cacheTokens is 0")
	}
}

func TestToolbarCacheTokens(t *testing.T) {
	tb := newToolbar("test-model", "test", "/workspace")
	tb.UpdateTokens(1500, 2300, 500, 200000)

	view := tb.View(120)
	if !strings.Contains(view, "500 cache") {
		t.Errorf("expected '500 cache' in token display, got %q", view)
	}
}

func TestToolbarContextPctExcludesOutput(t *testing.T) {
	tb := newToolbar("test-model", "test", "/workspace")
	// input=5000, output=5000, cache=0, ctx=100000
	// Context pct should be (5000+0)/100000 = 5%, NOT (5000+5000)/100000 = 10%
	tb.UpdateTokens(5000, 5000, 0, 100000)

	view := tb.View(120)
	if !strings.Contains(view, "5% ctx") {
		t.Errorf("expected '5%% ctx' (excludes output), got %q", view)
	}

	// With cache: (5000+2000)/100000 = 7%
	tb.UpdateTokens(5000, 5000, 2000, 100000)
	view = tb.View(120)
	if !strings.Contains(view, "7% ctx") {
		t.Errorf("expected '7%% ctx' (input+cache), got %q", view)
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1k"},
		{1500, "1.5k"},
		{2000, "2k"},
		{150000, "150k"},
		{1000000, "1m"},
		{2500000, "2.5m"},
	}
	for _, tt := range tests {
		got := formatTokens(tt.n)
		if got != tt.want {
			t.Errorf("formatTokens(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{30 * time.Second, "30s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m"},
		{90 * time.Second, "1m 30s"},
		{5 * time.Minute, "5m"},
		{60 * time.Minute, "1h 0m 0s"},
		{90 * time.Minute, "1h 30m 0s"},
		{125*time.Minute + 15*time.Second, "2h 5m 15s"},
		{25 * time.Hour, "1d 1h 0m"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		cost float64
		want string
	}{
		{0, "$0.0000"},
		{0.0042, "$0.0042"},
		{0.50, "$0.5000"},
		{0.51, "$0.51"},
		{1.234, "$1.23"},
		{10.0, "$10.00"},
	}
	for _, tt := range tests {
		got := formatCost(tt.cost)
		if got != tt.want {
			t.Errorf("formatCost(%v) = %q, want %q", tt.cost, got, tt.want)
		}
	}
}

func TestToolbarCostSummary(t *testing.T) {
	tb := newToolbar("test", "test", "/workspace")
	tb.UpdateTokens(1500, 2300, 500, 200000)
	tb.UpdateCost(0.0042)

	summary := tb.CostSummary()
	if !strings.Contains(summary, "$0.0042") {
		t.Errorf("expected cost in summary, got %q", summary)
	}
	if !strings.Contains(summary, "1.5k in") {
		t.Errorf("expected input tokens in summary, got %q", summary)
	}
	if !strings.Contains(summary, "500 cache") {
		t.Errorf("expected cache tokens in summary, got %q", summary)
	}
}
