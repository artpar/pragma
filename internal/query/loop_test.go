package query

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/provider"
	"github.com/artpar/gogent/internal/tool"
)

// testProvider is a real provider.Provider implementation backed by predetermined chunks.
// Each call to Stream() returns the next set of chunks in the turns slice.
type testProvider struct {
	turns   [][]provider.StreamChunk
	mu      sync.Mutex
	callIdx int
	pricing model.Pricing
}

func (tp *testProvider) Name() string { return "test" }

func (tp *testProvider) Stream(_ context.Context, _ provider.RequestParams) (<-chan provider.StreamChunk, error) {
	tp.mu.Lock()
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

func (tp *testProvider) Pricing(_ string) model.Pricing { return tp.pricing }

// errorProvider returns an error from Stream().
type errorProvider struct {
	err error
}

func (ep *errorProvider) Name() string                            { return "error-test" }
func (ep *errorProvider) SupportsFeature(_ provider.Feature) bool { return true }
func (ep *errorProvider) Pricing(_ string) model.Pricing          { return model.Pricing{} }
func (ep *errorProvider) Complete(_ context.Context, _ provider.RequestParams) (model.Response, error) {
	return model.Response{}, ep.err
}
func (ep *errorProvider) Stream(_ context.Context, _ provider.RequestParams) (<-chan provider.StreamChunk, error) {
	return nil, ep.err
}

// allowAllChecker allows all tool invocations.
type allowAllChecker struct{}

func (a *allowAllChecker) Check(_ context.Context, _ string, _ json.RawMessage) permission.CheckResult {
	return permission.CheckResult{
		Decision: permission.DecisionAllow,
		Rule:     permission.Rule{Pattern: "*", Decision: permission.DecisionAllow, Source: "test"},
	}
}

// echoTool is a real tool.Descriptor that returns its input as output.
type echoTool struct{}

func (echoTool) Name() string        { return "echo" }
func (echoTool) Description() string { return "echoes input" }
func (echoTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`)
}
func (echoTool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (string, error) {
	var args struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	return "echo: " + args.Text, nil
}
func (echoTool) CheckPerm(_ context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(context.Background(), "echo", nil)
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
	orch := tool.NewOrchestrator(registry, checker, bus)

	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", "/tmp/test")
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          "/tmp/test",
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	ct := model.NewCostTracker()

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
