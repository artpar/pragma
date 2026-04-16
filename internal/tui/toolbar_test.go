package tui

import (
	"strings"
	"testing"
)

func TestToolbarView(t *testing.T) {
	tb := newToolbar("claude-sonnet-4", "anthropic", "pragma")

	view := tb.View(80)
	if !strings.Contains(view, "claude-sonnet-4") {
		t.Error("expected model name in toolbar")
	}
	if !strings.Contains(view, "turns: 0") {
		t.Error("expected turn count in toolbar")
	}
	if !strings.Contains(view, "ready") {
		t.Error("expected status in toolbar")
	}
}

func TestToolbarUpdates(t *testing.T) {
	tb := newToolbar("llama-3.3", "groq", "myproject")

	tb.IncrementTurn()
	tb.IncrementTurn()
	tb.UpdateCost(0.0042)
	tb.SetStatus("streaming...")

	view := tb.View(100)
	if !strings.Contains(view, "turns: 2") {
		t.Error("expected turn count 2")
	}
	if !strings.Contains(view, "streaming...") {
		t.Error("expected streaming status")
	}
}

func TestToolbarTokenDisplay(t *testing.T) {
	tb := newToolbar("test-model", "test", "workspace")
	tb.UpdateTokens(1500, 2300, 200000)

	view := tb.View(120)
	if !strings.Contains(view, "1.5K") {
		t.Errorf("expected formatted input tokens, got %q", view)
	}
	if !strings.Contains(view, "2.3K") {
		t.Errorf("expected formatted output tokens, got %q", view)
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.0K"},
		{1500, "1.5K"},
		{150000, "150.0K"},
		{1000000, "1.0M"},
		{2500000, "2.5M"},
	}
	for _, tt := range tests {
		got := formatTokens(tt.n)
		if got != tt.want {
			t.Errorf("formatTokens(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
