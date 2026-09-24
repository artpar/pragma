package query

import (
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// BUILD-budget-cap gates: a positive MaxCostUSD must hard-stop the loop
// with a distinct spend-ceiling error before any further provider call,
// in BOTH loop modes; zero (default) must leave behavior unchanged.

func budgetCapEngine(t *testing.T, loopMode string, maxCost float64, initialSpend float64) (*Engine, *pragmaLoopTestProvider) {
	t.Helper()
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{Content: []model.ContentPart{model.TextPart{Text: "hello"}}, StopReason: model.StopEndTurn},
	}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{Conversation: conv, CWD: t.TempDir(), Model: "test-model", Provider: "test", MaxTokens: 4096})
	engine := NewEngine(prov, store, model.NewCostTracker(initialSpend), observe.NewEventBus(64), EngineConfig{
		Model:      "test-model",
		LoopMode:   loopMode,
		MaxTokens:  4096,
		MaxCostUSD: maxCost,
	})
	return engine, prov
}

func TestProviderToolsLoopSpendCeiling(t *testing.T) {
	engine, prov := budgetCapEngine(t, LoopModeProviderTools, 0.50, 1.00)
	events := collectPragmaLoopEvents(engine.Run(t.Context(), "work"))
	var capErr string
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			capErr = e.Err.Error()
		}
	}
	if !strings.Contains(capErr, "spend ceiling") {
		t.Fatalf("loop error = %q, want spend ceiling error", capErr)
	}
	if prov.calls != 0 {
		t.Fatalf("provider calls = %d, want 0 (ceiling must abort before any request)", prov.calls)
	}
}

func TestProviderToolsLoopSpendCeilingDisabledByDefault(t *testing.T) {
	engine, prov := budgetCapEngine(t, LoopModeProviderTools, 0, 1.00)
	events := collectPragmaLoopEvents(engine.Run(t.Context(), "work"))
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected error with cap disabled: %v", e.Err)
		}
	}
	if prov.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", prov.calls)
	}
}

func TestPragmaLoopSpendCeiling(t *testing.T) {
	engine, prov := budgetCapEngine(t, LoopModePragma, 0.50, 1.00)
	events := collectPragmaLoopEvents(engine.Run(t.Context(), "work"))
	var capErr string
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			capErr = e.Err.Error()
		}
	}
	if !strings.Contains(capErr, "spend ceiling") {
		t.Fatalf("loop error = %q, want spend ceiling error", capErr)
	}
	if prov.calls != 0 {
		t.Fatalf("provider calls = %d, want 0 (ceiling must abort before any request)", prov.calls)
	}
}
