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
)

// mcpEchoToolDef mirrors the shape produced by internal/mcp.ToolDef.
func mcpEchoToolDef(name string) model.ToolDef {
	return model.ToolDef{
		Name:        name,
		Description: "Echoes input through a test MCP server",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}`),
	}
}

func newProviderToolsEngine(t *testing.T, responses []model.Response, cfg EngineConfig) (*Engine, *pragmaLoopTestProvider) {
	t.Helper()
	prov := &pragmaLoopTestProvider{responses: responses}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          conv.WorkDir,
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	cfg.Model = "test-model"
	cfg.LoopMode = LoopModeProviderTools
	cfg.MaxTokens = 4096
	cfg.MaxTurns = 5
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(64), cfg)
	return engine, prov
}

func toolNamesOf(tools []model.ToolDef) []string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name)
	}
	return names
}

func contains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// MCPINJ-001 gate: the provider-tools loop must carry injected MCP tool defs
// on every request, after the built-ins.
func TestProviderToolsLoopInjectsMCPToolDefs(t *testing.T) {
	engine, prov := newProviderToolsEngine(t, []model.Response{
		{Content: []model.ContentPart{model.TextPart{Text: "done"}}, StopReason: model.StopEndTurn},
	}, EngineConfig{})
	engine.config.MCPToolDefs = func(_ context.Context) []model.ToolDef {
		return []model.ToolDef{mcpEchoToolDef("mcp__test-server__echo")}
	}

	collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))

	if len(prov.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(prov.requests))
	}
	names := toolNamesOf(prov.requests[0].Tools)
	if !contains(names, "Bash") || !contains(names, "apply_patch") {
		t.Fatalf("built-in tools missing: %v", names)
	}
	if !contains(names, "mcp__test-server__echo") {
		t.Fatalf("MCP tool def not injected: %v", names)
	}
	if names[len(names)-1] != "mcp__test-server__echo" {
		t.Fatalf("injected defs must follow built-ins, got %v", names)
	}
}

// MCPINJ-001 gate: without hooks the tool list is exactly the built-ins.
func TestProviderToolsLoopWithoutMCPHooksSendsBuiltinsOnly(t *testing.T) {
	engine, prov := newProviderToolsEngine(t, []model.Response{
		{Content: []model.ContentPart{model.TextPart{Text: "done"}}, StopReason: model.StopEndTurn},
	}, EngineConfig{})

	collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))

	if len(prov.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(prov.requests))
	}
	names := toolNamesOf(prov.requests[0].Tools)
	if len(names) != 2 || !contains(names, "Bash") || !contains(names, "apply_patch") {
		t.Fatalf("tools = %v, want exactly [Bash apply_patch]", names)
	}
}

// MCPINJ-001 gate: an injected MCP tool call routes through MCPCallTool and
// its result pairs with the call (the second request passes pairing
// validation).
func TestProviderToolsLoopExecutesMCPToolCall(t *testing.T) {
	callInput, err := json.Marshal(map[string]string{"message": "mcp-routing"})
	if err != nil {
		t.Fatal(err)
	}
	engine, prov := newProviderToolsEngine(t, []model.Response{
		{
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "call-mcp-1", Name: "mcp__test-server__echo", Input: callInput},
			},
			StopReason: model.StopToolUse,
		},
		{Content: []model.ContentPart{model.TextPart{Text: "done"}}, StopReason: model.StopEndTurn},
	}, EngineConfig{})

	var routedName string
	var routedInput json.RawMessage
	engine.config.MCPToolDefs = func(_ context.Context) []model.ToolDef {
		return []model.ToolDef{mcpEchoToolDef("mcp__test-server__echo")}
	}
	engine.config.MCPCallTool = func(_ context.Context, name string, input json.RawMessage) (string, error) {
		routedName = name
		routedInput = input
		return "echo:mcp-routing", nil
	}

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))

	if routedName != "mcp__test-server__echo" {
		t.Fatalf("routed tool name = %q", routedName)
	}
	if string(routedInput) != string(callInput) {
		t.Fatalf("routed input = %s, want %s", routedInput, callInput)
	}
	var resultEvent ToolResultEvent
	for _, ev := range events {
		if e, ok := ev.(ToolResultEvent); ok {
			resultEvent = e
		}
	}
	if resultEvent.Result.ToolCallID != "call-mcp-1" {
		t.Fatalf("result ToolCallID = %q, want call-mcp-1", resultEvent.Result.ToolCallID)
	}
	if resultEvent.Result.Content != "echo:mcp-routing" || resultEvent.Result.IsError {
		t.Fatalf("result = %+v, want echo:mcp-routing non-error", resultEvent.Result)
	}
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2 (second request requires paired results)", prov.calls)
	}
	if !contains(toolNamesOf(prov.requests[1].Tools), "mcp__test-server__echo") {
		t.Fatalf("second request lost the injected tool: %v", toolNamesOf(prov.requests[1].Tools))
	}
}

// MCPINJ-001 gate: a failing MCP tool call still produces a paired,
// error-marked result and the loop continues.
func TestProviderToolsLoopMCPToolCallFailurePairsResult(t *testing.T) {
	callInput, err := json.Marshal(map[string]string{"message": "boom"})
	if err != nil {
		t.Fatal(err)
	}
	engine, prov := newProviderToolsEngine(t, []model.Response{
		{
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "call-mcp-2", Name: "mcp__test-server__boom", Input: callInput},
			},
			StopReason: model.StopToolUse,
		},
		{Content: []model.ContentPart{model.TextPart{Text: "done"}}, StopReason: model.StopEndTurn},
	}, EngineConfig{})

	engine.config.MCPToolDefs = func(_ context.Context) []model.ToolDef {
		return []model.ToolDef{mcpEchoToolDef("mcp__test-server__boom")}
	}
	engine.config.MCPCallTool = func(_ context.Context, name string, _ json.RawMessage) (string, error) {
		return "", fmt.Errorf("server exploded for %s", name)
	}

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))

	var result ToolResultEvent
	for _, ev := range events {
		if e, ok := ev.(ToolResultEvent); ok {
			result = e
		}
	}
	if result.Result.ToolCallID != "call-mcp-2" {
		t.Fatalf("result ToolCallID = %q, want call-mcp-2", result.Result.ToolCallID)
	}
	if !result.Result.IsError {
		t.Fatal("failed MCP call must produce an error result")
	}
	if !strings.Contains(result.Result.Content, "server exploded") {
		t.Fatalf("result content = %q, want failure detail", result.Result.Content)
	}
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2 (error result must still pair)", prov.calls)
	}
}

// MCPINJ-001 gate: Bash and an MCP call issued in the same assistant turn
// both pair into the next request (tool-result pairing across mixed tools).
func TestProviderToolsLoopPairsBashAndMCPCallsInOneTurn(t *testing.T) {
	bashInput, err := json.Marshal(map[string]string{"cmd": "printf paired-bash"})
	if err != nil {
		t.Fatal(err)
	}
	mcpInput, err := json.Marshal(map[string]string{"message": "paired-mcp"})
	if err != nil {
		t.Fatal(err)
	}
	engine, prov := newProviderToolsEngine(t, []model.Response{
		{
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "call-bash", Name: "Bash", Input: bashInput},
				model.ToolCallPart{ID: "call-mcp", Name: "mcp__test-server__echo", Input: mcpInput},
			},
			StopReason: model.StopToolUse,
		},
		{Content: []model.ContentPart{model.TextPart{Text: "done"}}, StopReason: model.StopEndTurn},
	}, EngineConfig{})

	engine.config.MCPToolDefs = func(_ context.Context) []model.ToolDef {
		return []model.ToolDef{mcpEchoToolDef("mcp__test-server__echo")}
	}
	engine.config.MCPCallTool = func(_ context.Context, _ string, _ json.RawMessage) (string, error) {
		return "echo:paired-mcp", nil
	}

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))

	var bashResult, mcpResult string
	for _, ev := range events {
		if e, ok := ev.(ToolResultEvent); ok {
			if strings.Contains(e.Result.Content, "paired-bash") {
				bashResult = e.Result.Content
			}
			if strings.Contains(e.Result.Content, "echo:paired-mcp") {
				mcpResult = e.Result.Content
			}
		}
	}
	if bashResult == "" {
		t.Fatal("missing Bash result")
	}
	if mcpResult == "" {
		t.Fatal("missing MCP result")
	}
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2 (both results must pair for request 2)", prov.calls)
	}
	if err := validateToolResultPairing(prov.requests[1].Messages); err != nil {
		t.Fatalf("pairing validation failed on request 2 messages: %v", err)
	}
}

// Unit gates for the merge: collisions and duplicates.
func TestWithMCPToolDefsMergeRules(t *testing.T) {
	engine := &Engine{}
	tools := []model.ToolDef{{Name: "Bash"}, {Name: "apply_patch"}}

	out := engine.withMCPToolDefs(t.Context(), tools)
	if len(out) != 2 {
		t.Fatalf("no hook: tools = %d, want unchanged 2", len(out))
	}

	engine.config.MCPToolDefs = func(_ context.Context) []model.ToolDef { return nil }
	out = engine.withMCPToolDefs(t.Context(), tools)
	if len(out) != 2 {
		t.Fatalf("empty defs: tools = %d, want unchanged 2", len(out))
	}

	engine.config.MCPToolDefs = func(_ context.Context) []model.ToolDef {
		return []model.ToolDef{
			{Name: "Bash"}, // must not shadow the built-in
			{Name: "mcp__a__dup"},
			{Name: "mcp__a__dup"}, // duplicate must appear once
			{Name: "mcp__b__other"},
		}
	}
	out = engine.withMCPToolDefs(t.Context(), tools)
	want := []string{"Bash", "apply_patch", "mcp__a__dup", "mcp__b__other"}
	if fmt.Sprint(toolNamesOf(out)) != fmt.Sprint(want) {
		t.Fatalf("merged tools = %v, want %v", toolNamesOf(out), want)
	}
}

// The pragma loop mode never injects provider tools; guard the boundary.
func TestPragmaLoopModeDoesNotInjectMCPTools(t *testing.T) {
	engine, prov := newProviderToolsEngine(t, []model.Response{
		{Content: []model.ContentPart{model.TextPart{Text: "PRAGMA_MODE_OK"}}, StopReason: model.StopEndTurn},
	}, EngineConfig{})
	engine.config.LoopMode = LoopModePragma
	engine.config.MCPToolDefs = func(_ context.Context) []model.ToolDef {
		return []model.ToolDef{mcpEchoToolDef("mcp__test-server__echo")}
	}
	engine.config.MCPCallTool = func(_ context.Context, _ string, _ json.RawMessage) (string, error) {
		t.Fatal("pragma loop must not route MCP tool calls")
		return "", nil
	}

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "say ok"))
	for _, ev := range events {
		if e, ok := ev.(ErrorEvent); ok {
			t.Fatalf("unexpected ErrorEvent: %v", e.Err)
		}
	}
	if prov.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", prov.calls)
	}
	if len(prov.requests[0].Tools) != 0 {
		t.Fatalf("pragma mode sent tools: %v", toolNamesOf(prov.requests[0].Tools))
	}
}

