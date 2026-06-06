package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
	history    []provider.RequestParams
}

func (tp *testProvider) Name() string { return "test" }

func (tp *testProvider) Stream(_ context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	tp.mu.Lock()
	tp.lastParams = params
	tp.history = append(tp.history, params)
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

func (tp *testProvider) Pricing(_ string) (model.Pricing, bool) { return tp.pricing, true }
func (tp *testProvider) ContextWindow(_ string) (int, bool)     { return 200_000, true }

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
func (a *allowAllChecker) AddPersistentRule(_ permission.Rule) error {
	return nil
}

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

type mutatingTestTool struct {
	invoked bool
}

func (m *mutatingTestTool) Name() string        { return "mutate" }
func (m *mutatingTestTool) Description() string { return "mutates test state" }
func (m *mutatingTestTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (m *mutatingTestTool) Invoke(_ context.Context, _ json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	m.invoked = true
	return tool.InvokeResult{Content: "mutated"}, nil
}
func (m *mutatingTestTool) CheckPerm(_ context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(context.Background(), "mutate", "")
}
func (m *mutatingTestTool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

type namedTestTool struct {
	name     string
	content  string
	readOnly bool
}

func (n namedTestTool) Name() string        { return n.name }
func (n namedTestTool) Description() string { return "test tool " + n.name }
func (n namedTestTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (n namedTestTool) Invoke(_ context.Context, _ json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	return tool.InvokeResult{Content: n.content}, nil
}
func (n namedTestTool) CheckPerm(_ context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(context.Background(), n.name, "")
}
func (n namedTestTool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: n.readOnly, Concurrent: false}
}

type bashOutputTool struct {
	outputs map[string]string
}

func (b bashOutputTool) Name() string        { return "Bash" }
func (b bashOutputTool) Description() string { return "test bash" }
func (b bashOutputTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`)
}
func (b bashOutputTool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return tool.InvokeResult{}, err
	}
	return tool.InvokeResult{Content: b.outputs[args.Command]}, nil
}
func (b bashOutputTool) CheckPerm(_ context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(context.Background(), "Bash", "")
}
func (b bashOutputTool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: false}
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

func messageText(msg model.Message) string {
	var b strings.Builder
	for _, part := range msg.Content {
		if tp, ok := part.(model.TextPart); ok {
			b.WriteString(tp.Text)
		}
	}
	return b.String()
}

func systemText(system model.SystemPrompt) string {
	var b strings.Builder
	for _, block := range system.Blocks {
		b.WriteString(block.Text)
		b.WriteString("\n")
	}
	return b.String()
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

func TestRun_IncludesMCPServerStatusInSystemPrompt(t *testing.T) {
	prov := &testProvider{
		turns:   [][]provider.StreamChunk{textChunks("ok", model.StopEndTurn)},
		pricing: model.Pricing{InputPerMToken: 3, OutputPerMToken: 15},
	}
	engine, _ := newTestEngine(prov)
	engine.config.MCPServerStatuses = func() []MCPServerStatus {
		return []MCPServerStatus{
			{Name: "chrome-devtools", Status: "connected"},
			{Name: "missing", Status: "failed"},
		}
	}

	events := drain(engine.Run(context.Background(), "Hi"))
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	var combined strings.Builder
	for _, block := range prov.lastParams.System.Blocks {
		combined.WriteString(block.Text)
		combined.WriteString("\n")
	}
	got := combined.String()
	for _, want := range []string{
		"mcp_servers:",
		"name: chrome-devtools",
		"status: connected",
		"name: missing",
		"status: failed",
		"answer directly from mcp_servers without calling tools",
		"ListMcpResourcesTool lists resources only",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, got)
		}
	}
}

func TestRun_AddsPatchGuidanceOnlyWhenApplyPatchActive(t *testing.T) {
	provWithPatch := &testProvider{turns: [][]provider.StreamChunk{textChunks("done", model.StopEndTurn)}}
	engine, _ := newTestEngine(provWithPatch, namedTestTool{name: "apply_patch"})
	drain(engine.Run(context.Background(), "hi"))
	if !strings.Contains(systemText(provWithPatch.lastParams.System), "use apply_patch") {
		t.Fatal("expected dynamic apply_patch guidance")
	}

	provWithoutPatch := &testProvider{turns: [][]provider.StreamChunk{textChunks("done", model.StopEndTurn)}}
	engine, _ = newTestEngine(provWithoutPatch, namedTestTool{name: "Edit"})
	drain(engine.Run(context.Background(), "hi"))
	if strings.Contains(systemText(provWithoutPatch.lastParams.System), "use apply_patch") {
		t.Fatal("did not expect apply_patch guidance without apply_patch tool")
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

func TestRun_StopAfterToolExec(t *testing.T) {
	toolCallID := "tc-stop-after-tool"
	prov := &testProvider{
		turns: [][]provider.StreamChunk{
			{
				{ToolCallStart: &model.ToolCallPart{ID: toolCallID, Name: "echo"}},
				{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: toolCallID, JSONDelta: `{"text":"handoff"}`}},
				{Done: &provider.StreamDone{StopReason: model.StopToolUse, Usage: model.TokenUsage{InputTokens: 80, OutputTokens: 20}}},
			},
			textChunks("should not be requested", model.StopEndTurn),
		},
	}
	engine, _ := newTestEngine(prov, echoTool{})
	engine.config.StopAfterToolExec = true

	events := drain(engine.Run(context.Background(), "Call echo once"))

	var gotToolCall, gotToolResult bool
	var complete *TurnCompleteEvent
	for _, ev := range events {
		switch e := ev.(type) {
		case ToolCallEvent:
			gotToolCall = true
			if e.Call.ID != toolCallID {
				t.Errorf("tool call ID = %q, want %q", e.Call.ID, toolCallID)
			}
		case ToolResultEvent:
			gotToolResult = true
			if e.Result.ToolCallID != toolCallID {
				t.Errorf("tool result ID = %q, want %q", e.Result.ToolCallID, toolCallID)
			}
			if e.Result.Content != "echo: handoff" {
				t.Errorf("tool result = %q, want %q", e.Result.Content, "echo: handoff")
			}
		case TextEvent:
			t.Fatalf("unexpected second provider text event: %q", e.Text)
		case TurnCompleteEvent:
			ev := e
			complete = &ev
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
	if complete == nil {
		t.Fatal("expected TurnCompleteEvent")
	}
	if complete.StopReason != model.StopToolUse {
		t.Errorf("StopReason = %s, want %s", complete.StopReason, model.StopToolUse)
	}

	prov.mu.Lock()
	calls := prov.callIdx
	prov.mu.Unlock()
	if calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}

	msgs := engine.store.Snapshot().Conversation.Messages
	if len(msgs) != 3 {
		t.Fatalf("conversation messages = %d, want 3", len(msgs))
	}
	if msgs[1].Role != model.RoleAssistant {
		t.Fatalf("message[1].Role = %s, want assistant", msgs[1].Role)
	}
	if _, ok := msgs[1].Content[0].(model.ToolCallPart); !ok {
		t.Fatalf("message[1] content[0] = %T, want ToolCallPart", msgs[1].Content[0])
	}
	if msgs[2].Role != model.RoleUser {
		t.Fatalf("message[2].Role = %s, want user", msgs[2].Role)
	}
	if _, ok := msgs[2].Content[0].(model.ToolResultPart); !ok {
		t.Fatalf("message[2] content[0] = %T, want ToolResultPart", msgs[2].Content[0])
	}
}

func TestRun_EndTurnCompletesAfterMutationWithoutValidationGate(t *testing.T) {
	prov := &testProvider{
		turns: [][]provider.StreamChunk{
			{
				{ToolCallStart: &model.ToolCallPart{ID: "write-1", Name: "Write"}},
				{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: "write-1", JSONDelta: `{}`}},
				{Done: &provider.StreamDone{StopReason: model.StopToolUse}},
			},
			textChunks("done", model.StopEndTurn),
		},
	}
	engine, _ := newTestEngine(
		prov,
		namedTestTool{name: "Write", content: "updated"},
	)
	events := drain(engine.Run(context.Background(), "change a file"))

	var complete *TurnCompleteEvent
	var text string
	for _, ev := range events {
		switch e := ev.(type) {
		case TextEvent:
			text += e.Text
		case TurnCompleteEvent:
			ev := e
			complete = &ev
		case ErrorEvent:
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}
	if complete == nil {
		t.Fatal("expected completion")
	}
	if !strings.Contains(text, "done") {
		t.Fatalf("expected final response, text=%q", text)
	}
	if prov.callIdx != 2 {
		t.Fatalf("provider calls = %d, want 2", prov.callIdx)
	}
	history := engine.store.Snapshot().Conversation.APIMessages()
	foundGate := false
	for _, msg := range history {
		if messageText(msg) != "" && strings.Contains(messageText(msg), "completion validation required") {
			foundGate = true
			break
		}
	}
	if foundGate {
		t.Fatal("did not expect completion validation prompt in conversation")
	}
}

func TestRun_FailedValidationCommandDoesNotInjectCompletionGate(t *testing.T) {
	prov := &testProvider{
		turns: [][]provider.StreamChunk{
			{
				{ToolCallStart: &model.ToolCallPart{ID: "write-1", Name: "Write"}},
				{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: "write-1", JSONDelta: `{}`}},
				{Done: &provider.StreamDone{StopReason: model.StopToolUse}},
			},
			{
				{ToolCallStart: &model.ToolCallPart{ID: "bash-1", Name: "Bash"}},
				{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: "bash-1", JSONDelta: `{"command":"go test ./internal/tui -run TestHistory"}`}},
				{Done: &provider.StreamDone{StopReason: model.StopToolUse}},
			},
			textChunks("done", model.StopEndTurn),
		},
	}
	engine, _ := newTestEngine(
		prov,
		namedTestTool{name: "Write", content: "updated"},
		bashOutputTool{outputs: map[string]string{
			"go test ./internal/tui -run TestHistory": "testing: warning: no tests to run\nPASS",
		}},
	)
	events := drain(engine.Run(context.Background(), "change a file"))

	var complete bool
	for _, ev := range events {
		switch ev.(type) {
		case TurnCompleteEvent:
			complete = true
		case ErrorEvent:
			t.Fatalf("unexpected error: %v", ev.(ErrorEvent).Err)
		}
	}
	if !complete {
		t.Fatal("expected completion")
	}
	if prov.callIdx != 3 {
		t.Fatalf("provider calls = %d, want 3", prov.callIdx)
	}
	history := engine.store.Snapshot().Conversation.APIMessages()
	var sawNoOpFeedback bool
	for _, msg := range history {
		if strings.Contains(messageText(msg), "no tests to run do not satisfy") {
			sawNoOpFeedback = true
			break
		}
	}
	if sawNoOpFeedback {
		t.Fatal("did not expect no-op validation feedback in conversation")
	}
}

func TestRun_StateHandoffSendsLatestToolExchangeOnly(t *testing.T) {
	toolCallID := "tc-state-handoff"
	prov := &testProvider{
		turns: [][]provider.StreamChunk{
			{
				{ToolCallStart: &model.ToolCallPart{ID: toolCallID, Name: "echo"}},
				{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: toolCallID, JSONDelta: `{"text":"handoff"}`}},
				{Done: &provider.StreamDone{StopReason: model.StopToolUse, Usage: model.TokenUsage{InputTokens: 80, OutputTokens: 20}}},
			},
			textChunks("done", model.StopEndTurn),
		},
	}
	engine, _ := newTestEngine(prov, echoTool{})
	engine.config.ContextMode = model.ContextModeStateHandoff
	engine.config.HandoffSchema = model.HandoffSchemaV1

	events := drain(engine.Run(context.Background(), "Find the request boundary and keep going"))
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	prov.mu.Lock()
	history := append([]provider.RequestParams(nil), prov.history...)
	prov.mu.Unlock()
	if len(history) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(history))
	}

	second := history[1]
	if len(second.Messages) != 2 {
		t.Fatalf("second request messages = %d, want latest assistant tool call plus user tool result", len(second.Messages))
	}
	if second.Messages[0].Role != model.RoleAssistant || !messageHasToolCall(second.Messages[0]) {
		t.Fatalf("second message[0] = role %s content %#v, want assistant tool call", second.Messages[0].Role, second.Messages[0].Content)
	}
	if second.Messages[1].Role != model.RoleUser || !messageHasToolResult(second.Messages[1]) {
		t.Fatalf("second message[1] = role %s content %#v, want user tool result", second.Messages[1].Role, second.Messages[1].Content)
	}
	for _, msg := range second.Messages {
		if strings.Contains(messageText(msg), "Find the request boundary") {
			t.Fatalf("second request leaked original user message: %#v", second.Messages)
		}
	}

	sys := systemText(second.System)
	if !strings.Contains(sys, "current_handoff_state:") {
		t.Fatalf("system prompt missing handoff state:\n%s", sys)
	}
	if !strings.Contains(sys, "Find the request boundary and keep going") {
		t.Fatalf("system prompt missing goal in handoff state:\n%s", sys)
	}

	var foundPatchTool bool
	for _, td := range second.Tools {
		if td.Name == "PatchHandoffState" {
			foundPatchTool = true
			break
		}
	}
	if !foundPatchTool {
		t.Fatalf("PatchHandoffState tool missing from state-handoff request")
	}
}

func TestRun_ChatModeExcludesHandoffToolsAndSystemText(t *testing.T) {
	prov := &testProvider{
		turns: [][]provider.StreamChunk{textChunks("done", model.StopEndTurn)},
	}
	engine, _ := newTestEngine(prov, echoTool{})

	events := drain(engine.Run(context.Background(), "Finish the task"))
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	for _, td := range prov.lastParams.Tools {
		if td.Name == "PatchHandoffState" || td.Name == "CertifyFact" {
			t.Fatalf("chat mode exposed handoff tool %q", td.Name)
		}
	}
	sys := systemText(prov.lastParams.System)
	if strings.Contains(sys, "PatchHandoffState") || strings.Contains(sys, "current_handoff_state") {
		t.Fatalf("chat mode leaked handoff system text:\n%s", sys)
	}
}

func TestExecuteToolBatchRejectsHandoffToolsOutsideStateHandoff(t *testing.T) {
	engine, _ := newTestEngine(&testProvider{})
	calls := []model.ToolCallPart{
		{ID: "patch-call", Name: "PatchHandoffState", Input: json.RawMessage(`{"ops":[]}`)},
		{ID: "certify-call", Name: "CertifyFact", Input: json.RawMessage(`{"id":"x","kind":"tool_result_contains"}`)},
	}

	result, err := engine.executeToolBatch(context.Background(), calls, engine.store.Snapshot(), nil)
	if err != nil {
		t.Fatalf("executeToolBatch: %v", err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(result.Results))
	}
	for _, got := range result.Results {
		if !got.IsError {
			t.Fatalf("handoff tool result should be an error: %#v", got)
		}
		if !strings.Contains(got.Content, "only available in state-handoff") {
			t.Fatalf("unexpected rejection content: %q", got.Content)
		}
	}
	if !engine.store.Snapshot().HandoffState.IsZero() {
		t.Fatal("handoff state should not be created outside state-handoff mode")
	}
}

func TestRun_StateHandoffPatchToolUpdatesStateAndPreservesResults(t *testing.T) {
	patchCallID := "tc-patch"
	echoCallID := "tc-echo"
	prov := &testProvider{
		turns: [][]provider.StreamChunk{
			{
				{ToolCallStart: &model.ToolCallPart{ID: patchCallID, Name: "PatchHandoffState"}},
				{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: patchCallID, JSONDelta: `{"ops":[{"op":"add","path":"/latest_tool_result_interpretation","value":"echo result guides next step"},{"op":"add","path":"/completed/-","value":"patched durable state"}]}`}},
				{ToolCallStart: &model.ToolCallPart{ID: echoCallID, Name: "echo"}},
				{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: echoCallID, JSONDelta: `{"text":"next"}`}},
				{Done: &provider.StreamDone{StopReason: model.StopToolUse, Usage: model.TokenUsage{InputTokens: 90, OutputTokens: 30}}},
			},
			textChunks("done", model.StopEndTurn),
		},
	}
	engine, _ := newTestEngine(prov, echoTool{})
	engine.config.ContextMode = model.ContextModeStateHandoff
	engine.config.HandoffSchema = model.HandoffSchemaV1

	events := drain(engine.Run(context.Background(), "Keep state patched while using tools"))
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	snap := engine.store.Snapshot()
	if snap.HandoffState.LatestToolResultInterpretation != "echo result guides next step" {
		t.Fatalf("LatestToolResultInterpretation = %q", snap.HandoffState.LatestToolResultInterpretation)
	}
	if len(snap.HandoffState.Completed) != 1 || snap.HandoffState.Completed[0] != "patched durable state" {
		t.Fatalf("Completed = %#v", snap.HandoffState.Completed)
	}

	msgs := snap.Conversation.Messages
	if len(msgs) < 3 {
		t.Fatalf("conversation messages = %d, want at least 3", len(msgs))
	}
	resultsMsg := msgs[2]
	if resultsMsg.Role != model.RoleUser {
		t.Fatalf("results message role = %s, want user", resultsMsg.Role)
	}
	if len(resultsMsg.Content) != 2 {
		t.Fatalf("result parts = %d, want 2", len(resultsMsg.Content))
	}
	first, ok := resultsMsg.Content[0].(model.ToolResultPart)
	if !ok {
		t.Fatalf("result[0] = %T, want ToolResultPart", resultsMsg.Content[0])
	}
	second, ok := resultsMsg.Content[1].(model.ToolResultPart)
	if !ok {
		t.Fatalf("result[1] = %T, want ToolResultPart", resultsMsg.Content[1])
	}
	if first.ToolCallID != patchCallID || first.Content != "handoff state patched" || first.IsError {
		t.Fatalf("patch result = %#v", first)
	}
	if second.ToolCallID != echoCallID || second.Content != "echo: next" || second.IsError {
		t.Fatalf("echo result = %#v", second)
	}

	prov.mu.Lock()
	history := append([]provider.RequestParams(nil), prov.history...)
	prov.mu.Unlock()
	if len(history) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(history))
	}
	if len(history[1].Messages) != 2 {
		t.Fatalf("second request messages = %d, want latest exchange only", len(history[1].Messages))
	}
	if !messageHasToolCall(history[1].Messages[0]) || !messageHasToolResult(history[1].Messages[1]) {
		t.Fatalf("second request did not preserve latest tool call/result exchange: %#v", history[1].Messages)
	}
}

func TestRun_StateHandoffDoesNotBlockMutatingTools(t *testing.T) {
	patchCallID := "tc-patch"
	mutateCallID := "tc-mutate"
	mutate := &mutatingTestTool{}
	prov := &testProvider{
		turns: [][]provider.StreamChunk{
			{
				{ToolCallStart: &model.ToolCallPart{ID: patchCallID, Name: "PatchHandoffState"}},
				{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: patchCallID, JSONDelta: `{"ops":[{"op":"replace","path":"/current_focus","value":"trying to edit too early"}]}`}},
				{ToolCallStart: &model.ToolCallPart{ID: mutateCallID, Name: "mutate"}},
				{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: mutateCallID, JSONDelta: `{}`}},
				{Done: &provider.StreamDone{StopReason: model.StopToolUse, Usage: model.TokenUsage{InputTokens: 90, OutputTokens: 30}}},
			},
			textChunks("done", model.StopEndTurn),
		},
	}
	engine, _ := newTestEngine(prov, mutate)
	engine.config.ContextMode = model.ContextModeStateHandoff
	engine.config.HandoffSchema = model.HandoffSchemaV1

	events := drain(engine.Run(context.Background(), "Change code without enough investigation"))
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}
	if !mutate.invoked {
		t.Fatal("mutating tool was blocked in state-handoff mode")
	}
	resultsMsg := engine.store.Snapshot().Conversation.Messages[2]
	result := resultsMsg.Content[1].(model.ToolResultPart)
	if result.IsError || result.Content != "mutated" {
		t.Fatalf("mutating result = %#v, want successful mutation result", result)
	}
}

func TestRun_StateHandoffAllowsMutatingToolsAfterInvestigationReady(t *testing.T) {
	patchCallID := "tc-patch"
	certifyCallID := "tc-certify"
	mutateCallID := "tc-mutate"
	mutate := &mutatingTestTool{}
	contractFile := t.TempDir() + "/contract.txt"
	if err := os.WriteFile(contractFile, []byte("real contract marker"), 0o644); err != nil {
		t.Fatal(err)
	}
	patchInput := `{"ops":[{"op":"add","path":"/investigation/certified_fact_refs/-","value":"contract_fact"},{"op":"add","path":"/investigation/observed_contracts/-","value":{"name":"test contract","source":"contract.txt","evidence":"real contract marker","fact_refs":["contract_fact"]}},{"op":"add","path":"/investigation/acceptance_checks/-","value":{"description":"run against real contract","command":"go test ./internal/query","expected_result":"pass","fact_refs":["contract_fact"]}},{"op":"replace","path":"/investigation/ready_for_changes","value":true}]}`
	certifyInput := fmt.Sprintf(`{"id":"contract_fact","kind":"file_contains","path":%q,"contains":"real contract marker","claim":"contract file contains the marker"}`, contractFile)
	prov := &testProvider{
		turns: [][]provider.StreamChunk{
			{
				{ToolCallStart: &model.ToolCallPart{ID: patchCallID, Name: "PatchHandoffState"}},
				{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: patchCallID, JSONDelta: patchInput}},
				{ToolCallStart: &model.ToolCallPart{ID: certifyCallID, Name: "CertifyFact"}},
				{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: certifyCallID, JSONDelta: certifyInput}},
				{ToolCallStart: &model.ToolCallPart{ID: mutateCallID, Name: "mutate"}},
				{ToolCallInputDelta: &provider.ToolCallDelta{ToolCallID: mutateCallID, JSONDelta: `{}`}},
				{Done: &provider.StreamDone{StopReason: model.StopToolUse, Usage: model.TokenUsage{InputTokens: 90, OutputTokens: 30}}},
			},
			textChunks("done", model.StopEndTurn),
		},
	}
	engine, _ := newTestEngine(prov, mutate)
	engine.config.ContextMode = model.ContextModeStateHandoff
	engine.config.HandoffSchema = model.HandoffSchemaV1

	events := drain(engine.Run(context.Background(), "Change code after enough investigation"))
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}
	if !mutate.invoked {
		t.Fatal("mutating tool was not invoked after investigation gate was ready")
	}
	resultsMsg := engine.store.Snapshot().Conversation.Messages[2]
	result := resultsMsg.Content[2].(model.ToolResultPart)
	if result.IsError || result.Content != "mutated" {
		t.Fatalf("mutating result = %#v, want successful mutation", result)
	}
	if _, ok := engine.store.Snapshot().HandoffState.CertifiedFacts["contract_fact"]; !ok {
		t.Fatal("certified fact was not stored")
	}
}

func TestRecordHandoffToolFailuresPromotesVerifiedFeedback(t *testing.T) {
	engine, _ := newTestEngine(&testProvider{})
	engine.config.ContextMode = model.ContextModeStateHandoff
	engine.config.HandoffSchema = model.HandoffSchemaV1

	engine.recordHandoffToolFailures(
		[]model.ToolCallPart{
			{
				ID:    "tc-bash",
				Name:  "Bash",
				Input: json.RawMessage(`{"command":"go test ./internal/query"}`),
			},
			{
				ID:    "tc-edit",
				Name:  "Edit",
				Input: json.RawMessage(`{"file_path":"internal/query/loop.go"}`),
			},
		},
		[]model.ToolResultPart{
			{
				ToolCallID: "tc-bash",
				Content:    "internal/query/loop_observability_test.go:46:8: impossible type switch case\nExit code 1",
			},
			{
				ToolCallID: "tc-edit",
				Content:    "string to replace not found in file",
				IsError:    true,
			},
		},
		[]string{"exit_code:1", ""},
	)

	state := engine.store.Snapshot().HandoffState
	if len(state.VerifiedFailures) != 2 {
		t.Fatalf("VerifiedFailures = %#v, want 2 entries", state.VerifiedFailures)
	}
	if state.VerifiedFailures[0].Command != "go test ./internal/query" {
		t.Fatalf("bash command summary = %q", state.VerifiedFailures[0].Command)
	}
	if state.VerifiedFailures[0].ErrorType != "exit_code:1" {
		t.Fatalf("bash error type = %q", state.VerifiedFailures[0].ErrorType)
	}
	if !strings.Contains(state.VerifiedFailures[0].OutputExcerpt, "impossible type switch case") {
		t.Fatalf("bash output excerpt = %q", state.VerifiedFailures[0].OutputExcerpt)
	}
	if state.VerifiedFailures[1].Command != "Edit internal/query/loop.go" {
		t.Fatalf("edit command summary = %q", state.VerifiedFailures[1].Command)
	}
	if !containsString(state.InvalidatedAssumptions, "The shell command succeeded.") {
		t.Fatalf("InvalidatedAssumptions missing shell failure: %#v", state.InvalidatedAssumptions)
	}
	if !containsString(state.InvalidatedAssumptions, "The Edit old_string or target region exists exactly in the current file.") {
		t.Fatalf("InvalidatedAssumptions missing edit failure: %#v", state.InvalidatedAssumptions)
	}
	if !strings.Contains(state.NextAction, "Before another Edit") {
		t.Fatalf("NextAction = %q", state.NextAction)
	}
}

func TestRecordHandoffToolFailuresUsesEditRetryCandidate(t *testing.T) {
	engine, _ := newTestEngine(&testProvider{})
	engine.config.ContextMode = model.ContextModeStateHandoff
	engine.config.HandoffSchema = model.HandoffSchemaV1

	engine.recordHandoffToolFailures(
		[]model.ToolCallPart{
			{
				ID:    "tc-edit",
				Name:  "Edit",
				Input: json.RawMessage(`{"file_path":"internal/tui/ask.go"}`),
			},
		},
		[]model.ToolResultPart{
			{
				ToolCallID: "tc-edit",
				Content:    "string to replace not found in file.\nRetry with this exact old_string:\n```\n\tfreeText    strings.Builder   // typed text\n```\nString: bad",
				IsError:    true,
			},
		},
		[]string{""},
	)

	state := engine.store.Snapshot().HandoffState
	if !strings.Contains(state.NextAction, "Retry the Edit using this exact old_string") {
		t.Fatalf("NextAction = %q", state.NextAction)
	}
	if !strings.Contains(state.NextAction, "\tfreeText    strings.Builder   // typed text") {
		t.Fatalf("NextAction missing retry candidate: %q", state.NextAction)
	}
	if strings.Contains(state.NextAction, "re-read") {
		t.Fatalf("NextAction should not ask for another blind read: %q", state.NextAction)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestCertifyFactToolSchemaNamesSupportedKinds(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
			Type string   `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(certifyFactToolDef().InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(schema.Properties["kind"].Enum, ",")
	want := "file_contains,json_shape,jsonl_shape,tool_result_contains"
	if got != want {
		t.Fatalf("CertifyFact kind enum = %q, want %q", got, want)
	}
	if schema.Properties["required_paths"].Type != "array" {
		t.Fatalf("CertifyFact required_paths type = %q, want array", schema.Properties["required_paths"].Type)
	}
}

func TestCertifyJSONShapeSupportsNestedRequiredPaths(t *testing.T) {
	dir := t.TempDir()
	logPath := dir + "/recording.jsonl"
	data := strings.Join([]string{
		`{"kind":"APIRequestCompleted","stop_reason":"tool_use","content":[{"type":"tool_call","data":{"name":"PatchHandoffState","input":{"ops":[]}}}]}`,
		`{"kind":"APIRequestCompleted","stop_reason":"end_turn","content":[{"type":"text","data":{"text":"done"}}]}`,
	}, "\n")
	if err := os.WriteFile(logPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	engine, _ := newTestEngine(&testProvider{})
	engine.store.Update(func(s *app.AppState) {
		s.CWD = dir
	})

	fact, err := engine.certifyFact(certifyFactInput{
		ID:            "api-completed-tool-call-shape",
		Kind:          "jsonl_shape",
		Path:          "recording.jsonl",
		RequiredPaths: []string{"content.0.type", "content.0.data.name", "content.0.data.input.ops"},
		Selector:      &certifyFactSelector{Field: "stop_reason", Equals: "tool_use"},
		Claim:         "tool-use APIRequestCompleted records carry nested tool call shape",
	})
	if err != nil {
		t.Fatalf("certifyFact: %v", err)
	}
	if fact.MatchingRecords != 1 {
		t.Fatalf("MatchingRecords = %d, want 1", fact.MatchingRecords)
	}
	if strings.Join(fact.Paths, ",") != "content.0.data.input.ops,content.0.data.name,content.0.type" {
		t.Fatalf("Paths = %#v", fact.Paths)
	}
}

func TestMessagesForRequest_StateHandoffIncludesLatestToolExchangeBeforeNewUserMessage(t *testing.T) {
	engine, _ := newTestEngine(&testProvider{})
	engine.config.ContextMode = model.ContextModeStateHandoff

	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", "/tmp/test")
	conv.Append(model.Message{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "old user"}}})
	conv.Append(model.Message{Role: model.RoleAssistant, Content: []model.ContentPart{
		model.ToolCallPart{ID: "tc-1", Name: "echo", Input: json.RawMessage(`{"text":"old"}`)},
	}})
	conv.Append(model.Message{Role: model.RoleUser, Content: []model.ContentPart{
		model.ToolResultPart{ToolCallID: "tc-1", Content: "old result"},
	}})
	conv.Append(model.Message{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: "old answer"}}})
	conv.Append(model.Message{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "new instruction"}}})

	got := engine.messagesForRequest(conv)
	if len(got) != 3 {
		t.Fatalf("messages = %d, want latest tool exchange plus latest user message", len(got))
	}
	if got[0].Role != model.RoleAssistant || !messageHasToolCall(got[0]) {
		t.Fatalf("message[0] = %#v, want latest assistant tool call", got[0])
	}
	if got[1].Role != model.RoleUser || !messageHasToolResult(got[1]) {
		t.Fatalf("message[1] = %#v, want latest user tool result", got[1])
	}
	if got[2].Role != model.RoleUser || messageText(got[2]) != "new instruction" {
		t.Fatalf("message[2] = %#v, want latest user instruction", got[2])
	}
}

func TestMessagesForRequest_StateHandoffBoundsLatestToolResults(t *testing.T) {
	engine, _ := newTestEngine(&testProvider{})
	engine.config.ContextMode = model.ContextModeStateHandoff

	large := strings.Repeat("x", stateHandoffMaxToolResultChars+10_000)
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", "/tmp/test")
	conv.Append(model.Message{Role: model.RoleAssistant, Content: []model.ContentPart{
		model.ToolCallPart{ID: "tc-1", Name: "Read", Input: json.RawMessage(`{"file_path":"large.log"}`)},
	}})
	conv.Append(model.Message{Role: model.RoleUser, Content: []model.ContentPart{
		model.ToolResultPart{ToolCallID: "tc-1", Content: large},
	}})

	got := engine.messagesForRequest(conv)
	if len(got) != 2 {
		t.Fatalf("messages = %d, want latest exchange", len(got))
	}
	result := got[1].Content[0].(model.ToolResultPart)
	if len(result.Content) > stateHandoffMaxToolResultChars {
		t.Fatalf("bounded result length = %d, want <= %d", len(result.Content), stateHandoffMaxToolResultChars)
	}
	if !strings.Contains(result.Content, "tool result truncated for state-handoff prompt") {
		t.Fatalf("bounded result missing truncation marker: %q", result.Content[len(result.Content)-200:])
	}
	original := conv.Messages[1].Content[0].(model.ToolResultPart)
	if original.Content != large {
		t.Fatal("messagesForRequest mutated stored conversation tool result")
	}
}

func TestMessagesForRequest_StateHandoffUsesLatestUserMessageWithoutToolExchange(t *testing.T) {
	engine, _ := newTestEngine(&testProvider{})
	engine.config.ContextMode = model.ContextModeStateHandoff

	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", "/tmp/test")
	conv.Append(model.Message{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "old user"}}})
	conv.Append(model.Message{Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: "old answer"}}})
	conv.Append(model.Message{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "new instruction"}}})

	got := engine.messagesForRequest(conv)
	if len(got) != 1 {
		t.Fatalf("messages = %d, want latest user message only", len(got))
	}
	if got[0].Role != model.RoleUser || messageText(got[0]) != "new instruction" {
		t.Fatalf("message = %#v, want latest user instruction", got[0])
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

func TestRun_DoesNotInjectTurnBudgetWarning(t *testing.T) {
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

	// Inspect conversation messages for Pragma-only turn budget guidance.
	snap := engine.store.Snapshot()
	msgs := snap.Conversation.APIMessages()

	var foundWarning bool
	for _, m := range msgs {
		for _, part := range m.Content {
			if tp, ok := part.(model.TextPart); ok {
				if strings.Contains(tp.Text, "turns remaining") {
					foundWarning = true
				}
			}
		}
	}
	if foundWarning {
		t.Error("did not expect turn budget warning in conversation")
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
	for range 3 {
		// Preflight compaction turn — empty text triggers ErrEmptySummary
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
