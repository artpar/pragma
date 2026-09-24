package query

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

// CMP-001.4 F6 gates: the auto-compact token count must reflect the
// REQUEST shape — messages plus the system prompt and tool schemas the
// model request actually carries — matching the fed8bd7^ requestTokenCount
// semantics. The CMP-001..CMP-001.3 restoration counted APIMessages only,
// delaying triggers by the size of the system and tools; the precise
// provider branch was also never exercised by any test and sent an
// unshaped count request (raw conversation system, no tools).

func requestShapeWindow() compact.WindowConfig {
	// EffectiveWindow = 20000 - 4096 - 2000 = 13904; threshold = 904.
	return compact.WindowConfig{
		ContextWindow:   20_000,
		MaxOutput:       4096,
		SystemPromptEst: 2000,
	}
}

func requestShapeEngine(t *testing.T, prov provider.Provider, conv model.Conversation, cfg EngineConfig) (*Engine, *app.StateStore) {
	t.Helper()
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          t.TempDir(),
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(64), cfg)
	engine.SetCompaction(CompactionDeps{
		Compactor:    compact.NewService(prov, observe.NewEventBus(64), model.NewCostTracker(0), "test-model"),
		AutoTracker:  compact.NewAutoTracker(false),
		WindowConfig: requestShapeWindow(),
	})
	return engine, store
}

func smallSeedMessages() []model.Message {
	return []model.Message{
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hi"}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: "hello"}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "continue"}}},
	}
}

func countCompactionStarted(events []LoopEvent) int {
	var started int
	for _, ev := range events {
		if _, ok := ev.(CompactionStartedEvent); ok {
			started++
		}
	}
	return started
}

// TestProviderToolsLoopAutoCompactEstimateCountsRequestShape: with only
// the heuristic estimator (no provider TokenCounter), the trigger must
// count the request shape. The conversation's system block is carried by
// every provider-tools request (WithCustomSystemPrompt prepends the
// conversation system), so a conversation whose messages are far below
// the 904-token threshold but whose system block alone is far above it
// must still compact.
func TestProviderToolsLoopAutoCompactEstimateCountsRequestShape(t *testing.T) {
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{Content: []model.ContentPart{model.TextPart{Text: "Summary: small messages, huge system."}}, StopReason: model.StopEndTurn},
		{Content: []model.ContentPart{model.TextPart{Text: "OK, noted."}}, StopReason: model.StopEndTurn},
	}}
	bigSystem := strings.Repeat("word ", 2500) // ~12,500 bytes → ~3,125 heuristic tokens
	conv := model.NewConversation(
		model.SystemPrompt{Blocks: []model.SystemBlock{{Text: bigSystem}}},
		"test-model", "test", t.TempDir())
	conv.Messages = smallSeedMessages()
	engine, _ := requestShapeEngine(t, prov, conv, EngineConfig{
		Model:    "test-model",
		LoopMode: LoopModeProviderTools,
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "continue"))
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		}
	}
	if n := countCompactionStarted(events); n != 1 {
		t.Fatalf("CompactionStartedEvent count = %d, want 1 — the messages-only estimate ignored the %d-token system block the request actually carries (CMP-001.4 F6)", n, len(bigSystem)/4)
	}
	// The big system block is real request payload: the post-compaction
	// model request (call 2; call 1 is the summary request) must carry
	// it, proving the estimate divergence is not a fixture artifact.
	sawBigSystem := false
	for _, block := range prov.requests[1].System.Blocks {
		if strings.Contains(block.Text, strings.Repeat("word ", 10)) {
			sawBigSystem = true
		}
	}
	if !sawBigSystem {
		t.Fatalf("post-compaction model request does not carry the conversation system block the estimate must count")
	}
}

// countingProvider implements provider.TokenCounter over the scripted
// test provider, driving the loops' precise-token branch — which had
// zero test coverage before CMP-001.4 F6 — and recording the count
// request it receives.
type countingProvider struct {
	pragmaLoopTestProvider
	countCalls  int
	countParams []provider.RequestParams
}

func (p *countingProvider) CountTokens(_ context.Context, params provider.RequestParams) (int, error) {
	p.countCalls++
	p.countParams = append(p.countParams, params)
	return 6_000, nil
}

// TestProviderToolsLoopPreciseCounterCountsRequestShape: the precise
// count must be taken over the REQUEST shape. The pre-F6 code sent the
// counter the raw conversation system and no tools; provider counters
// (Google's Gemini CountTokens is a network call) count what they are
// given, so an unshaped count request systematically undercounts.
func TestProviderToolsLoopPreciseCounterCountsRequestShape(t *testing.T) {
	prov := &countingProvider{}
	prov.responses = []model.Response{
		{Content: []model.ContentPart{model.TextPart{Text: "Summary: small conversation."}}, StopReason: model.StopEndTurn},
		{Content: []model.ContentPart{model.TextPart{Text: "OK, noted."}}, StopReason: model.StopEndTurn},
	}
	conv := model.NewConversation(
		model.SystemPrompt{Blocks: []model.SystemBlock{{Text: "CONTSYS-61ad conversation system marker"}}},
		"test-model", "test", t.TempDir())
	conv.Messages = smallSeedMessages()
	engine, _ := requestShapeEngine(t, prov, conv, EngineConfig{
		Model:    "test-model",
		LoopMode: LoopModeProviderTools,
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "continue"))
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		}
	}
	// The seeded conversation is far below the 904-token heuristic
	// estimate; only the precise count (6,000) can cross the threshold.
	if n := countCompactionStarted(events); n != 1 {
		t.Fatalf("CompactionStartedEvent count = %d, want 1 — the precise TokenCounter count never reached the trigger decision (CMP-001.4 F6)", n)
	}
	if prov.countCalls < 1 {
		t.Fatalf("CountTokens was never called — the provider implements TokenCounter, so the precise branch must run")
	}
	first := prov.countParams[0]
	var sawManifest, sawConversationSystem bool
	for _, block := range first.System.Blocks {
		if strings.Contains(block.Text, "Harness manifest") {
			sawManifest = true
		}
		if strings.Contains(block.Text, "CONTSYS-61ad") {
			sawConversationSystem = true
		}
	}
	if !sawManifest || !sawConversationSystem {
		t.Fatalf("count request system blocks missing request payload: manifest=%v conversation-system=%v — the precise count must be taken over the same system the model request carries (CMP-001.4 F6)", sawManifest, sawConversationSystem)
	}
	var sawBashTool bool
	for _, tool := range first.Tools {
		if tool.Name == "Bash" {
			sawBashTool = true
		}
	}
	if !sawBashTool {
		t.Fatalf("count request carried %d tools, want the request toolset (Bash present) — tool schemas are request payload the count must include (CMP-001.4 F6)", len(first.Tools))
	}
	// Strict parity: the count request must carry the exact system and
	// toolset the model request carries (requests[1]; requests[0] is the
	// compaction summary request) — the hoisted build makes them one
	// value, and this pins that they cannot diverge again.
	if !reflect.DeepEqual(first.System, prov.requests[1].System) || !reflect.DeepEqual(first.Tools, prov.requests[1].Tools) {
		t.Fatalf("count request shape diverges from the model request: systemParity=%v toolsParity=%v (CMP-001.4 F6)",
			reflect.DeepEqual(first.System, prov.requests[1].System), reflect.DeepEqual(first.Tools, prov.requests[1].Tools))
	}
}

// TestPragmaLoopAutoCompactEstimateCountsSystemPrompt: the pragma loop's
// request system is custom prompt + PragmaLoopSystemPrompt (run.System);
// the pre-F6 estimate counted messages only, so a session whose fixed
// system overhead alone crosses the threshold never triggered.
func TestPragmaLoopAutoCompactEstimateCountsSystemPrompt(t *testing.T) {
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{Content: []model.ContentPart{model.TextPart{Text: "Summary: small messages, huge fixed system."}}, StopReason: model.StopEndTurn},
		{Content: []model.ContentPart{model.TextPart{Text: "OK, noted."}}, StopReason: model.StopEndTurn},
	}}
	customPrompt := strings.Repeat("word ", 2500) // ~3,125 heuristic tokens
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	conv.Messages = smallSeedMessages()
	engine, _ := requestShapeEngine(t, prov, conv, EngineConfig{
		Model:              "test-model",
		LoopMode:           LoopModePragma,
		CustomSystemPrompt: customPrompt,
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "continue"))
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		}
	}
	if n := countCompactionStarted(events); n != 1 {
		t.Fatalf("CompactionStartedEvent count = %d, want 1 — the messages-only estimate ignored the %d-token system prompt the pragma request carries (CMP-001.4 F6)", n, len(customPrompt)/4)
	}
}

// CMP-001.4 F6 bound gates: CountTokens is a network call on Google
// (Gemini CountTokens API), so the precise path must not run when the
// tracker's decision is count-independent. ShouldAutoCompact short-
// circuits on disabled / tripped-breaker / active-cooldown states
// BEFORE comparing the token count, so those iterations burn a network
// call for a decision already fixed at false. AutoCompactEligible is
// the pre-check the loops consult; these gates pin the skip without
// which every iteration counts.

// cooldownScript: iteration 0 counts, compacts (summary A), succeeds;
// iteration 1 is inside MinTurnsCooldown=2 and must NOT count; iteration
// 2 is eligible again, counts, and compacts (summary B); the run ends.
func cooldownScriptResponses() []model.Response {
	callInput, err := json.Marshal(map[string]string{"cmd": "true"})
	if err != nil {
		panic(err)
	}
	toolUse := func() model.Response {
		return model.Response{
			Content:    []model.ContentPart{model.ToolCallPart{ID: model.NewUUID(), Name: "Bash", Input: callInput}},
			StopReason: model.StopToolUse,
		}
	}
	return []model.Response{
		{Content: []model.ContentPart{model.TextPart{Text: "Summary A: conversation summarized."}}, StopReason: model.StopEndTurn},
		toolUse(),
		toolUse(),
		{Content: []model.ContentPart{model.TextPart{Text: "Summary B: conversation summarized again."}}, StopReason: model.StopEndTurn},
		{Content: []model.ContentPart{model.TextPart{Text: "done"}}, StopReason: model.StopEndTurn},
	}
}

func bigRequestShapeSeedMessages() []model.Message {
	bigText := strings.Repeat("word ", 3000)
	return []model.Message{
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "more questions"}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: "more answers"}}},
	}
}

func TestProviderToolsLoopPreciseCounterSkipsCooldownIterations(t *testing.T) {
	prov := &countingProvider{}
	prov.responses = cooldownScriptResponses()
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	conv.Messages = bigRequestShapeSeedMessages()
	engine, _ := requestShapeEngine(t, prov, conv, EngineConfig{
		Model:    "test-model",
		LoopMode: LoopModeProviderTools,
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "work"))
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		}
	}
	if n := countCompactionStarted(events); n != 2 {
		t.Fatalf("CompactionStartedEvent count = %d, want 2 (iteration 0 and the first post-cooldown iteration)", n)
	}
	if prov.countCalls != 2 {
		t.Fatalf("CountTokens calls = %d, want 2 — iteration 1 sits inside MinTurnsCooldown=%d, where ShouldAutoCompact is false for EVERY token count, so the Gemini network call is wasted (CMP-001.4 F6 bound)", prov.countCalls, compact.MinTurnsCooldown)
	}
	var complete int
	for _, ev := range events {
		if _, ok := ev.(TurnCompleteEvent); ok {
			complete++
		}
	}
	if complete != 1 {
		t.Fatalf("TurnCompleteEvent count = %d, want 1", complete)
	}
}

// failingSummaryCountingProvider fails every compaction summary request
// (like failingCompactProvider) while recording CountTokens calls.
type failingSummaryCountingProvider struct {
	countingProvider
	compactCalls int
}

func (p *failingSummaryCountingProvider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	if len(params.Messages) > 0 && len(params.Messages[0].Content) > 0 {
		if tp, ok := params.Messages[0].Content[0].(model.TextPart); ok &&
			strings.Contains(tp.Text, "Here is the conversation to summarize") {
			p.compactCalls++
			return model.Response{}, errors.New("summary backend unavailable")
		}
	}
	return p.countingProvider.Complete(ctx, params)
}

func breakerScriptResponses() []model.Response {
	callInput, err := json.Marshal(map[string]string{"cmd": "true"})
	if err != nil {
		panic(err)
	}
	toolUse := func() model.Response {
		return model.Response{
			Content:    []model.ContentPart{model.ToolCallPart{ID: model.NewUUID(), Name: "Bash", Input: callInput}},
			StopReason: model.StopToolUse,
		}
	}
	return []model.Response{toolUse(), toolUse(), toolUse(), {
		Content:    []model.ContentPart{model.TextPart{Text: "done"}},
		StopReason: model.StopEndTurn,
	}}
}

func TestProviderToolsLoopPreciseCounterStopsAfterBreakerTrips(t *testing.T) {
	prov := &failingSummaryCountingProvider{}
	prov.responses = breakerScriptResponses()
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	conv.Messages = bigRequestShapeSeedMessages()
	engine, _ := requestShapeEngine(t, prov, conv, EngineConfig{
		Model:    "test-model",
		LoopMode: LoopModeProviderTools,
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "work"))
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		}
	}
	var failures, disabled, complete int
	for _, ev := range events {
		switch e := ev.(type) {
		case CompactionFailedEvent:
			failures++
		case CompactionDisabledEvent:
			disabled++
		case TurnCompleteEvent:
			complete++
		case CompactionEvent:
			_ = e
			t.Fatalf("unexpected successful CompactionEvent despite failing summary backend")
		}
	}
	if failures != 3 || disabled != 1 || complete != 1 {
		t.Fatalf("failures=%d disabled=%d complete=%d, want 3/1/1 (breaker semantics unchanged)", failures, disabled, complete)
	}
	if prov.compactCalls != 3 {
		t.Fatalf("compaction attempts = %d, want 3 (breaker stops the 4th)", prov.compactCalls)
	}
	if prov.countCalls != 3 {
		t.Fatalf("CountTokens calls = %d, want 3 — once the breaker trips (consecutiveFailures >= %d) ShouldAutoCompact is false for EVERY token count for the rest of the session, so the Gemini network call is wasted on every later iteration (CMP-001.4 F6 bound)", prov.countCalls, compact.MaxConsecutiveFailures)
	}
}
