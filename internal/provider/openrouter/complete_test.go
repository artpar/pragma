package openrouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

func TestCompletePreservesReasoningAcrossToolTurn(t *testing.T) {
	details := json.RawMessage(`[{"type":"reasoning.text","text":"inspect the file","index":0,"format":"unknown"},{"type":"reasoning.encrypted","data":"opaque","index":1}]`)
	var requests []map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			http.Error(w, "bad request", 400)
			return
		}
		requests = append(requests, req)
		w.Header().Set("Content-Type", "application/json")
		message := map[string]any{"role": "assistant", "content": nil, "reasoning": "inspect the file", "reasoning_details": details, "tool_calls": []any{map[string]any{"id": "call_1", "type": "function", "function": map[string]any{"name": "Bash", "arguments": "{\"cmd\":\"pwd\"}"}}}}
		if len(requests) > 1 {
			message = map[string]any{"role": "assistant", "content": "done"}
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"id": "response", "model": DefaultModel, "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	p, err := New("test-key", observe.NewEventBus(32), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	params := provider.RequestParams{Model: DefaultModel, MaxTokens: 16384, Messages: []model.Message{{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "inspect"}}}}}
	response, err := p.Complete(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if response.StopReason != model.StopToolUse {
		t.Fatalf("stop=%s", response.StopReason)
	}
	thinking, ok := response.Content[0].(model.ThinkingPart)
	if !ok || thinking.Text != "inspect the file" {
		t.Fatalf("thinking=%#v", response.Content[0])
	}
	// Persist and restore the assistant contents before the next turn.
	encoded, err := model.MarshalContentParts(response.Content)
	if err != nil {
		t.Fatal(err)
	}
	parts, err := model.UnmarshalContentParts(encoded)
	if err != nil {
		t.Fatal(err)
	}
	params.Messages = append(params.Messages, model.Message{Role: model.RoleAssistant, Content: parts}, model.Message{Role: model.RoleUser, Content: []model.ContentPart{model.ToolResultPart{ToolCallID: "call_1", Content: "/workspace"}}})
	if _, err := p.Complete(context.Background(), params); err != nil {
		t.Fatal(err)
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(requests[1]["messages"], &messages); err != nil {
		t.Fatal(err)
	}
	var gotText string
	if err := json.Unmarshal(messages[1]["reasoning"], &gotText); err != nil {
		t.Fatal(err)
	}
	if gotText != "inspect the file" {
		t.Fatalf("reasoning=%q", gotText)
	}
	var want, got any
	if err := json.Unmarshal(details, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(messages[1]["reasoning_details"], &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("details=%s", messages[1]["reasoning_details"])
	}
}

func TestCompleteRequestMatchesEmbeddedAdapterExceptReasoning(t *testing.T) {
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		requests = append(requests, request)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"r","model":"test","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
	defer server.Close()
	p, err := New("test-key", observe.NewEventBus(32), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	temperature := 0.0
	params := provider.RequestParams{Model: DefaultModel, MaxTokens: 16384, Temperature: &temperature,
		System: model.SystemPrompt{Blocks: []model.SystemBlock{{Text: "system"}}},
		Tools:  []model.ToolDef{{Name: "Bash", Description: "Run bash", InputSchema: json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string"}}}`)}},
		Messages: []model.Message{
			{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hello"}}},
			{Role: model.RoleAssistant, Content: []model.ContentPart{model.ThinkingPart{Text: "think", OpenRouterReasoningDetails: json.RawMessage(`[{"type":"reasoning.text","text":"think"}]`)}, model.ToolCallPart{ID: "c", Name: "Bash", Input: json.RawMessage(`{"cmd":"pwd"}`)}}},
			{Role: model.RoleUser, Content: []model.ContentPart{model.ToolResultPart{ToolCallID: "c", Content: "/app"}}},
		},
	}
	before, err := p.Provider.Complete(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	after, err := p.Complete(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if before.Usage != after.Usage {
		t.Fatalf("usage before=%+v after=%+v", before.Usage, after.Usage)
	}
	for _, raw := range requests[1]["messages"].([]any) {
		m := raw.(map[string]any)
		delete(m, "reasoning")
		delete(m, "reasoning_details")
	}
	if !reflect.DeepEqual(requests[0], requests[1]) {
		t.Fatalf("request changed outside reasoning\nbefore: %#v\nafter: %#v", requests[0], requests[1])
	}
}

func TestReasoningDetailsDeepCopy(t *testing.T) {
	details := json.RawMessage(`[{"type":"reasoning.encrypted","data":"opaque","index":0}]`)
	conv := model.Conversation{Messages: []model.Message{{Role: model.RoleAssistant, Content: []model.ContentPart{model.ThinkingPart{OpenRouterReasoningDetails: details}}}}}
	for _, copy := range []model.Conversation{conv.DeepCopy(), conv.Fork("fork")} {
		part := copy.Messages[0].Content[0].(model.ThinkingPart)
		part.OpenRouterReasoningDetails[0] = 'x'
		if details[0] != '[' {
			t.Fatal("copy aliases original reasoning details")
		}
	}
}

func TestCompleteUsesExistingKeyFallback(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "local-test-fallback")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer local-test-fallback" {
			t.Error("key fallback changed")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	p, err := New("", observe.NewEventBus(16), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	params := provider.RequestParams{Model: DefaultModel, Messages: []model.Message{{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hi"}}}}}
	if _, err := p.Complete(context.Background(), params); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteRejectsInvalidInputBeforeTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid input reached transport")
		w.WriteHeader(400)
	}))
	defer server.Close()
	p, err := New("test-key", observe.NewEventBus(16), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, params := range []provider.RequestParams{{}, {Model: DefaultModel}} {
		if _, err := p.Complete(context.Background(), params); err == nil {
			t.Error("invalid input accepted")
		}
	}
}
