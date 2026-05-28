package lilac

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-test",
			"object": "chat.completion",
			"created": 1,
			"model": "minimaxai/minimax-m2.7",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "ok"}, "finish_reason": "stop"}],
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
	resp, err := p.Complete(context.Background(), provider.RequestParams{
		Model: "minimaxai/minimax-m2.7",
		Messages: []model.Message{{
			Role:    model.RoleUser,
			Content: []model.ContentPart{model.TextPart{Text: "hello"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != model.StopEndTurn {
		t.Fatalf("stop reason = %s", resp.StopReason)
	}

	captureDir := onlyRawCaptureDir(t, captureRoot)
	requestBody := readRawCaptureFile(t, filepath.Join(captureDir, "request.json"))
	if !json.Valid([]byte(requestBody)) {
		t.Fatalf("captured request is not JSON: %q", requestBody)
	}
	var headers map[string][]string
	readRawCaptureJSON(t, filepath.Join(captureDir, "request.headers.json"), &headers)
	if got := headers["Authorization"]; len(got) != 1 || got[0] != "<redacted>" {
		t.Fatalf("captured Authorization = %#v", got)
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
