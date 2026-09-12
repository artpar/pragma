package query

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

// TURN-003 gates: a provider-tools loop with no explicit MaxTurns runs
// past the former default of 100 turns instead of dying at the cap (the
// operator directive after TURN-001/002's cap deaths); sub-agents keep a
// runaway bound when the parent is uncapped; explicit bounds are
// unchanged (covered by the turn_budget suite).

func turnCapRemovalResponses(t *testing.T, toolTurns int) []model.Response {
	t.Helper()
	cmd, err := json.Marshal(map[string]string{"cmd": "true"})
	if err != nil {
		t.Fatal(err)
	}
	responses := make([]model.Response, 0, toolTurns+1)
	for i := 0; i < toolTurns; i++ {
		responses = append(responses, model.Response{
			Content: []model.ContentPart{
				model.ToolCallPart{ID: model.NewUUID(), Name: "Bash", Input: cmd},
			},
			StopReason: model.StopToolUse,
		})
	}
	responses = append(responses, model.Response{
		Content:   []model.ContentPart{model.TextPart{Text: "done"}},
		StopReason: model.StopEndTurn,
	})
	return responses
}

// newUncappedProviderToolsEngine builds a provider-tools engine with
// MaxTurns unset — the path the default-cap semantics govern. The shared
// helper pins MaxTurns 5, which would gate the explicit-bound path
// instead of the default one this case changes.
func newUncappedProviderToolsEngine(t *testing.T, prov provider.Provider) *Engine {
	t.Helper()
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          t.TempDir(),
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	return NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(64), EngineConfig{
		Model:     "test-model",
		LoopMode:  LoopModeProviderTools,
		MaxTokens: 4096,
	})
}

func TestProviderToolsLoopNoDefaultTurnCap(t *testing.T) {
	// MaxTurns unset: the loop must run past 100 tool turns and complete
	// on the model's end-turn — the baseline capped at 100.
	prov := &pragmaLoopTestProvider{responses: turnCapRemovalResponses(t, 105)}
	engine := newUncappedProviderToolsEngine(t, prov)

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "work"))

	for _, ev := range events {
		if errEv, isErr := ev.(ErrorEvent); isErr {
			if strings.Contains(errEv.Err.Error(), "exceeded maximum") {
				t.Fatalf("loop died on the turn cap with no explicit MaxTurns: %v", errEv.Err)
			}
			t.Fatalf("unexpected error event: %v", errEv.Err)
		}
	}
	if prov.calls != 106 {
		t.Fatalf("provider calls = %d, want 106 (105 tool turns + final end-turn)", prov.calls)
	}
	var completed bool
	for _, ev := range events {
		if _, ok := ev.(TurnCompleteEvent); ok {
			completed = true
		}
	}
	if !completed {
		t.Fatal("loop never emitted TurnCompleteEvent")
	}
}

func TestForkPinsSubAgentTurnCapWhenParentUncapped(t *testing.T) {
	// Uncapped parent → sub keeps the runaway bound; bounded parent →
	// sub inherits the explicit bound.
	engine := newUncappedProviderToolsEngine(t, &pragmaLoopTestProvider{})
	sub, _ := engine.ForkFreshConversation()
	want := 100 // the former default; DefaultSubAgentMaxTurns on the candidate
	if sub.config.MaxTurns != want {
		t.Fatalf("uncapped parent's sub MaxTurns = %d, want %d", sub.config.MaxTurns, want)
	}

	bounded := newUncappedProviderToolsEngine(t, &pragmaLoopTestProvider{})
	bounded.config.MaxTurns = 50
	subBounded, _ := bounded.ForkFreshConversation()
	if subBounded.config.MaxTurns != 50 {
		t.Fatalf("bounded parent's sub MaxTurns = %d, want inherited 50", subBounded.config.MaxTurns)
	}
}
