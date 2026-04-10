package query

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/provider"
	"github.com/artpar/gogent/internal/tool"
)

// TestToolOrchestration_MultipleToolsConcurrentAndSerial verifies the full tool path:
// - Multiple tool calls in a single response (1 concurrent + 1 serial)
// - Permission checks pass through allowAllChecker
// - Each tool is invoked with correct input
// - Results are returned in order
// - Conversation state includes tool call + result messages
func TestToolOrchestration_MultipleToolsConcurrentAndSerial(t *testing.T) {
	prov := &testProvider{
		turns: [][]provider.StreamChunk{
			// Turn 1: model calls both tools
			{
				{ToolCallStart: &model.ToolCallPart{ID: "tc-1", Name: "echo"}},
				{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: "tc-1", JSONDelta: `{"text":"first"}`}},
				{ToolCallStart: &model.ToolCallPart{ID: "tc-2", Name: "counter"}},
				{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: "tc-2", JSONDelta: `{}`}},
				{Done: &provider.StreamDone{StopReason: model.StopToolUse, Usage: model.TokenUsage{InputTokens: 100, OutputTokens: 40}}},
			},
			// Turn 2: model responds
			{
				{TextDelta: "Got results"},
				{Done: &provider.StreamDone{StopReason: model.StopEndTurn, Usage: model.TokenUsage{InputTokens: 200, OutputTokens: 20}}},
			},
		},
	}

	bus := observe.NewEventBus(256)
	eventCollect := &eventCollector{}
	bus.Subscribe(eventCollect)

	registry := tool.NewRegistry(bus)
	_ = registry.Register(echoTool{})    // concurrent
	_ = registry.Register(&counterTool{}) // serial (destructive)

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
	ct := model.NewCostTracker()

	engine := NewEngine(prov, registry, orch, store, ct, bus, EngineConfig{
		Model:     "test-model",
		MaxTokens: 4096,
	})

	events := drain(engine.Run(context.Background(), "use both tools"))

	var toolCalls []ToolCallEvent
	var toolResults []ToolResultEvent
	var complete *TurnCompleteEvent

	for _, ev := range events {
		switch e := ev.(type) {
		case ToolCallEvent:
			toolCalls = append(toolCalls, e)
		case ToolResultEvent:
			toolResults = append(toolResults, e)
		case TurnCompleteEvent:
			complete = &e
		case ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	// 2 tool calls
	if len(toolCalls) != 2 {
		t.Fatalf("tool calls = %d, want 2", len(toolCalls))
	}
	if toolCalls[0].Call.Name != "echo" {
		t.Errorf("call[0] name = %q, want echo", toolCalls[0].Call.Name)
	}
	if toolCalls[1].Call.Name != "counter" {
		t.Errorf("call[1] name = %q, want counter", toolCalls[1].Call.Name)
	}

	// 2 results in correct order
	if len(toolResults) != 2 {
		t.Fatalf("tool results = %d, want 2", len(toolResults))
	}
	if toolResults[0].Result.ToolCallID != "tc-1" {
		t.Errorf("result[0] ToolCallID = %q, want tc-1", toolResults[0].Result.ToolCallID)
	}
	if toolResults[0].Result.Content != "echo: first" {
		t.Errorf("result[0] content = %q, want %q", toolResults[0].Result.Content, "echo: first")
	}
	if toolResults[1].Result.ToolCallID != "tc-2" {
		t.Errorf("result[1] ToolCallID = %q, want tc-2", toolResults[1].Result.ToolCallID)
	}
	if toolResults[1].Result.Content != "count: 1" {
		t.Errorf("result[1] content = %q, want %q", toolResults[1].Result.Content, "count: 1")
	}

	// Verify completion
	if complete == nil {
		t.Fatal("no TurnCompleteEvent")
	}

	// Conversation: user, assistant(tool_calls), user(tool_results), assistant(text) = 4 msgs
	snap := store.Snapshot()
	if len(snap.Conversation.Messages) != 4 {
		t.Fatalf("messages = %d, want 4", len(snap.Conversation.Messages))
	}

	// Tool result message has 2 parts
	resultMsg := snap.Conversation.Messages[2]
	if len(resultMsg.Content) != 2 {
		t.Fatalf("result msg parts = %d, want 2", len(resultMsg.Content))
	}

	// Verify observe events: should have ToolCallReceived, ToolPermissionChecked,
	// ToolExecutionStarted, ToolExecutionCompleted, ToolBatchStarted, ToolBatchCompleted
	bus.Drain()

	var kinds []string
	for _, ev := range eventCollect.events {
		kinds = append(kinds, ev.EventKind())
	}

	assertContains(t, kinds, "ToolBatchStarted")
	assertContains(t, kinds, "ToolBatchCompleted")
	assertContains(t, kinds, "ToolCallReceived")
	assertContains(t, kinds, "ToolPermissionChecked")
	assertContains(t, kinds, "ToolExecutionStarted")
	assertContains(t, kinds, "ToolExecutionCompleted")
}

// TestUnknownToolInResponse verifies that an unknown tool call returns error result
// and the loop continues to the next turn.
func TestUnknownToolInResponse(t *testing.T) {
	prov := &testProvider{
		turns: [][]provider.StreamChunk{
			// Turn 1: model calls non-existent tool
			{
				{ToolCallStart: &model.ToolCallPart{ID: "tc-bad", Name: "nonexistent_tool"}},
				{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: "tc-bad", JSONDelta: `{}`}},
				{Done: &provider.StreamDone{StopReason: model.StopToolUse, Usage: model.TokenUsage{InputTokens: 50, OutputTokens: 15}}},
			},
			// Turn 2: model sees error and responds with text
			{
				{TextDelta: "Sorry, that tool doesn't exist."},
				{Done: &provider.StreamDone{StopReason: model.StopEndTurn, Usage: model.TokenUsage{InputTokens: 100, OutputTokens: 30}}},
			},
		},
	}

	engine, _ := newTestEngine(prov) // no tools registered
	events := drain(engine.Run(context.Background(), "call something"))

	var toolResults []ToolResultEvent
	var gotComplete bool

	for _, ev := range events {
		switch e := ev.(type) {
		case ToolResultEvent:
			toolResults = append(toolResults, e)
		case TurnCompleteEvent:
			gotComplete = true
		case ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	// Should get error result for unknown tool
	if len(toolResults) != 1 {
		t.Fatalf("tool results = %d, want 1", len(toolResults))
	}
	if !toolResults[0].Result.IsError {
		t.Error("expected IsError=true for unknown tool")
	}
	if toolResults[0].Result.ToolCallID != "tc-bad" {
		t.Errorf("ToolCallID = %q, want tc-bad", toolResults[0].Result.ToolCallID)
	}

	// Loop should continue to turn 2 and complete
	if !gotComplete {
		t.Error("expected TurnCompleteEvent after unknown tool error")
	}
}

// TestStreamEndsWithoutDone verifies the error path when stream closes without Done chunk.
func TestStreamEndsWithoutDone(t *testing.T) {
	prov := &testProvider{
		turns: [][]provider.StreamChunk{{
			{TextDelta: "some text"},
			// no Done chunk — stream just ends
		}},
	}
	engine, _ := newTestEngine(prov)
	events := drain(engine.Run(context.Background(), "Hi"))

	var gotError bool
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			gotError = true
			if e.Err == nil {
				t.Error("expected non-nil error")
			}
		}
	}
	if !gotError {
		t.Error("expected ErrorEvent when stream ends without Done")
	}
}

// TestToolCallInputDeltaForUnknownID verifies error when delta references unknown tool call.
func TestToolCallInputDeltaForUnknownID(t *testing.T) {
	prov := &testProvider{
		turns: [][]provider.StreamChunk{{
			{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: "ghost", JSONDelta: `{}`}},
			{Done: &provider.StreamDone{StopReason: model.StopEndTurn}},
		}},
	}
	engine, _ := newTestEngine(prov)
	events := drain(engine.Run(context.Background(), "Hi"))

	var gotError bool
	for _, ev := range events {
		if _, ok := ev.(ErrorEvent); ok {
			gotError = true
		}
	}
	if !gotError {
		t.Error("expected ErrorEvent for delta with unknown tool call ID")
	}
}

// TestMultiplePauseTurnContinuations verifies the loop handles multiple consecutive PauseTurns.
func TestMultiplePauseTurnContinuations(t *testing.T) {
	prov := &testProvider{
		turns: [][]provider.StreamChunk{
			textChunks("Part 1.", model.StopPauseTurn),
			textChunks(" Part 2.", model.StopPauseTurn),
			textChunks(" Part 3.", model.StopEndTurn),
		},
	}
	engine, _ := newTestEngine(prov)
	events := drain(engine.Run(context.Background(), "Long story"))

	var fullText string
	var completions int
	for _, ev := range events {
		switch e := ev.(type) {
		case TextEvent:
			fullText += e.Text
		case TurnCompleteEvent:
			completions++
		case ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	if fullText != "Part 1. Part 2. Part 3." {
		t.Errorf("text = %q, want %q", fullText, "Part 1. Part 2. Part 3.")
	}
	if completions != 1 {
		t.Errorf("completions = %d, want 1", completions)
	}

	// Verify conversation has: user, asst, cont_user, asst, cont_user, asst = 6 messages
	snap := engine.store.Snapshot()
	if len(snap.Conversation.Messages) != 6 {
		t.Errorf("messages = %d, want 6", len(snap.Conversation.Messages))
	}
	// Verify continuation messages are "Please continue."
	contMsg := snap.Conversation.Messages[2]
	if contMsg.Role != model.RoleUser {
		t.Errorf("cont msg role = %s, want user", contMsg.Role)
	}
	if tp, ok := contMsg.Content[0].(model.TextPart); !ok || tp.Text != "Please continue." {
		t.Errorf("cont msg text = %v, want %q", contMsg.Content[0], "Please continue.")
	}
}

// TestOnlyThinkingNoText verifies a response with only thinking (no text).
func TestOnlyThinkingNoText(t *testing.T) {
	prov := &testProvider{
		turns: [][]provider.StreamChunk{{
			{ThinkingDelta: "deep thoughts"},
			{Done: &provider.StreamDone{StopReason: model.StopEndTurn, Usage: model.TokenUsage{OutputTokens: 10}}},
		}},
	}
	engine, _ := newTestEngine(prov)
	events := drain(engine.Run(context.Background(), "Think"))

	var gotThinking bool
	var gotText bool
	for _, ev := range events {
		switch ev.(type) {
		case ThinkingEvent:
			gotThinking = true
		case TextEvent:
			gotText = true
		case ErrorEvent:
			t.Fatalf("unexpected error: %v", ev.(ErrorEvent).Err)
		}
	}
	if !gotThinking {
		t.Error("expected ThinkingEvent")
	}
	if gotText {
		t.Error("expected no TextEvent for thinking-only response")
	}
}

// counterTool is a serial (non-concurrent) tool for testing concurrent/serial partitioning.
type counterTool struct {
	count int
}

func (c *counterTool) Name() string        { return "counter" }
func (c *counterTool) Description() string { return "increments counter" }
func (c *counterTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (c *counterTool) Invoke(_ context.Context, _ json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	c.count++
	return tool.InvokeResult{Content: "count: " + string(rune('0'+c.count))}, nil
}
func (c *counterTool) CheckPerm(_ context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(context.Background(), "counter", "")
}
func (c *counterTool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: false, Concurrent: false, Destructive: true} // serial!
}

func assertContains(t *testing.T, items []string, want string) {
	t.Helper()
	for _, item := range items {
		if item == want {
			return
		}
	}
	t.Errorf("expected %q in %v", want, items)
}
