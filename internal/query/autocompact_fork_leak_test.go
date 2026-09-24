package query

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// CMP-001.4 F5 gates: ForkFreshConversation copied the parent's live
// compactor/AutoTracker/windowConfig into every child engine. The Agent
// tool nilled the tracker after forking (subagent.go), but the
// orchestration persona forks (runner.go engineForOrchestrationState)
// inherited the LIVE deps: one mutex-less AutoTracker shared across
// conversations, so a persona's compaction failures counted toward the
// root's circuit breaker and the persona's iterations advanced the
// root's cooldown (#27794: only the root engine auto-compacts).

func forkLeakTestWindow() compact.WindowConfig {
	// EffectiveWindow = 20000 - 4096 - 2000 = 13904; threshold = 904.
	return compact.WindowConfig{
		ContextWindow:   20_000,
		MaxOutput:       4096,
		SystemPromptEst: 2000,
	}
}

func TestForkFreshConversationDoesNotInheritCompactionDeps(t *testing.T) {
	prov := &pragmaLoopTestProvider{}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          t.TempDir(),
		Model:        "test-model",
		Provider:     "test",
	})
	root := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(64), EngineConfig{
		Model:    "test-model",
		LoopMode: LoopModeProviderTools,
	})
	tracker := compact.NewAutoTracker(false)
	root.SetCompaction(CompactionDeps{
		Compactor:    compact.NewService(prov, observe.NewEventBus(64), model.NewCostTracker(0), "test-model"),
		AutoTracker:  tracker,
		WindowConfig: forkLeakTestWindow(),
	})

	fork, _ := root.ForkFreshConversation()
	if fork.autoTracker != nil {
		t.Fatalf("fork inherited the root's live AutoTracker — a mutex-less tracker shared across conversations lets persona-fork failures trip the root breaker and persona turns advance the root cooldown (CMP-001.4 F5)")
	}
	if fork.compactor != nil {
		t.Fatalf("fork inherited the root's compactor (CMP-001.4 F5)")
	}
	if fork.windowConfig != (compact.WindowConfig{}) {
		t.Fatalf("fork inherited the root's windowConfig %+v (CMP-001.4 F5)", fork.windowConfig)
	}
	// The root keeps its own deps — disabling compaction on forks must
	// not affect the root engine.
	if root.autoTracker != tracker {
		t.Fatalf("root lost its own tracker")
	}
}

// TestForkCompactionFailuresDoNotTripRootBreaker runs a forked engine —
// exactly the shape orchestration persona forks and the Agent tool build
// (ForkFreshConversation + provider-tools loop) — whose every compaction
// fails. With the F5 fork the fork shared the root's AutoTracker, so the
// fork's three failures tripped the root's breaker and the root could
// never compact its own over-threshold conversation again.
func TestForkCompactionFailuresDoNotTripRootBreaker(t *testing.T) {
	callInput, err := json.Marshal(map[string]string{"cmd": "true"})
	if err != nil {
		t.Fatal(err)
	}
	toolUse := func(text string) model.Response {
		content := []model.ContentPart{model.ToolCallPart{ID: model.NewUUID(), Name: "Bash", Input: callInput}}
		if text != "" {
			content = append([]model.ContentPart{model.TextPart{Text: text}}, content...)
		}
		return model.Response{Content: content, StopReason: model.StopToolUse}
	}
	// The fork starts with a FRESH conversation: iteration 1 must grow it
	// over the 904-token threshold (huge assistant text + a tool call),
	// iterations 2-4 hit the failing compactor, iteration 5 ends.
	prov := &failingCompactProvider{}
	prov.responses = []model.Response{
		toolUse(strings.Repeat("word ", 3000)),
		toolUse(""),
		toolUse(""),
		toolUse(""),
		{Content: []model.ContentPart{model.TextPart{Text: "done"}}, StopReason: model.StopEndTurn},
	}

	bigText := strings.Repeat("word ", 3000)
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	conv.Messages = []model.Message{
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
	}
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          t.TempDir(),
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	window := forkLeakTestWindow()
	root := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(64), EngineConfig{
		Model:    "test-model",
		LoopMode: LoopModeProviderTools,
	})
	root.SetCompaction(CompactionDeps{
		Compactor:    compact.NewService(prov, observe.NewEventBus(64), model.NewCostTracker(0), "test-model"),
		AutoTracker:  compact.NewAutoTracker(false),
		WindowConfig: window,
	})

	fork, _ := root.ForkFreshConversation()
	for _, ev := range collectPragmaLoopEvents(fork.Run(t.Context(), "persona work")) {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected ErrorEvent from the fork run: %v", e.Err)
		}
	}

	if got := root.autoTracker.FailureCount(); got != 0 {
		t.Fatalf("root breaker contaminated by the fork run: %d compaction failures counted toward the root (CMP-001.4 F5)", got)
	}
	if !root.autoTracker.ShouldAutoCompact(20_000, window) {
		t.Fatalf("root auto-compaction denied after the fork's failures — the fork's compaction errors tripped the root's breaker (CMP-001.4 F5)")
	}
}
