package tui

import (
	"strings"
	"testing"
)

func TestToolbarView(t *testing.T) {
	tb := newToolbar("claude-sonnet-4", "anthropic")

	view := tb.View(80)
	if !strings.Contains(view, "claude-sonnet-4") {
		t.Error("expected model name in toolbar")
	}
	if !strings.Contains(view, "anthropic") {
		t.Error("expected provider in toolbar")
	}
	if !strings.Contains(view, "turns: 0") {
		t.Error("expected turn count in toolbar")
	}
	if !strings.Contains(view, "ready") {
		t.Error("expected status in toolbar")
	}
}

func TestToolbarUpdates(t *testing.T) {
	tb := newToolbar("llama-3.3", "groq")

	tb.IncrementTurn()
	tb.IncrementTurn()
	tb.UpdateCost(0.0042)
	tb.SetStatus("streaming...")

	view := tb.View(100)
	if !strings.Contains(view, "turns: 2") {
		t.Error("expected turn count 2")
	}
	if !strings.Contains(view, "0.0042") {
		t.Error("expected cost in toolbar")
	}
	if !strings.Contains(view, "streaming...") {
		t.Error("expected streaming status")
	}
}
