package groq

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
)

func TestProvider_Name(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()
	p := New("key", bus)
	if p.Name() != "groq" {
		t.Errorf("Name()=%q, want groq", p.Name())
	}
}

func TestProvider_SupportsFeature(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()
	p := New("key", bus)

	tests := []struct {
		feature provider.Feature
		want    bool
	}{
		{provider.FeatureToolUse, true},
		{provider.FeatureStreaming, true},
		{provider.FeatureImages, true},
		{provider.FeatureThinking, true},
		{provider.FeaturePrefixCaching, false},
	}
	for _, tt := range tests {
		if got := p.SupportsFeature(tt.feature); got != tt.want {
			t.Errorf("SupportsFeature(%q)=%v, want %v", tt.feature, got, tt.want)
		}
	}
}

func TestProvider_Pricing(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()
	p := New("key", bus)

	pricing, ok := p.Pricing("llama-3.3-70b-versatile")
	if !ok {
		t.Fatal("expected known model")
	}
	if pricing.InputPerMToken != 0.59 {
		t.Errorf("input=%v, want 0.59", pricing.InputPerMToken)
	}

	_, ok = p.Pricing("nonexistent")
	if ok {
		t.Error("expected unknown model to return false")
	}
}

func TestProvider_Complete(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	// Mock server returning a simple response
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth header=%q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type=%q", r.Header.Get("Content-Type"))
		}

		resp := wireResponse{
			ID:    "chatcmpl-123",
			Model: "llama-3.3-70b-versatile",
			Choices: []wireChoice{{
				Index:        0,
				Message:      wireMessage{Role: "assistant", Content: "4"},
				FinishReason: "stop",
			}},
			Usage: wireUsage{PromptTokens: 10, CompletionTokens: 1, TotalTokens: 11},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := New("test-key", bus, WithBaseURL(srv.URL))

	params := provider.RequestParams{
		Model:     "llama-3.3-70b-versatile",
		MaxTokens: 100,
		Messages:  []model.Message{{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "2+2=?"}}}},
	}

	result, err := p.Complete(t.Context(), params)
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if result.StopReason != model.StopEndTurn {
		t.Errorf("stop_reason=%q", result.StopReason)
	}
	if len(result.Content) != 1 {
		t.Fatalf("got %d parts", len(result.Content))
	}
	tp, ok := result.Content[0].(model.TextPart)
	if !ok {
		t.Fatalf("part type=%T", result.Content[0])
	}
	if tp.Text != "4" {
		t.Errorf("text=%q", tp.Text)
	}
	if result.Usage.InputTokens != 10 {
		t.Errorf("input_tokens=%d", result.Usage.InputTokens)
	}
}

func TestProvider_Complete_ToolUse(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := wireResponse{
			Model: "llama-3.3-70b-versatile",
			Choices: []wireChoice{{
				Message: wireMessage{
					Role: "assistant",
					ToolCalls: []wireToolCall{{
						ID:   "call_test1",
						Type: "function",
						Function: wireFunction{
							Name:      "bash",
							Arguments: `{"cmd":"ls -la"}`,
						},
					}},
				},
				FinishReason: "tool_calls",
			}},
			Usage: wireUsage{PromptTokens: 20, CompletionTokens: 15},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := New("key", bus, WithBaseURL(srv.URL))
	params := provider.RequestParams{
		Model:     "llama-3.3-70b-versatile",
		MaxTokens: 100,
		Messages:  []model.Message{{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "list files"}}}},
		Tools:     []model.ToolDef{{Name: "bash", Description: "Run cmd", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	}

	result, err := p.Complete(t.Context(), params)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result.StopReason != model.StopToolUse {
		t.Errorf("stop_reason=%q", result.StopReason)
	}
	if len(result.Content) != 1 {
		t.Fatalf("got %d parts", len(result.Content))
	}
	tc, ok := result.Content[0].(model.ToolCallPart)
	if !ok {
		t.Fatalf("part type=%T", result.Content[0])
	}
	if tc.Name != "bash" {
		t.Errorf("tool name=%q", tc.Name)
	}
}

func TestProvider_Stream(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	sseBody := "data: {\"id\":\"1\",\"model\":\"llama\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hi\"}}]}\n\n" +
		"data: {\"id\":\"1\",\"model\":\"llama\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":1,\"total_tokens\":6}}\n\n" +
		"data: [DONE]\n\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte(sseBody))
	}))
	defer srv.Close()

	p := New("key", bus, WithBaseURL(srv.URL), WithIdleTimeout(0))
	params := provider.RequestParams{
		Model:     "llama-3.3-70b-versatile",
		MaxTokens: 100,
		Messages:  []model.Message{{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hey"}}}},
	}

	ch, err := p.Stream(t.Context(), params)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	chunks := drainChunks(ch)
	assertChunkText(t, chunks, "Hi")
	assertDone(t, chunks, model.StopEndTurn)
}

func TestProvider_Complete_Error(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"invalid api key","type":"authentication_error"}}`))
	}))
	defer srv.Close()

	p := New("bad-key", bus, WithBaseURL(srv.URL), WithMaxRetries(0))
	params := provider.RequestParams{
		Model:     "llama-3.3-70b-versatile",
		MaxTokens: 100,
		Messages:  []model.Message{{Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "hi"}}}},
	}

	_, err := p.Complete(t.Context(), params)
	if err == nil {
		t.Fatal("expected error")
	}
}
