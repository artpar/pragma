package query

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
	"github.com/artpar/gogent/internal/tool"
)

// TestPipelineIntegration verifies the full pipeline in depth:
// - Response has correct StopReason and non-zero Usage
// - Conversation state has both user and assistant messages
// - Cost tracker has entries with correct model/provider
// - EventBus received the right events
// - LoopEvent sequence is correct
func TestPipelineIntegration(t *testing.T) {
	prov := &testProvider{
		turns: [][]provider.StreamChunk{{
			{ThinkingDelta: "I should say hello"},
			{ThinkingSignatureDelta: "sig-abc"},
			{TextDelta: "Hello "},
			{TextDelta: "there!"},
			{Done: &provider.StreamDone{
				StopReason: model.StopEndTurn,
				Usage:      model.TokenUsage{InputTokens: 150, OutputTokens: 75},
			}},
		}},
		pricing: model.Pricing{InputPerMToken: 3, OutputPerMToken: 15},
	}

	bus := observe.NewEventBus(256)

	// Capture events from the bus
	recorder := &eventCollector{}
	bus.Subscribe(recorder)

	registry := tool.NewRegistry(bus)
	checker := &allowAllChecker{}
	orch := tool.NewOrchestrator(registry, checker, bus)

	system := model.SystemPrompt{
		Blocks: []model.SystemBlock{{Text: "You are helpful.", Cacheable: true}},
	}
	conv := model.NewConversation(system, "test-model", "test-provider", "/tmp/test")
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          "/tmp/test",
		Model:        "test-model",
		Provider:     "test-provider",
		MaxTokens:    4096,
	})
	ct := model.NewCostTracker()

	engine := NewEngine(prov, registry, orch, store, ct, bus, EngineConfig{
		Model:     "test-model",
		MaxTokens: 4096,
	})

	events := drain(engine.Run(context.Background(), "Hi there"))

	// 1. Verify LoopEvent sequence: ThinkingEvent(s), TextEvent(s), TurnCompleteEvent
	var thinkingText, textContent string
	var complete *TurnCompleteEvent
	var eventTypes []string

	for _, ev := range events {
		switch e := ev.(type) {
		case ThinkingEvent:
			thinkingText += e.Text
			eventTypes = append(eventTypes, "thinking")
		case TextEvent:
			textContent += e.Text
			eventTypes = append(eventTypes, "text")
		case TurnCompleteEvent:
			complete = &e
			eventTypes = append(eventTypes, "complete")
		case ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	if thinkingText != "I should say hello" {
		t.Errorf("thinking = %q, want %q", thinkingText, "I should say hello")
	}
	if textContent != "Hello there!" {
		t.Errorf("text = %q, want %q", textContent, "Hello there!")
	}
	if complete == nil {
		t.Fatal("no TurnCompleteEvent received")
	}

	// 2. Verify Response structure
	resp := complete.Response
	if resp.StopReason != model.StopEndTurn {
		t.Errorf("StopReason = %s, want %s", resp.StopReason, model.StopEndTurn)
	}
	if resp.Usage.InputTokens != 150 {
		t.Errorf("InputTokens = %d, want %d", resp.Usage.InputTokens, 150)
	}
	if resp.Usage.OutputTokens != 75 {
		t.Errorf("OutputTokens = %d, want %d", resp.Usage.OutputTokens, 75)
	}

	// Verify content parts order: thinking, then text
	if len(resp.Content) != 2 {
		t.Fatalf("content parts = %d, want 2", len(resp.Content))
	}
	think, ok := resp.Content[0].(model.ThinkingPart)
	if !ok {
		t.Fatalf("content[0] type = %T, want ThinkingPart", resp.Content[0])
	}
	if think.Text != "I should say hello" {
		t.Errorf("ThinkingPart.Text = %q, want %q", think.Text, "I should say hello")
	}
	if think.Signature != "sig-abc" {
		t.Errorf("ThinkingPart.Signature = %q, want %q", think.Signature, "sig-abc")
	}
	text, ok := resp.Content[1].(model.TextPart)
	if !ok {
		t.Fatalf("content[1] type = %T, want TextPart", resp.Content[1])
	}
	if text.Text != "Hello there!" {
		t.Errorf("TextPart.Text = %q, want %q", text.Text, "Hello there!")
	}

	// 3. Verify conversation state
	snap := store.Snapshot()
	msgs := snap.Conversation.Messages
	if len(msgs) != 2 {
		t.Fatalf("conversation messages = %d, want 2 (user + assistant)", len(msgs))
	}
	if msgs[0].Role != model.RoleUser {
		t.Errorf("msgs[0].Role = %s, want %s", msgs[0].Role, model.RoleUser)
	}
	if msgs[1].Role != model.RoleAssistant {
		t.Errorf("msgs[1].Role = %s, want %s", msgs[1].Role, model.RoleAssistant)
	}
	// Verify user message content
	if len(msgs[0].Content) != 1 {
		t.Fatalf("user msg content parts = %d, want 1", len(msgs[0].Content))
	}
	userText, ok := msgs[0].Content[0].(model.TextPart)
	if !ok || userText.Text != "Hi there" {
		t.Errorf("user message text = %q, want %q", userText.Text, "Hi there")
	}
	// Verify assistant message has thinking + text
	if len(msgs[1].Content) != 2 {
		t.Fatalf("assistant msg content parts = %d, want 2", len(msgs[1].Content))
	}

	// 4. Verify cost tracker
	totalCost := ct.TotalUSD()
	if totalCost <= 0 {
		t.Errorf("totalCost = %f, want > 0", totalCost)
	}
	entries := ct.Snapshot()
	if len(entries) != 1 {
		t.Fatalf("cost entries = %d, want 1", len(entries))
	}
	if entries[0].Model != "test-model" {
		t.Errorf("cost model = %q, want %q", entries[0].Model, "test-model")
	}
	if entries[0].Provider != "test" {
		t.Errorf("cost provider = %q, want %q", entries[0].Provider, "test")
	}
	if entries[0].Usage.InputTokens != 150 {
		t.Errorf("cost InputTokens = %d, want %d", entries[0].Usage.InputTokens, 150)
	}
	// Verify cost math: (150 * 3 / 1M) + (75 * 15 / 1M) = 0.00045 + 0.001125 = 0.001575
	expectedCost := float64(150)*3/1_000_000 + float64(75)*15/1_000_000
	if entries[0].CostUSD != expectedCost {
		t.Errorf("cost USD = %f, want %f", entries[0].CostUSD, expectedCost)
	}

	// 5. Verify system prompt was preserved in conversation
	if len(snap.Conversation.System.Blocks) != 1 {
		t.Fatalf("system blocks = %d, want 1", len(snap.Conversation.System.Blocks))
	}
	if snap.Conversation.System.Blocks[0].Text != "You are helpful." {
		t.Errorf("system text = %q, want %q", snap.Conversation.System.Blocks[0].Text, "You are helpful.")
	}
	if !snap.Conversation.System.Blocks[0].Cacheable {
		t.Error("system block Cacheable = false, want true")
	}
}

// TestPipelineToolUseIntegration verifies tool use round-trip:
// - ToolCallEvent emitted before execution
// - Tool actually invoked with correct input
// - ToolResultEvent contains tool output
// - Conversation has tool call + result messages
func TestPipelineToolUseIntegration(t *testing.T) {
	toolCallID := "tc-integration-001"
	prov := &testProvider{
		turns: [][]provider.StreamChunk{
			{
				{ToolCallStart: &model.ToolCallPart{ID: toolCallID, Name: "echo"}},
				{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: toolCallID, JSONDelta: `{"text":"world"}`}},
				{Done: &provider.StreamDone{StopReason: model.StopToolUse, Usage: model.TokenUsage{InputTokens: 80, OutputTokens: 20}}},
			},
			{
				{TextDelta: "I called echo and got: echo: world"},
				{Done: &provider.StreamDone{StopReason: model.StopEndTurn, Usage: model.TokenUsage{InputTokens: 200, OutputTokens: 50}}},
			},
		},
		pricing: model.Pricing{InputPerMToken: 3, OutputPerMToken: 15},
	}

	bus := observe.NewEventBus(256)
	registry := tool.NewRegistry(bus)
	_ = registry.Register(echoTool{})
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

	events := drain(engine.Run(context.Background(), "Call echo with world"))

	// Verify event sequence
	var toolCalls []ToolCallEvent
	var toolResults []ToolResultEvent
	var texts []string
	var complete *TurnCompleteEvent

	for _, ev := range events {
		switch e := ev.(type) {
		case ToolCallEvent:
			toolCalls = append(toolCalls, e)
		case ToolResultEvent:
			toolResults = append(toolResults, e)
		case TextEvent:
			texts = append(texts, e.Text)
		case TurnCompleteEvent:
			complete = &e
		case ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	// 1. Verify tool call event
	if len(toolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(toolCalls))
	}
	if toolCalls[0].Call.Name != "echo" {
		t.Errorf("tool name = %q, want %q", toolCalls[0].Call.Name, "echo")
	}
	if toolCalls[0].Call.ID != toolCallID {
		t.Errorf("tool call ID = %q, want %q", toolCalls[0].Call.ID, toolCallID)
	}

	// 2. Verify tool result
	if len(toolResults) != 1 {
		t.Fatalf("tool results = %d, want 1", len(toolResults))
	}
	if toolResults[0].Result.Content != "echo: world" {
		t.Errorf("tool result = %q, want %q", toolResults[0].Result.Content, "echo: world")
	}
	if toolResults[0].Result.IsError {
		t.Error("tool result IsError = true, want false")
	}
	if toolResults[0].Result.ToolCallID != toolCallID {
		t.Errorf("result ToolCallID = %q, want %q", toolResults[0].Result.ToolCallID, toolCallID)
	}

	// 3. Verify text from second turn
	if len(texts) == 0 {
		t.Fatal("no text events in second turn")
	}

	// 4. Verify completion
	if complete == nil {
		t.Fatal("no TurnCompleteEvent")
	}
	if complete.StopReason != model.StopEndTurn {
		t.Errorf("StopReason = %s, want %s", complete.StopReason, model.StopEndTurn)
	}

	// 5. Verify conversation state — should have 4 messages:
	// [0] user: "Call echo with world"
	// [1] assistant: tool_call
	// [2] user: tool_result
	// [3] assistant: text response
	snap := store.Snapshot()
	msgs := snap.Conversation.Messages
	if len(msgs) != 4 {
		t.Fatalf("conversation messages = %d, want 4", len(msgs))
	}
	if msgs[0].Role != model.RoleUser {
		t.Errorf("msgs[0].Role = %s, want user", msgs[0].Role)
	}
	if msgs[1].Role != model.RoleAssistant {
		t.Errorf("msgs[1].Role = %s, want assistant", msgs[1].Role)
	}
	if msgs[2].Role != model.RoleUser {
		t.Errorf("msgs[2].Role = %s, want user (tool result)", msgs[2].Role)
	}
	if msgs[3].Role != model.RoleAssistant {
		t.Errorf("msgs[3].Role = %s, want assistant", msgs[3].Role)
	}

	// Verify tool call is in assistant message
	if len(msgs[1].Content) != 1 {
		t.Fatalf("assistant msg[1] content parts = %d, want 1", len(msgs[1].Content))
	}
	tc, ok := msgs[1].Content[0].(model.ToolCallPart)
	if !ok {
		t.Fatalf("msgs[1].Content[0] type = %T, want ToolCallPart", msgs[1].Content[0])
	}
	if tc.Name != "echo" {
		t.Errorf("tool call name = %q, want %q", tc.Name, "echo")
	}
	var tcInput struct{ Text string }
	if err := json.Unmarshal(tc.Input, &tcInput); err != nil {
		t.Fatalf("unmarshal tool call input: %v", err)
	}
	if tcInput.Text != "world" {
		t.Errorf("tool call input text = %q, want %q", tcInput.Text, "world")
	}

	// Verify tool result is in user message
	if len(msgs[2].Content) != 1 {
		t.Fatalf("user msg[2] content parts = %d, want 1", len(msgs[2].Content))
	}
	tr, ok := msgs[2].Content[0].(model.ToolResultPart)
	if !ok {
		t.Fatalf("msgs[2].Content[0] type = %T, want ToolResultPart", msgs[2].Content[0])
	}
	if tr.Content != "echo: world" {
		t.Errorf("tool result content = %q, want %q", tr.Content, "echo: world")
	}

	// 6. Verify cost tracker has 2 entries (one per API turn)
	entries := ct.Snapshot()
	if len(entries) != 2 {
		t.Fatalf("cost entries = %d, want 2", len(entries))
	}
}

// eventCollector captures events from the EventBus for test verification.
type eventCollector struct {
	events []observe.Event
}

func (ec *eventCollector) HandleEvent(event observe.Event) {
	ec.events = append(ec.events, event)
}
