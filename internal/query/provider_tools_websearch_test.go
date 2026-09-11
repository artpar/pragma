package query

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/tools/websearch"
)

// WEB-001 loop gates: injection, dispatch, pairing, and keyless parity.

func TestProviderToolsLoopInjectsWebSearchTool(t *testing.T) {
	engine, prov := newProviderToolsEngine(t, []model.Response{
		{Content: []model.ContentPart{model.TextPart{Text: "done"}}, StopReason: model.StopEndTurn},
	}, EngineConfig{})
	engine.config.WebSearch = func(_ context.Context, _ json.RawMessage) (string, error) {
		t.Fatal("no call expected")
		return "", nil
	}

	collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))

	if len(prov.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(prov.requests))
	}
	names := toolNames(toolNamesOf(prov.requests[0].Tools))
	if !contains(names, "Bash") || !contains(names, "apply_patch") {
		t.Fatalf("built-ins missing: %v", names)
	}
	if !contains(names, "WebSearch") {
		t.Fatalf("WebSearch missing: %v", names)
	}
	if names.index("WebSearch") <= names.index("apply_patch") {
		t.Fatalf("WebSearch must follow the built-ins: %v", names)
	}
}

type toolNames []string

func (n toolNames) index(name string) int {
	for i, s := range n {
		if s == name {
			return i
		}
	}
	return -1
}

func TestProviderToolsLoopWithoutWebSearchHookUnchanged(t *testing.T) {
	engine, prov := newProviderToolsEngine(t, []model.Response{
		{Content: []model.ContentPart{model.TextPart{Text: "done"}}, StopReason: model.StopEndTurn},
	}, EngineConfig{})

	collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))

	names := toolNamesOf(prov.requests[0].Tools)
	if len(names) != 2 || !contains(names, "Bash") || !contains(names, "apply_patch") {
		t.Fatalf("tools = %v, want exactly built-ins when no key resolves", names)
	}
}

func TestProviderToolsLoopExecutesWebSearchCall(t *testing.T) {
	callInput, err := json.Marshal(map[string]string{"query": "pragma harness"})
	if err != nil {
		t.Fatal(err)
	}
	engine, prov := newProviderToolsEngine(t, []model.Response{
		{
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "call-ws-1", Name: websearch.ToolName, Input: callInput},
			},
			StopReason: model.StopToolUse,
		},
		{Content: []model.ContentPart{model.TextPart{Text: "done"}}, StopReason: model.StopEndTurn},
	}, EngineConfig{})

	var gotInput json.RawMessage
	engine.config.WebSearch = func(_ context.Context, input json.RawMessage) (string, error) {
		gotInput = input
		return "Web search results for query: \"pragma harness\"\n\n1. Result\n   https://example.com\n", nil
	}

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))

	if string(gotInput) != string(callInput) {
		t.Fatalf("input routed = %s, want %s", gotInput, callInput)
	}
	var resultEvent ToolResultEvent
	for _, ev := range events {
		if e, ok := ev.(ToolResultEvent); ok {
			resultEvent = e
		}
	}
	if resultEvent.Result.ToolCallID != "call-ws-1" {
		t.Fatalf("ToolCallID = %q", resultEvent.Result.ToolCallID)
	}
	if !strings.Contains(resultEvent.Result.Content, "Web search results") || resultEvent.Result.IsError {
		t.Fatalf("result = %+v", resultEvent.Result)
	}
	if prov.calls != 2 {
		t.Fatalf("calls = %d, want 2 (result must pair for request 2)", prov.calls)
	}
}

func TestProviderToolsLoopWebSearchFailurePairsErrorResult(t *testing.T) {
	callInput, _ := json.Marshal(map[string]string{"query": "boom"})
	engine, prov := newProviderToolsEngine(t, []model.Response{
		{
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "call-ws-2", Name: websearch.ToolName, Input: callInput},
			},
			StopReason: model.StopToolUse,
		},
		{Content: []model.ContentPart{model.TextPart{Text: "done"}}, StopReason: model.StopEndTurn},
	}, EngineConfig{})
	engine.config.WebSearch = func(_ context.Context, _ json.RawMessage) (string, error) {
		return "", context.DeadlineExceeded
	}

	events := collectPragmaLoopEvents(engine.Run(t.Context(), "hello"))

	var result ToolResultEvent
	for _, ev := range events {
		if e, ok := ev.(ToolResultEvent); ok {
			result = e
		}
	}
	if !result.Result.IsError {
		t.Fatal("failed WebSearch must produce an error result")
	}
	if !strings.Contains(result.Result.Content, "WebSearch failed") {
		t.Fatalf("failure content = %q", result.Result.Content)
	}
	if prov.calls != 2 {
		t.Fatalf("calls = %d, want 2 (error result must still pair)", prov.calls)
	}
}
