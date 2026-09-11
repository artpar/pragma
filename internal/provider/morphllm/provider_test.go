package morphllm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

func TestMetadata(t *testing.T) {
	p, err := New("test-key", observe.NewEventBus(8), "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "morphllm" {
		t.Fatalf("Name() = %q", p.Name())
	}
	if got, ok := p.ContextWindow(DefaultModel); !ok || got != ContextWindow {
		t.Fatalf("ContextWindow() = (%d, %v)", got, ok)
	}
	if !p.SupportsFeature(provider.FeatureToolUse) || !p.SupportsFeature(provider.FeatureThinking) {
		t.Fatal("Morph provider must advertise tool use and thinking")
	}
	if pricing, ok := p.Pricing(DefaultModel); !ok || pricing.InputPerMToken != 1.25 || pricing.OutputPerMToken != 4.40 {
		t.Fatalf("Pricing() = (%+v, %v)", pricing, ok)
	}
}

func TestOpenAICompatibleToolTurn(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization header missing")
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"r","model":"morph-glm53-744b","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"Bash","arguments":"{\"cmd\":\"pwd\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
	defer server.Close()

	p, err := New("test-key", observe.NewEventBus(16), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response, err := p.Complete(context.Background(), provider.RequestParams{
		Model: DefaultModel, MaxTokens: 4096,
		Messages: []model.Message{{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "inspect"}}}},
		Tools:    []model.ToolDef{{Name: "Bash", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if request["model"] != DefaultModel || request["max_tokens"] != float64(4096) {
		t.Fatalf("request metadata = %#v", request)
	}
	if _, ok := request["stream_options"]; ok {
		t.Fatalf("non-streaming request contains stream_options: %#v", request)
	}
	if response.StopReason != model.StopToolUse || len(response.Content) != 1 {
		t.Fatalf("response = %#v", response)
	}
	call, ok := response.Content[0].(model.ToolCallPart)
	if !ok || call.Name != "Bash" || string(call.Input) != `{"cmd":"pwd"}` {
		t.Fatalf("tool call = %#v", response.Content[0])
	}
}

func TestRecordedReasoningContentIsPreserved(t *testing.T) {
	recorded, err := os.ReadFile("testdata/recorded-reasoning-response.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(recorded)
	}))
	defer server.Close()

	p, err := New("test-key", observe.NewEventBus(16), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response, err := p.Complete(context.Background(), provider.RequestParams{
		Model: DefaultModel,
		Messages: []model.Message{{Role: model.RoleUser, Content: []model.ContentPart{
			model.TextPart{Text: "Reply exactly MORPH_PRAGMA_OK."},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Content) != 1 {
		t.Fatalf("content = %#v, want one thinking part", response.Content)
	}
	thinking, ok := response.Content[0].(model.ThinkingPart)
	if !ok || thinking.Text == "" {
		t.Fatalf("content = %#v, want recorded reasoning", response.Content)
	}
}

func TestReasoningContentIsReplayedAcrossToolTurn(t *testing.T) {
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		requests = append(requests, request)
		w.Header().Set("Content-Type", "application/json")
		if len(requests) == 1 {
			_, _ = w.Write([]byte(`{"model":"morph-glm53-744b","choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":"run the command","tool_calls":[{"id":"call_1","type":"function","function":{"name":"Bash","arguments":"{\"cmd\":\"pwd\"}"}}]},"finish_reason":"tool_calls"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"morph-glm53-744b","choices":[{"index":0,"message":{"role":"assistant","content":"done","reasoning_content":null},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	p, err := New("test-key", observe.NewEventBus(16), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	params := provider.RequestParams{Model: DefaultModel, Messages: []model.Message{{
		Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "inspect"}},
	}}}
	response, err := p.Complete(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	params.Messages = append(params.Messages,
		model.Message{Role: model.RoleAssistant, Content: response.Content},
		model.Message{Role: model.RoleUser, Content: []model.ContentPart{
			model.ToolResultPart{ToolCallID: "call_1", Content: "/workspace"},
		}},
	)
	if _, err := p.Complete(context.Background(), params); err != nil {
		t.Fatal(err)
	}
	messages := requests[1]["messages"].([]any)
	assistant := messages[1].(map[string]any)
	if assistant["reasoning_content"] != "run the command" {
		t.Fatalf("assistant message = %#v", assistant)
	}
}
