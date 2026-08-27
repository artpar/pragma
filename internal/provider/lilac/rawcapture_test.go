package lilac

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/provider/rawcapture"
)

func TestRawCaptureRecordsAnyLLMWireRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("request body is not JSON: %v", err)
		}
		if payload["model"] != "minimaxai/minimax-m2.7" {
			t.Fatalf("model = %v", payload["model"])
		}
		if _, ok := payload["chat_template_kwargs"]; ok {
			t.Fatal("chat_template_kwargs sent for non-GLM-5.2 model")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-test",
			"object": "chat.completion",
			"created": 1,
			"model": "minimaxai/minimax-m2.7",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "ok", "reasoning": "thinking"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
		}`))
	}))
	defer server.Close()

	captureRoot := t.TempDir()
	t.Setenv(rawcapture.EnvDir, captureRoot)

	p, err := New("test-key", observe.NewEventBus(16), WithBaseURL(server.URL+"/v1"))
	if err != nil {
		t.Fatal(err)
	}
	temp := 0.0
	resp, err := p.Complete(context.Background(), provider.RequestParams{
		Model:       "minimaxai/minimax-m2.7",
		MaxTokens:   16384,
		Temperature: &temp,
		Messages: []model.Message{{
			Role:    model.RoleUser,
			Content: []model.ContentPart{model.TextPart{Text: "hello <tag> a && b > c"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != model.StopEndTurn {
		t.Fatalf("stop reason = %s", resp.StopReason)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("response content length = %d, want 2", len(resp.Content))
	}
	if thinking, ok := resp.Content[0].(model.ThinkingPart); !ok || thinking.Text != "thinking" {
		t.Fatalf("response content[0] = %#v, want thinking part", resp.Content[0])
	}
	if text, ok := resp.Content[1].(model.TextPart); !ok || text.Text != "ok" {
		t.Fatalf("response content[1] = %#v, want text ok", resp.Content[1])
	}

	captureDir := onlyRawCaptureDir(t, captureRoot)
	requestBody := readRawCaptureFile(t, filepath.Join(captureDir, "request.json"))
	if !json.Valid([]byte(requestBody)) {
		t.Fatalf("captured request is not JSON: %q", requestBody)
	}
	var requestPayload map[string]any
	if err := json.Unmarshal([]byte(requestBody), &requestPayload); err != nil {
		t.Fatal(err)
	}
	if got := requestPayload["max_tokens"]; got != float64(16384) {
		t.Fatalf("max_tokens = %v, want 16384", got)
	}
	if got := requestPayload["temperature"]; got != float64(0) {
		t.Fatalf("temperature = %v, want 0", got)
	}
	if !strings.Contains(requestBody, `"temperature":0.0`) {
		t.Fatalf("temperature raw JSON = %q, want 0.0 token", requestBody)
	}
	if _, ok := requestPayload["max_completion_tokens"]; ok {
		t.Fatal("request used max_completion_tokens; Pragma loop sends max_tokens")
	}
	if strings.Contains(requestBody, `\u003c`) || strings.Contains(requestBody, `\u003e`) || strings.Contains(requestBody, `\u0026`) {
		t.Fatalf("request body contains HTML-escaped JSON: %q", requestBody)
	}
	if !strings.Contains(requestBody, "hello <tag> a && b > c") {
		t.Fatalf("request body does not contain literal prompt bytes: %q", requestBody)
	}
	var headers map[string][]string
	readRawCaptureJSON(t, filepath.Join(captureDir, "request.headers.json"), &headers)
	if got := headers["Authorization"]; len(got) != 1 || got[0] != "<redacted>" {
		t.Fatalf("captured Authorization = %#v", got)
	}
}

func TestGLM52RequestPreservesThinking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Model              string `json:"model"`
			ChatTemplateKwargs *struct {
				ClearThinking *bool `json:"clear_thinking"`
			} `json:"chat_template_kwargs"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("request body is not JSON: %v", err)
		}
		if payload.Model != "zai-org/glm-5.2" {
			t.Fatalf("model = %q", payload.Model)
		}
		if payload.ChatTemplateKwargs == nil || payload.ChatTemplateKwargs.ClearThinking == nil {
			t.Fatalf("chat_template_kwargs = %#v", payload.ChatTemplateKwargs)
		}
		if *payload.ChatTemplateKwargs.ClearThinking {
			t.Fatal("clear_thinking = true, want false")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-test",
			"object": "chat.completion",
			"created": 1,
			"model": "zai-org/glm-5.2",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "ok"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
		}`))
	}))
	defer server.Close()

	p, err := New("test-key", observe.NewEventBus(16), WithBaseURL(server.URL+"/v1"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Complete(context.Background(), provider.RequestParams{
		Model:     "zai-org/glm-5.2",
		MaxTokens: 32,
		Messages: []model.Message{{
			Role:    model.RoleUser,
			Content: []model.ContentPart{model.TextPart{Text: "continue"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func onlyRawCaptureDir(t *testing.T, root string) string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("capture entries = %d, want 1", len(entries))
	}
	return filepath.Join(root, entries[0].Name())
}

func readRawCaptureFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func readRawCaptureJSON(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
}
