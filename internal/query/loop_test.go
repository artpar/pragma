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
