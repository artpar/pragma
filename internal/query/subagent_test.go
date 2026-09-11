package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

// SUB-001 unit gates: def shape, recursion guard, fresh-conversation fork,
// envelope, pairing, validation.

func newTestStore(conv model.Conversation) *app.StateStore {
	return app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
}

func newTestBus() *observe.EventBus {
	return observe.NewEventBus(64)
}

func TestAgentToolDefShape(t *testing.T) {
	def := agentToolDef()
	if def.Name != AgentToolName {
		t.Fatalf("name = %q", def.Name)
	}
	var schema map[string]any
	if err := json.Unmarshal(def.InputSchema, &schema); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	if req, _ := schema["required"].([]any); len(req) != 1 || req[0] != "prompt" {
		t.Fatalf("required = %v", schema["required"])
	}
}

func TestWithSubAgentToolRecursionGuard(t *testing.T) {
	engine := &Engine{}
	tools := []model.ToolDef{{Name: "Bash"}}
	out := engine.withSubAgentTool(tools)
	if len(out) != 2 || out[1].Name != AgentToolName {
		t.Fatalf("root engine tools = %v, want Agent appended", toolNamesOf(out))
	}
	engine.config.DisableSubAgents = true
	out = engine.withSubAgentTool(tools)
	if len(out) != 1 {
		t.Fatalf("sub-agent engine tools = %v, want no Agent", toolNamesOf(out))
	}
	// collision: an existing Agent-named tool is not shadowed
	engine.config.DisableSubAgents = false
	out = engine.withSubAgentTool([]model.ToolDef{{Name: "Bash"}, {Name: AgentToolName}})
	if len(out) != 2 {
		t.Fatalf("collision tools = %v", toolNamesOf(out))
	}
}

func TestProviderToolsLoopExecutesAgentTool(t *testing.T) {
	agentInput, err := json.Marshal(map[string]string{
		"prompt":      "Say SUB_OK",
		"description": "probe",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Provider response queue: parent turn 1 → Agent call; the sub-agent's
	// single request → final text; parent turn 2 → final text.
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "call-agent-1", Name: AgentToolName, Input: agentInput},
			},
			StopReason: model.StopToolUse,
		},
		{Content: []model.ContentPart{model.TextPart{Text: "SUB_OK"}}, StopReason: model.StopEndTurn},
		{Content: []model.ContentPart{model.TextPart{Text: "parent done"}}, StopReason: model.StopEndTurn},
	}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := newTestStore(conv)
	engine := NewEngine(prov, store, model.NewCostTracker(0), newTestBus(), EngineConfig{
		Model:     "test-model",
		LoopMode:  LoopModeProviderTools,
		MaxTokens: 4096,
		MaxTurns:  5,
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "delegate"))

	if prov.calls != 3 {
		t.Fatalf("provider calls = %d, want 3 (parent, sub, parent)", prov.calls)
	}
	// The sub-agent request must be a fresh conversation with no Agent tool.
	subReq := prov.requests[1]
	if len(subReq.Tools) == 0 {
		t.Fatalf("sub request lost tools entirely")
	}
	for _, tool := range subReq.Tools {
		if tool.Name == AgentToolName {
			t.Fatalf("sub request advertises Agent (recursion guard broken): %v", toolNamesOf(subReq.Tools))
		}
	}
	userMsgs := 0
	for _, msg := range subReq.Messages {
		if msg.Role == model.RoleUser {
			userMsgs++
		}
	}
	if userMsgs != 1 {
		t.Fatalf("sub request user messages = %d, want exactly 1 (fresh conversation)", userMsgs)
	}
	// Parent requests must advertise Agent, and the result must pair.
	for i := 0; i < 3; i += 2 {
		if !contains(toolNamesOf(prov.requests[i].Tools), AgentToolName) {
			t.Fatalf("parent request %d missing Agent: %v", i, toolNamesOf(prov.requests[i].Tools))
		}
	}
	var result ToolResultEvent
	for _, ev := range events {
		if e, ok := ev.(ToolResultEvent); ok {
			result = e
		}
	}
	if result.Result.ToolCallID != "call-agent-1" {
		t.Fatalf("ToolCallID = %q", result.Result.ToolCallID)
	}
	if result.Result.IsError {
		t.Fatalf("result errored: %q", result.Result.Content)
	}
	var envelope subAgentResult
	if err := json.Unmarshal([]byte(result.Result.Content), &envelope); err != nil {
		t.Fatalf("result not the JSON envelope: %v (%s)", err, result.Result.Content)
	}
	if envelope.Status != "completed" || envelope.Result != "SUB_OK" {
		t.Fatalf("envelope = %+v", envelope)
	}
	if envelope.Prompt != "Say SUB_OK" {
		t.Fatalf("envelope prompt = %q", envelope.Prompt)
	}
	if err := validateToolResultPairing(prov.requests[2].Messages); err != nil {
		t.Fatalf("pairing failed on parent request 3: %v", err)
	}
}

func TestProviderToolsLoopAgentValidation(t *testing.T) {
	engine, prov := newProviderToolsEngine(t, []model.Response{
		{Content: []model.ContentPart{model.TextPart{Text: "done"}}, StopReason: model.StopEndTurn},
	}, EngineConfig{})
	events := collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))
	_ = events

	for _, bad := range []string{`{"description":"no prompt"}`, `{bad json`} {
		engine2, _ := newProviderToolsEngine(t, []model.Response{
			{Content: []model.ContentPart{model.TextPart{Text: "done"}}, StopReason: model.StopEndTurn},
		}, EngineConfig{})
		// dispatch the bad call directly through the production path
		result := engine2.executeProviderToolCall(t.Context(), model.ToolCallPart{ID: "x", Name: AgentToolName, Input: []byte(bad)})
		if !result.IsError {
			t.Fatalf("bad input %s must produce an error result", bad)
		}
		if result.Content == "" {
			t.Fatal("error result must carry a message")
		}
	}
	_ = prov
}

type failAtProvider struct {
	responses []model.Response
	failAt    int
	calls     int
	requests  []provider.RequestParams
}

func (p *failAtProvider) Name() string { return "test" }

func (p *failAtProvider) Stream(context.Context, provider.RequestParams) (<-chan provider.StreamChunk, error) {
	return nil, fmt.Errorf("Stream is not used by this test provider")
}

func (p *failAtProvider) Complete(_ context.Context, params provider.RequestParams) (model.Response, error) {
	idx := p.calls
	p.calls++
	p.requests = append(p.requests, params)
	if idx == p.failAt {
		return model.Response{}, fmt.Errorf("subagent provider failure at call %d", idx)
	}
	if idx >= len(p.responses) {
		return model.Response{}, fmt.Errorf("no response configured for call %d", idx)
	}
	return p.responses[idx], nil
}

func (p *failAtProvider) SupportsFeature(provider.Feature) bool { return true }

func (p *failAtProvider) Pricing(string) (model.Pricing, bool) { return model.Pricing{}, false }

func (p *failAtProvider) ContextWindow(string) (int, bool) { return 200_000, true }

func TestProviderToolsLoopAgentSubFailurePairsError(t *testing.T) {
	agentInput, _ := json.Marshal(map[string]string{"prompt": "fail please"})
	prov := &failAtProvider{
		failAt: 1, // the sub-agent's request errors; parent requests succeed
		responses: []model.Response{
			{
				Content: []model.ContentPart{
					model.ToolCallPart{ID: "call-agent-2", Name: AgentToolName, Input: agentInput},
				},
				StopReason: model.StopToolUse,
			},
			{}, // unused: call 1 fails
			{Content: []model.ContentPart{model.TextPart{Text: "parent done"}}, StopReason: model.StopEndTurn},
		},
	}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := newTestStore(conv)
	engine := NewEngine(prov, store, model.NewCostTracker(0), newTestBus(), EngineConfig{
		Model:     "test-model",
		LoopMode:  LoopModeProviderTools,
		MaxTokens: 4096,
		MaxTurns:  5,
	})

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "delegate"))

	var result ToolResultEvent
	for _, ev := range events {
		if e, ok := ev.(ToolResultEvent); ok {
			result = e
		}
	}
	if !result.Result.IsError {
		t.Fatal("sub-agent failure must produce an error result")
	}
	if !strings.Contains(result.Result.Content, "Agent failed") {
		t.Fatalf("failure content = %q", result.Result.Content)
	}
	if result.Result.ToolCallID != "call-agent-2" {
		t.Fatalf("ToolCallID = %q", result.Result.ToolCallID)
	}
	if prov.calls != 3 {
		t.Fatalf("provider calls = %d, want 3 (failure result must still pair)", prov.calls)
	}
	if err := validateToolResultPairing(prov.requests[2].Messages); err != nil {
		t.Fatalf("pairing failed on parent request 3: %v", err)
	}
}
