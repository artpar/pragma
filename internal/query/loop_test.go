package query

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/task"
	"github.com/artpar/pragma/internal/tool"
)

// testProvider is a real provider.Provider implementation backed by predetermined chunks.
// Each call to Stream() returns the next set of chunks in the turns slice.
type testProvider struct {
	turns      [][]provider.StreamChunk
	mu         sync.Mutex
	callIdx    int
	pricing    model.Pricing
	lastParams provider.RequestParams // captured from most recent Stream call
}

func (tp *testProvider) Name() string { return "test" }

func (tp *testProvider) Stream(_ context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	tp.mu.Lock()
	tp.lastParams = params
	idx := tp.callIdx
	tp.callIdx++
	tp.mu.Unlock()

	if idx >= len(tp.turns) {
		return nil, errors.New("no more turns configured in test provider")
	}

	chunks := tp.turns[idx]
	ch := make(chan provider.StreamChunk, len(chunks))
	for _, c := range chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func (tp *testProvider) Complete(_ context.Context, params provider.RequestParams) (model.Response, error) {
	ch, err := tp.Stream(context.Background(), params)
	if err != nil {
		return model.Response{}, err
	}
	return provider.AccumulateStream(ch)
}

func (tp *testProvider) SupportsFeature(_ provider.Feature) bool { return true }

func (tp *testProvider) Pricing(_ string) (model.Pricing, bool)    { return tp.pricing, true }
func (tp *testProvider) ContextWindow(_ string) (int, bool)        { return 200_000, true }

// errorProvider returns an error from Stream().
type errorProvider struct {
	err error
}

func (ep *errorProvider) Name() string                            { return "error-test" }
func (ep *errorProvider) SupportsFeature(_ provider.Feature) bool { return true }
func (ep *errorProvider) Pricing(_ string) (model.Pricing, bool)  { return model.Pricing{}, false }
func (ep *errorProvider) ContextWindow(_ string) (int, bool)      { return 200_000, true }
func (ep *errorProvider) Complete(_ context.Context, _ provider.RequestParams) (model.Response, error) {
	return model.Response{}, ep.err
}
func (ep *errorProvider) Stream(_ context.Context, _ provider.RequestParams) (<-chan provider.StreamChunk, error) {
	return nil, ep.err
}

// allowAllChecker allows all tool invocations.
type allowAllChecker struct{}

func (a *allowAllChecker) Check(_ context.Context, _ string, _ string) permission.CheckResult {
	return permission.CheckResult{
		Decision: permission.DecisionAllow,
		Rule:     &permission.Rule{ToolName: "*", Decision: permission.DecisionAllow, Source: "test"},
	}
}

func (a *allowAllChecker) AddSessionRule(_ permission.Rule) {}

// echoTool is a real tool.Descriptor that returns its input as output.
type echoTool struct{}

func (echoTool) Name() string        { return "echo" }
func (echoTool) Description() string { return "echoes input" }
func (echoTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`)
}
func (echoTool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var args struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return tool.InvokeResult{}, err
	}
	return tool.InvokeResult{Content: "echo: " + args.Text}, nil
}
func (echoTool) CheckPerm(_ context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(context.Background(), "echo", "")
}
func (echoTool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

// newTestEngine creates an Engine wired for testing.
func newTestEngine(prov provider.Provider, tools ...tool.Descriptor) (*Engine, *model.CostTracker) {
	bus := observe.NewEventBus(256)
	registry := tool.NewRegistry(bus)
	for _, t := range tools {
		_ = registry.Register(t)
	}
	checker := &allowAllChecker{}
	prompter := &permission.NonInteractivePrompter{}
	orch := tool.NewOrchestrator(registry, checker, prompter, bus)

	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", "/tmp/test")
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          "/tmp/test",
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	ct := model.NewCostTracker(0)

	engine := NewEngine(prov, registry, orch, store, ct, bus, EngineConfig{
		Model:     "test-model",
		MaxTokens: 4096,
	})
	return engine, ct
}

// drain reads all events from a LoopEvent channel into a slice.
func drain(ch <-chan LoopEvent) []LoopEvent {
	var events []LoopEvent
	for ev := range ch {
		events = append(events, ev)
	}
	return events
}

func textChunks(text string, stopReason model.StopReason) []provider.StreamChunk {
	return []provider.StreamChunk{
		{TextDelta: text},
		{Done: &provider.StreamDone{StopReason: stopReason, Usage: model.TokenUsage{InputTokens: 100, OutputTokens: 50}}},
	}
}

func TestRun_SimpleTextResponse(t *testing.T) {
	prov := &testProvider{
		turns:   [][]provider.StreamChunk{textChunks("Hello, world!", model.StopEndTurn)},
		pricing: model.Pricing{InputPerMToken: 3, OutputPerMToken: 15},
	}
	engine, _ := newTestEngine(prov)
	events := drain(engine.Run(context.Background(), "Hi"))

	var gotText string
	var gotComplete bool
	for _, ev := range events {
		switch e := ev.(type) {
		case TextEvent:
			gotText += e.Text
		case TurnCompleteEvent:
			gotComplete = true
			if e.StopReason != model.StopEndTurn {
				t.Errorf("StopReason = %s, want %s", e.StopReason, model.StopEndTurn)
			}
		case ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	if gotText != "Hello, world!" {
		t.Errorf("text = %q, want %q", gotText, "Hello, world!")
	}
	if !gotComplete {
		t.Error("expected TurnCompleteEvent")
	}
}

func TestRun_WithThinking(t *testing.T) {
	prov := &testProvider{
		turns: [][]provider.StreamChunk{{
			{ThinkingDelta: "Let me think..."},
			{TextDelta: "Here is my answer."},
			{Done: &provider.StreamDone{StopReason: model.StopEndTurn, Usage: model.TokenUsage{InputTokens: 50, OutputTokens: 30}}},
		}},
	}
	engine, _ := newTestEngine(prov)
	events := drain(engine.Run(context.Background(), "Think about this"))

	var gotThinking, gotText string
	for _, ev := range events {
		switch e := ev.(type) {
		case ThinkingEvent:
			gotThinking += e.Text
		case TextEvent:
			gotText += e.Text
		case ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	if gotThinking != "Let me think..." {
		t.Errorf("thinking = %q, want %q", gotThinking, "Let me think...")
	}
	if gotText != "Here is my answer." {
		t.Errorf("text = %q, want %q", gotText, "Here is my answer.")
	}
}

func TestRun_ToolUseLoop(t *testing.T) {
	toolCallID := "tc-001"
	prov := &testProvider{
		turns: [][]provider.StreamChunk{
			// Turn 1: model calls echo tool
			{
				{ToolCallStart: &model.ToolCallPart{ID: toolCallID, Name: "echo"}},
				{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: toolCallID, JSONDelta: `{"text":"hello"}`}},
				{Done: &provider.StreamDone{StopReason: model.StopToolUse, Usage: model.TokenUsage{InputTokens: 80, OutputTokens: 20}}},
			},
			// Turn 2: model responds with text
			textChunks("Tool said: echo: hello", model.StopEndTurn),
		},
	}
	engine, _ := newTestEngine(prov, echoTool{})
	events := drain(engine.Run(context.Background(), "Call echo"))

	var gotToolCall, gotToolResult, gotText bool
	for _, ev := range events {
		switch e := ev.(type) {
		case ToolCallEvent:
			gotToolCall = true
			if e.Call.Name != "echo" {
				t.Errorf("tool call name = %q, want %q", e.Call.Name, "echo")
			}
		case ToolResultEvent:
			gotToolResult = true
			if e.Result.Content != "echo: hello" {
				t.Errorf("tool result = %q, want %q", e.Result.Content, "echo: hello")
			}
			if e.Result.IsError {
				t.Error("tool result IsError = true, want false")
			}
		case TextEvent:
			gotText = true
		case TurnCompleteEvent:
			if e.StopReason != model.StopEndTurn {
				t.Errorf("StopReason = %s, want %s", e.StopReason, model.StopEndTurn)
			}
		case ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	if !gotToolCall {
		t.Error("expected ToolCallEvent")
	}
	if !gotToolResult {
		t.Error("expected ToolResultEvent")
	}
	if !gotText {
		t.Error("expected TextEvent in second turn")
	}
}

func TestRun_PauseTurnContinuation(t *testing.T) {
	prov := &testProvider{
		turns: [][]provider.StreamChunk{
			// Turn 1: pause_turn
			textChunks("Partial response...", model.StopPauseTurn),
			// Turn 2: completes
			textChunks(" and the rest.", model.StopEndTurn),
		},
	}
	engine, _ := newTestEngine(prov)
	events := drain(engine.Run(context.Background(), "Tell me a long story"))

	var gotText string
	var completions int
	for _, ev := range events {
		switch e := ev.(type) {
		case TextEvent:
			gotText += e.Text
		case TurnCompleteEvent:
			completions++
		case ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	if gotText != "Partial response... and the rest." {
		t.Errorf("text = %q, want %q", gotText, "Partial response... and the rest.")
	}
	if completions != 1 {
		t.Errorf("completions = %d, want 1", completions)
	}
}

func TestRun_MaxTokens(t *testing.T) {
	prov := &testProvider{
		turns: [][]provider.StreamChunk{textChunks("Truncated", model.StopMaxTokens)},
	}
	engine, _ := newTestEngine(prov)
	events := drain(engine.Run(context.Background(), "Hi"))

	for _, ev := range events {
		switch e := ev.(type) {
		case TurnCompleteEvent:
			if e.StopReason != model.StopMaxTokens {
				t.Errorf("StopReason = %s, want %s", e.StopReason, model.StopMaxTokens)
			}
		case ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}
}

func TestRun_StreamError(t *testing.T) {
	streamErr := errors.New("stream broke")
	prov := &testProvider{
		turns: [][]provider.StreamChunk{{
			{TextDelta: "start"},
			{Error: streamErr},
		}},
	}
	engine, _ := newTestEngine(prov)
	events := drain(engine.Run(context.Background(), "Hi"))

	var gotError bool
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			gotError = true
			if !errors.Is(e.Err, streamErr) {
				t.Errorf("error = %v, want %v", e.Err, streamErr)
			}
		}
	}
	if !gotError {
		t.Error("expected ErrorEvent")
	}
}

func TestRun_ProviderStreamError(t *testing.T) {
	provErr := errors.New("provider unavailable")
	prov := &errorProvider{err: provErr}
	engine, _ := newTestEngine(prov)
	events := drain(engine.Run(context.Background(), "Hi"))

	var gotError bool
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			gotError = true
			if !errors.Is(e.Err, provErr) {
				t.Errorf("error = %v, want %v", e.Err, provErr)
			}
		}
	}
	if !gotError {
		t.Error("expected ErrorEvent")
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	prov := &testProvider{
		turns: [][]provider.StreamChunk{textChunks("should not see", model.StopEndTurn)},
	}
	engine, _ := newTestEngine(prov)
	events := drain(engine.Run(ctx, "Hi"))

	var gotError bool
	for _, ev := range events {
		if _, ok := ev.(ErrorEvent); ok {
			gotError = true
		}
	}
	if !gotError {
		t.Error("expected ErrorEvent for cancelled context")
	}
}

func TestRun_CostTracking(t *testing.T) {
	prov := &testProvider{
		turns:   [][]provider.StreamChunk{textChunks("Hello", model.StopEndTurn)},
		pricing: model.Pricing{InputPerMToken: 3, OutputPerMToken: 15},
	}
	engine, ct := newTestEngine(prov)
	events := drain(engine.Run(context.Background(), "Hi"))

	// Ensure no errors
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	total := ct.TotalUSD()
	if total <= 0 {
		t.Errorf("cost = %f, want > 0", total)
	}

	entries := ct.Snapshot()
	if len(entries) != 1 {
		t.Errorf("entries = %d, want 1", len(entries))
	}
	if entries[0].Model != "test-model" {
		t.Errorf("model = %q, want %q", entries[0].Model, "test-model")
	}
	if entries[0].Provider != "test" {
		t.Errorf("provider = %q, want %q", entries[0].Provider, "test")
	}
}

func TestRun_PauseTurnExceedsTurns(t *testing.T) {
	// Provider that always returns StopPauseTurn — should be stopped by maxTurns.
	maxTurns := 5
	turns := make([][]provider.StreamChunk, maxTurns+10)
	for i := range turns {
		turns[i] = textChunks("chunk", model.StopPauseTurn)
	}

	prov := &testProvider{turns: turns}

	bus := observe.NewEventBus(256)
	registry := tool.NewRegistry(bus)
	checker := &allowAllChecker{}
	prompter := &permission.NonInteractivePrompter{}
	orch := tool.NewOrchestrator(registry, checker, prompter, bus)

	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", "/tmp/test")
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          "/tmp/test",
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	ct := model.NewCostTracker(0)

	engine := NewEngine(prov, registry, orch, store, ct, bus, EngineConfig{
		Model:     "test-model",
		MaxTokens: 4096,
		MaxTurns:  maxTurns,
	})
	events := drain(engine.Run(context.Background(), "pause forever"))

	var gotError bool
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			gotError = true
			if e.Err == nil {
				t.Fatal("ErrorEvent has nil Err")
			}
			t.Logf("got expected error: %v", e.Err)
		}
	}
	if !gotError {
		t.Fatal("expected ErrorEvent for maxTurns exceeded, but loop completed without error")
	}

	// Verify provider was called at most maxTurns times (not maxTurns+10)
	prov.mu.Lock()
	calls := prov.callIdx
	prov.mu.Unlock()
	if calls > maxTurns {
		t.Errorf("provider called %d times, want at most %d (maxTurns)", calls, maxTurns)
	}
}

func TestRun_TurnBudgetWarning(t *testing.T) {
	// Verify that the engine injects a warning when reaching 80% of maxTurns.
	// With maxTurns=10, warningThreshold = 10/5 = 2.
	// Warning fires when remaining == 2, i.e., when turnCount==7 (before increment to 8).
	// That means the 8th tool-call turn's result includes the warning.
	maxTurns := 10

	toolCallID := "tc-echo"
	toolCallTurns := make([][]provider.StreamChunk, 0)
	// 8 tool-call turns (each tool call increments turnCount by 1)
	for i := 0; i < 8; i++ {
		toolCallTurns = append(toolCallTurns, []provider.StreamChunk{
			{ToolCallStart: &model.ToolCallPart{ID: toolCallID, Name: "echo"}},
			{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: toolCallID, JSONDelta: `{"text":"ping"}`}},
			{Done: &provider.StreamDone{StopReason: model.StopToolUse, Usage: model.TokenUsage{InputTokens: 50, OutputTokens: 10}}},
		})
	}
	// Final turn: model outputs text and stops
	toolCallTurns = append(toolCallTurns, textChunks("Done!", model.StopEndTurn))

	prov := &testProvider{turns: toolCallTurns}

	bus := observe.NewEventBus(256)
	registry := tool.NewRegistry(bus)
	_ = registry.Register(echoTool{})
	checker := &allowAllChecker{}
	prompter := &permission.NonInteractivePrompter{}
	orch := tool.NewOrchestrator(registry, checker, prompter, bus)

	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", "/tmp/test")
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          "/tmp/test",
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	ct := model.NewCostTracker(0)

	engine := NewEngine(prov, registry, orch, store, ct, bus, EngineConfig{
		Model:     "test-model",
		MaxTokens: 4096,
		MaxTurns:  maxTurns,
	})

	events := drain(engine.Run(context.Background(), "Run echo 8 times"))

	// Check no errors
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	// Inspect conversation messages for the turn budget warning
	snap := engine.store.Snapshot()
	msgs := snap.Conversation.APIMessages()

	var foundWarning bool
	for _, m := range msgs {
		for _, part := range m.Content {
			if tp, ok := part.(model.TextPart); ok {
				if strings.Contains(tp.Text, "turns remaining") {
					foundWarning = true
					if !strings.Contains(tp.Text, "2 turns remaining") {
						t.Errorf("warning text = %q, expected '2 turns remaining'", tp.Text)
					}
				}
			}
		}
	}
	if !foundWarning {
		t.Error("expected turn budget warning at 80% usage (turn 8 of 10), but not found in conversation")
	}
}

func TestRun_ModelFromAppState(t *testing.T) {
	// Verify that when AppState.Model is set, the engine uses it instead of config.Model
	prov := &testProvider{
		turns: [][]provider.StreamChunk{
			textChunks("hello", model.StopEndTurn),
		},
	}

	bus := observe.NewEventBus(256)
	registry := tool.NewRegistry(bus)
	checker := &allowAllChecker{}
	prompter := &permission.NonInteractivePrompter{}
	orch := tool.NewOrchestrator(registry, checker, prompter, bus)

	conv := model.NewConversation(model.SystemPrompt{}, "config-model", "test", "/tmp/test")
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          "/tmp/test",
		Model:        "override-model", // AppState override
		Provider:     "test",
		MaxTokens:    4096,
	})
	ct := model.NewCostTracker(0)

	engine := NewEngine(prov, registry, orch, store, ct, bus, EngineConfig{
		Model:     "config-model",
		MaxTokens: 4096,
	})

	drain(engine.Run(context.Background(), "Hi"))

	prov.mu.Lock()
	gotModel := prov.lastParams.Model
	prov.mu.Unlock()

	if gotModel != "override-model" {
		t.Errorf("provider received model %q, want %q (from AppState)", gotModel, "override-model")
	}
}

func TestRun_ModelFallsBackToConfig(t *testing.T) {
	// When AppState.Model is empty, engine should use config.Model
	prov := &testProvider{
		turns: [][]provider.StreamChunk{
			textChunks("hello", model.StopEndTurn),
		},
	}

	bus := observe.NewEventBus(256)
	registry := tool.NewRegistry(bus)
	checker := &allowAllChecker{}
	prompter := &permission.NonInteractivePrompter{}
	orch := tool.NewOrchestrator(registry, checker, prompter, bus)

	conv := model.NewConversation(model.SystemPrompt{}, "config-model", "test", "/tmp/test")
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          "/tmp/test",
		Model:        "", // Empty — should fall back
		Provider:     "test",
		MaxTokens:    4096,
	})
	ct := model.NewCostTracker(0)

	engine := NewEngine(prov, registry, orch, store, ct, bus, EngineConfig{
		Model:     "config-model",
		MaxTokens: 4096,
	})

	drain(engine.Run(context.Background(), "Hi"))

	prov.mu.Lock()
	gotModel := prov.lastParams.Model
	prov.mu.Unlock()

	if gotModel != "config-model" {
		t.Errorf("provider received model %q, want %q (from config fallback)", gotModel, "config-model")
	}
}

func TestRunLoop_PendingMessages(t *testing.T) {
	// Pre-load a pending message into the task registry before running the engine.
	// The engine should drain it at the top of the first turn and inject it as a
	// user message with the "Messages from teammates:" prefix.
	taskBus := observe.NewEventBus(16)
	taskReg := task.NewRegistry(taskBus)
	tk := taskReg.Create("test-teammate", "integration test")
	_ = taskReg.Update(tk.ID, func(tt *task.Task) {
		tt.Status = task.TaskRunning
		tt.AgentName = "test-teammate"
		tt.PendingMessages = append(tt.PendingMessages, "Hello from another agent")
	})

	prov := &testProvider{
		turns: [][]provider.StreamChunk{textChunks("Got it!", model.StopEndTurn)},
	}
	engine, _ := newTestEngine(prov)
	engine.SetTaskRegistry(taskReg)
	engine.SetTaskID(tk.ID)

	events := drain(engine.Run(context.Background(), "Start"))

	// Verify the engine completed without errors.
	var gotComplete bool
	for _, ev := range events {
		switch ev.(type) {
		case TurnCompleteEvent:
			gotComplete = true
		case ErrorEvent:
			t.Fatalf("unexpected error: %v", ev.(ErrorEvent).Err)
		}
	}
	if !gotComplete {
		t.Fatal("expected TurnCompleteEvent")
	}

	// Verify the injected message appears in the conversation.
	snap := engine.store.Snapshot()
	msgs := snap.Conversation.APIMessages()
	var found bool
	for _, m := range msgs {
		for _, part := range m.Content {
			if tp, ok := part.(model.TextPart); ok {
				if strings.Contains(tp.Text, "Messages from teammates:") &&
					strings.Contains(tp.Text, "Hello from another agent") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("expected injected teammate message in conversation, but not found")
	}

	// Verify messages were drained (no pending messages left).
	drained := taskReg.DrainPendingMessages(tk.ID)
	if len(drained) != 0 {
		t.Errorf("expected 0 pending messages after drain, got %d", len(drained))
	}
}

// TestRun_AutoCompactionTriggersAndReplaces verifies auto-compaction fires when
// the token count heuristic exceeds the configured threshold and that conversation
// messages are replaced with the compaction summary.
func TestRun_AutoCompactionTriggersAndReplaces(t *testing.T) {
	// Turn 1: main conversation response
	// Turn 2: compaction call — the compactor calls provider.Complete which uses Stream
	prov := &testProvider{
		turns: [][]provider.StreamChunk{
			// Turn 1: normal response
			textChunks("OK, noted.", model.StopEndTurn),
			// Turn 2: compaction summary (compact.Service calls provider.Complete → Stream)
			textChunks("Summary: user asked questions, assistant answered.", model.StopEndTurn),
		},
		pricing: model.Pricing{InputPerMToken: 3, OutputPerMToken: 15},
	}

	bus := observe.NewEventBus(256)
	registry := tool.NewRegistry(bus)
	checker := &allowAllChecker{}
	prompter := &permission.NonInteractivePrompter{}
	orch := tool.NewOrchestrator(registry, checker, prompter, bus)

	// Create conversation with enough existing messages to exceed threshold
	system := model.SystemPrompt{
		Blocks: []model.SystemBlock{{Text: "You are helpful.", Cacheable: true}},
	}
	conv := model.NewConversation(system, "test-model", "test", "/tmp/test")

	// Add 6 existing messages (~6000 tokens via heuristic estimator, 4 chars ≈ 1 token)
	bigText := strings.Repeat("word ", 3000) // ~15000 chars ≈ 3750 tokens
	conv.Messages = []model.Message{
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "more questions"}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: "more answers"}}},
	}

	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          "/tmp/test",
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	ct := model.NewCostTracker(0)

	engine := NewEngine(prov, registry, orch, store, ct, bus, EngineConfig{
		Model:     "test-model",
		MaxTokens: 4096,
	})

	// Configure auto-compaction with a very small window so threshold is easily exceeded.
	// EffectiveWindow = 20000 - 4096 - 2000 = 13904
	// AutoCompactThreshold = 13904 - 13000 = 904 tokens
	// With ~15000 tokens of conversation, this will trigger immediately.
	compactSvc := compact.NewService(prov, bus, ct, "test-model")
	tracker := compact.NewAutoTracker(false)
	engine.SetCompaction(CompactionDeps{
		Compactor:   compactSvc,
		AutoTracker: tracker,
		WindowConfig: compact.WindowConfig{
			ContextWindow:   20_000,
			MaxOutput:       4096,
			SystemPromptEst: 2000,
		},
	})

	events := drain(engine.Run(context.Background(), "Tell me more"))

	// Verify event sequence includes CompactionStartedEvent and CompactionEvent
	var gotStarted, gotCompacted bool
	var preTokens, postTokens int
	var gotComplete bool

	for _, ev := range events {
		switch e := ev.(type) {
		case CompactionStartedEvent:
			gotStarted = true
		case CompactionEvent:
			gotCompacted = true
			preTokens = e.PreTokens
			postTokens = e.PostTokens
		case TurnCompleteEvent:
			gotComplete = true
		case ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	if !gotStarted {
		t.Error("expected CompactionStartedEvent")
	}
	if !gotCompacted {
		t.Error("expected CompactionEvent")
	}
	if gotCompacted && preTokens <= postTokens {
		t.Errorf("compaction should reduce tokens: pre=%d, post=%d", preTokens, postTokens)
	}
	if !gotComplete {
		t.Error("expected TurnCompleteEvent after compaction")
	}

	// Verify conversation state was replaced
	snap := store.Snapshot()
	msgs := snap.Conversation.Messages
	// After compaction: compact.Service replaces ALL messages with a summary.
	// The original 6 messages + 2 new = 8, then compacted down to summary.
	if len(msgs) >= 8 {
		t.Errorf("compaction should have reduced message count from 8, got %d", len(msgs))
	}
	if len(msgs) == 0 {
		t.Fatal("compaction should leave at least 1 summary message")
	}
}

// TestRun_AutoCompactionCircuitBreaker verifies that 3 consecutive compaction failures
// disable auto-compaction and emit CompactionDisabledEvent.
func TestRun_AutoCompactionCircuitBreaker(t *testing.T) {
	// We need 3 turns, each triggering compaction that fails.
	// The compaction fails because compact.Service will get an error from the provider
	// on the compaction call.
	// Turn flow: main response (tool_use → end_turn) for 3 turns + failed compaction calls.
	// Actually simpler: use a provider that works for main turns but the compaction
	// service uses the same provider. We need the compaction call to fail.
	// The simplest way: make the compaction content empty so ErrEmptySummary fires.

	// Build 3 main turns + 3 compaction turns (each returns empty → triggers ErrEmptySummary)
	var turns [][]provider.StreamChunk
	for i := 0; i < 3; i++ {
		// Main turn response (tool_use to keep looping)
		turns = append(turns, textChunks("Response "+string(rune('A'+i)), model.StopEndTurn))
		// Compaction turn — empty text triggers ErrEmptySummary
		turns = append(turns, []provider.StreamChunk{
			{TextDelta: ""},
			{Done: &provider.StreamDone{StopReason: model.StopEndTurn, Usage: model.TokenUsage{InputTokens: 10, OutputTokens: 5}}},
		})
	}

	prov := &testProvider{
		turns:   turns,
		pricing: model.Pricing{InputPerMToken: 3, OutputPerMToken: 15},
	}

	bus := observe.NewEventBus(256)
	registry := tool.NewRegistry(bus)
	checker := &allowAllChecker{}
	prompter := &permission.NonInteractivePrompter{}
	orch := tool.NewOrchestrator(registry, checker, prompter, bus)

	system := model.SystemPrompt{
		Blocks: []model.SystemBlock{{Text: "Be helpful.", Cacheable: true}},
	}
	conv := model.NewConversation(system, "test-model", "test", "/tmp/test")

	// Pre-fill with large messages to trigger compaction
	bigText := strings.Repeat("text ", 3000)
	conv.Messages = []model.Message{
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
		{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: bigText}}},
	}

	store := app.NewStateStore(app.AppState{Conversation: conv, CWD: "/tmp/test", Model: "test-model", Provider: "test", MaxTokens: 4096})
	ct := model.NewCostTracker(0)

	engine := NewEngine(prov, registry, orch, store, ct, bus, EngineConfig{
		Model:     "test-model",
		MaxTokens: 4096,
		MaxTurns:  3,
	})

	compactSvc := compact.NewService(prov, bus, ct, "test-model")
	tracker := compact.NewAutoTracker(false)
	engine.SetCompaction(CompactionDeps{
		Compactor:   compactSvc,
		AutoTracker: tracker,
		WindowConfig: compact.WindowConfig{
			ContextWindow:   20_000,
			MaxOutput:       4096,
			SystemPromptEst: 2000,
		},
	})

	// The engine processes first turn → StopEndTurn → exits.
	// Only 1 compaction attempt per run. Need tool_use to keep going.
	// Actually, StopEndTurn exits the loop, so only 1 turn executes.
	// To test circuit breaker, we need 3 separate runs.
	events1 := drain(engine.Run(context.Background(), "Turn 1"))
	events2 := drain(engine.Run(context.Background(), "Turn 2"))
	events3 := drain(engine.Run(context.Background(), "Turn 3"))

	allEvents := append(append(events1, events2...), events3...)

	var failCount, disabledCount int
	for _, ev := range allEvents {
		switch ev.(type) {
		case CompactionFailedEvent:
			failCount++
		case CompactionDisabledEvent:
			disabledCount++
		}
	}

	if failCount != 3 {
		t.Errorf("expected 3 CompactionFailedEvents, got %d", failCount)
	}
	if disabledCount != 1 {
		t.Errorf("expected 1 CompactionDisabledEvent, got %d", disabledCount)
	}
}
