package bridge_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/artpar/gogent/internal/lifecycle"
	"github.com/artpar/gogent/internal/lifecycle/bridge"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	gtesting "github.com/artpar/gogent/internal/testing"
	"github.com/artpar/gogent/internal/tool"
)

func newTestBus() *observe.EventBus {
	return observe.NewEventBus(64)
}

// testTool is a simple tool for bridge tests.
type testTool struct {
	name    string
	handler func(json.RawMessage) (string, error)
}

func (t *testTool) Name() string                  { return t.name }
func (t *testTool) Description() string            { return "test tool: " + t.name }
func (t *testTool) InputSchema() json.RawMessage   { return json.RawMessage(`{"type":"object","properties":{}}`) }
func (t *testTool) Flags() tool.ToolFlags          { return tool.ToolFlags{Concurrent: true} }
func (t *testTool) CheckPerm(_ context.Context, _ json.RawMessage, _ permission.Checker) permission.CheckResult {
	return permission.CheckResult{Decision: permission.DecisionAllow}
}
func (t *testTool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	result, err := t.handler(input)
	return tool.InvokeResult{Content: result}, err
}

func TestMessageReducer(t *testing.T) {
	a := []model.Message{{ID: "1", Role: model.RoleUser}}
	b := []model.Message{{ID: "2", Role: model.RoleAssistant}}

	result := bridge.MessageReducer(a, b).([]model.Message)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].ID != "1" || result[1].ID != "2" {
		t.Fatalf("wrong order: %s, %s", result[0].ID, result[1].ID)
	}

	// Verify original slices not mutated
	if len(a) != 1 || len(b) != 1 {
		t.Fatal("original slices were mutated")
	}
}

func TestMessageReducer_TypeMismatch(t *testing.T) {
	// Existing is not []model.Message — should return incoming
	result := bridge.MessageReducer("not-messages", []model.Message{{ID: "1"}})
	msgs, ok := result.([]model.Message)
	if !ok || len(msgs) != 1 {
		t.Fatal("expected incoming to be returned on type mismatch")
	}
}

func TestReflectionReducer(t *testing.T) {
	a := []string{"first"}
	b := []string{"second"}
	result := bridge.ReflectionReducer(a, b).([]string)
	if len(result) != 2 || result[0] != "first" || result[1] != "second" {
		t.Fatalf("unexpected result: %v", result)
	}
}

func TestStateAccessors(t *testing.T) {
	state := lifecycle.State{
		bridge.KeyMessages:  []model.Message{{ID: "m1"}},
		bridge.KeyModelID:   "test-model",
		bridge.KeyMaxTokens: 1024,
		bridge.KeyPassed:    true,
		bridge.KeyScore:     0.95,
	}

	msgs := bridge.Messages(state)
	if len(msgs) != 1 || msgs[0].ID != "m1" {
		t.Fatal("Messages accessor failed")
	}
	if bridge.ModelID(state) != "test-model" {
		t.Fatal("ModelID accessor failed")
	}
	if bridge.MaxTokens(state) != 1024 {
		t.Fatal("MaxTokens accessor failed")
	}
	if !bridge.Passed(state) {
		t.Fatal("Passed accessor failed")
	}
	if bridge.Score(state) != 0.95 {
		t.Fatal("Score accessor failed")
	}

	// Missing keys return zero values
	empty := lifecycle.State{}
	if bridge.Messages(empty) != nil {
		t.Fatal("expected nil for missing messages")
	}
	if bridge.ModelID(empty) != "" {
		t.Fatal("expected empty for missing model_id")
	}
}

func TestLLMNode_Basic(t *testing.T) {
	resp := model.Response{
		ID:         "resp-1",
		Model:      "test-model",
		Content:    []model.ContentPart{model.TextPart{Text: "Hello world"}},
		StopReason: model.StopEndTurn,
		Usage:      model.TokenUsage{InputTokens: 10, OutputTokens: 5},
	}
	prov := gtesting.NewSequenceProvider(resp)
	bus := newTestBus()

	node := bridge.LLMNode(prov, bus, bridge.LLMNodeConfig{})

	state := lifecycle.State{
		bridge.KeyMessages:  []model.Message{{ID: "user-1", Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "Hi"}}}},
		bridge.KeySystem:    model.SystemPrompt{Blocks: []model.SystemBlock{{Text: "You are helpful."}}},
		bridge.KeyModelID:   "test-model",
		bridge.KeyMaxTokens: 1024,
	}

	update, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check stop_reason
	stopReason, ok := update[bridge.KeyStopReason].(string)
	if !ok || stopReason != "end_turn" {
		t.Fatalf("expected stop_reason=end_turn, got %v", update[bridge.KeyStopReason])
	}

	// Check messages contains assistant message
	newMsgs, ok := update[bridge.KeyMessages].([]model.Message)
	if !ok || len(newMsgs) != 1 {
		t.Fatalf("expected 1 new message, got %v", update[bridge.KeyMessages])
	}
	if newMsgs[0].Role != model.RoleAssistant {
		t.Fatal("expected assistant role")
	}

	// Check turn_count
	turnCount, ok := update[bridge.KeyTurnCount].(int)
	if !ok || turnCount != 1 {
		t.Fatalf("expected turn_count=1, got %v", update[bridge.KeyTurnCount])
	}

	// Verify provider received correct params
	calls := prov.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 provider call, got %d", len(calls))
	}
	if calls[0].Model != "test-model" {
		t.Fatalf("expected model=test-model, got %s", calls[0].Model)
	}
}

func TestLLMNode_ToolUse(t *testing.T) {
	resp := model.Response{
		Content: []model.ContentPart{
			model.ToolCallPart{ID: "tc-1", Name: "Bash", Input: json.RawMessage(`{"command":"ls"}`)},
		},
		StopReason: model.StopToolUse,
	}
	prov := gtesting.NewSequenceProvider(resp)

	node := bridge.LLMNode(prov, newTestBus(), bridge.LLMNodeConfig{})
	state := lifecycle.State{
		bridge.KeyMessages:  []model.Message{},
		bridge.KeyModelID:   "test",
		bridge.KeyMaxTokens: 1024,
	}

	update, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stopReason := update[bridge.KeyStopReason].(string)
	if stopReason != "tool_use" {
		t.Fatalf("expected stop_reason=tool_use, got %s", stopReason)
	}
}

func TestLLMNode_NodePrompt(t *testing.T) {
	resp := model.Response{
		Content:    []model.ContentPart{model.TextPart{Text: "ok"}},
		StopReason: model.StopEndTurn,
	}
	prov := gtesting.NewSequenceProvider(resp)

	node := bridge.LLMNode(prov, newTestBus(), bridge.LLMNodeConfig{
		NodePrompt: "Custom system prompt",
	})
	state := lifecycle.State{
		bridge.KeyMessages:  []model.Message{},
		bridge.KeySystem:    model.SystemPrompt{Blocks: []model.SystemBlock{{Text: "Original"}}},
		bridge.KeyModelID:   "test",
		bridge.KeyMaxTokens: 1024,
	}

	_, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := prov.Calls()
	if len(calls[0].System.Blocks) != 2 {
		t.Fatalf("expected 2 system blocks (node prompt + original), got %d", len(calls[0].System.Blocks))
	}
	if calls[0].System.Blocks[0].Text != "Custom system prompt" {
		t.Fatalf("expected node prompt as first block, got %q", calls[0].System.Blocks[0].Text)
	}
	if calls[0].System.Blocks[1].Text != "Original" {
		t.Fatalf("expected original system prompt preserved as second block, got %q", calls[0].System.Blocks[1].Text)
	}
}

func TestToolNode_Execute(t *testing.T) {
	bus := newTestBus()
	reg := tool.NewRegistry(bus)
	_ = reg.Register(&testTool{
		name: "Echo",
		handler: func(input json.RawMessage) (string, error) {
			return "echoed", nil
		},
	})

	checker := permission.NewRuleChecker(nil, permission.ModeBypassPermissions, "/tmp", bus)
	orch := tool.NewOrchestrator(reg, checker, &permission.NonInteractivePrompter{}, bus)

	node := bridge.ToolNode(orch, "/tmp")

	// State with assistant message containing a tool call
	state := lifecycle.State{
		bridge.KeyMessages: []model.Message{
			{
				ID:   "user-1",
				Role: model.RoleUser,
				Content: []model.ContentPart{
					model.TextPart{Text: "echo something"},
				},
			},
			{
				ID:   "assistant-1",
				Role: model.RoleAssistant,
				Content: []model.ContentPart{
					model.ToolCallPart{ID: "tc-1", Name: "Echo", Input: json.RawMessage(`{}`)},
				},
			},
		},
	}

	update, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	newMsgs, ok := update[bridge.KeyMessages].([]model.Message)
	if !ok || len(newMsgs) != 1 {
		t.Fatalf("expected 1 result message, got %v", update[bridge.KeyMessages])
	}
	if newMsgs[0].Role != model.RoleUser {
		t.Fatal("expected user role for tool result message")
	}

	// Check content contains tool result
	found := false
	for _, part := range newMsgs[0].Content {
		if tr, ok := part.(model.ToolResultPart); ok {
			if tr.ToolCallID == "tc-1" && tr.Content == "echoed" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected tool result with content 'echoed'")
	}
}

func TestToolNode_NoToolCalls(t *testing.T) {
	bus := newTestBus()
	reg := tool.NewRegistry(bus)
	checker := permission.NewRuleChecker(nil, permission.ModeBypassPermissions, "/tmp", bus)
	orch := tool.NewOrchestrator(reg, checker, &permission.NonInteractivePrompter{}, bus)

	node := bridge.ToolNode(orch, "/tmp")

	// Last message is assistant but has no tool calls
	state := lifecycle.State{
		bridge.KeyMessages: []model.Message{
			{ID: "a-1", Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: "done"}}},
		},
	}

	update, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if update != nil {
		t.Fatalf("expected nil update for no tool calls, got %v", update)
	}
}

func TestStopReasonRouter(t *testing.T) {
	router := bridge.StopReasonRouter()

	tests := []struct {
		reason string
		want   string
	}{
		{"tool_use", "continue"},
		{"end_turn", "end"},
		{"max_tokens", "end"},
		{"", "end"},
	}

	for _, tt := range tests {
		state := lifecycle.State{bridge.KeyStopReason: tt.reason}
		got := router(state)
		if got != tt.want {
			t.Errorf("StopReasonRouter(%q) = %q, want %q", tt.reason, got, tt.want)
		}
	}
}

func TestFieldRouter(t *testing.T) {
	router := bridge.FieldRouter("done")

	tests := []struct {
		val  any
		want string
	}{
		{true, "true"},
		{false, "false"},
		{"custom", "custom"},
		{nil, ""},
	}

	for _, tt := range tests {
		state := lifecycle.State{"done": tt.val}
		got := router(state)
		if got != tt.want {
			t.Errorf("FieldRouter(done=%v) = %q, want %q", tt.val, got, tt.want)
		}
	}
}

func TestPassFailRouter(t *testing.T) {
	router := bridge.PassFailRouter()

	if got := router(lifecycle.State{bridge.KeyPassed: true}); got != "pass" {
		t.Errorf("expected pass, got %q", got)
	}
	if got := router(lifecycle.State{bridge.KeyPassed: false}); got != "fail" {
		t.Errorf("expected fail, got %q", got)
	}
}

func TestReActPattern_FullLoop(t *testing.T) {
	bus := newTestBus()
	reg := tool.NewRegistry(bus)
	_ = reg.Register(&testTool{
		name: "Bash",
		handler: func(_ json.RawMessage) (string, error) {
			return "file1.go\nfile2.go", nil
		},
	})

	checker := permission.NewRuleChecker(nil, permission.ModeBypassPermissions, "/tmp", bus)
	orch := tool.NewOrchestrator(reg, checker, &permission.NonInteractivePrompter{}, bus)

	// Turn 1: LLM returns tool call
	// Turn 2: LLM returns final text
	prov := gtesting.NewSequenceProvider(
		model.Response{
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "tc-1", Name: "Bash", Input: json.RawMessage(`{"command":"ls"}`)},
			},
			StopReason: model.StopToolUse,
		},
		model.Response{
			Content:    []model.ContentPart{model.TextPart{Text: "Found 2 files"}},
			StopReason: model.StopEndTurn,
		},
	)

	// Build ReAct graph directly with bridge node constructors (no hardcoded patterns).
	graph, buildErr := lifecycle.NewBuilder().
		AddNode("llm", bridge.LLMNode(prov, bus, bridge.LLMNodeConfig{})).
		AddNode("tools", bridge.ToolNode(orch, "/tmp")).
		SetInitialNode("llm").
		SetReducer(bridge.KeyMessages, bridge.MessageReducer).
		SetReducer(bridge.KeyTurnCount, lifecycle.ReducerSum).
		SetReducer(bridge.KeyTotalUsage, bridge.UsageReducer).
		AddConditionalEdges("llm", bridge.StopReasonRouter(), map[string]string{
			"continue": "tools",
			"end":      "",
		}).
		AddEdge("tools", "llm").
		Build()
	if buildErr != nil {
		t.Fatalf("graph build: %v", buildErr)
	}
	executor := lifecycle.NewExecutor(graph, lifecycle.WithEventBus(bus))

	initialState := lifecycle.State{
		bridge.KeyMessages: []model.Message{
			{ID: "user-1", Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "List files"}}},
		},
		bridge.KeyModelID:   "test",
		bridge.KeyMaxTokens: 1024,
		bridge.KeySystem:    model.SystemPrompt{Blocks: []model.SystemBlock{{Text: "You are helpful."}}},
	}

	finalState, err := executor.Run(context.Background(), initialState)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have: user msg + assistant tool_call + user tool_result + assistant text = 4 messages
	msgs := bridge.Messages(finalState)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	if msgs[0].Role != model.RoleUser {
		t.Error("msg[0] should be user")
	}
	if msgs[1].Role != model.RoleAssistant {
		t.Error("msg[1] should be assistant")
	}
	if msgs[2].Role != model.RoleUser {
		t.Error("msg[2] should be user (tool result)")
	}
	if msgs[3].Role != model.RoleAssistant {
		t.Error("msg[3] should be assistant")
	}

	// Final stop reason should be end_turn
	if bridge.StopReason(finalState) != "end_turn" {
		t.Errorf("expected final stop_reason=end_turn, got %q", bridge.StopReason(finalState))
	}

	// Provider should have been called twice
	if len(prov.Calls()) != 2 {
		t.Errorf("expected 2 provider calls, got %d", len(prov.Calls()))
	}
}

func TestNodeFactory_Create(t *testing.T) {
	bus := newTestBus()
	prov := gtesting.NewSequenceProvider()
	reg := tool.NewRegistry(bus)
	checker := permission.NewRuleChecker(nil, permission.ModeBypassPermissions, "/tmp", bus)
	orch := tool.NewOrchestrator(reg, checker, &permission.NonInteractivePrompter{}, bus)

	factory := bridge.NewNodeFactory(bridge.Infra{
		Provider:     prov,
		Orchestrator: orch,
		Registry:     reg,
		Bus:          bus,
		Cwd:          "/tmp",
	})

	// Valid types
	for _, nodeType := range []string{"llm", "tools", "eval", "reflect"} {
		_, err := factory.Create(nodeType, nil)
		if err != nil {
			t.Errorf("Create(%q) returned error: %v", nodeType, err)
		}
	}

	// Unknown type
	_, err := factory.Create("unknown", nil)
	if err == nil {
		t.Error("expected error for unknown node type")
	}
}
