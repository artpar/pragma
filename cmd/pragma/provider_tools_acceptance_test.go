package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestProviderToolsCLIContract exercises the actual Cobra entrypoint, OpenAI
// wire adapter, provider-tools loop, Bash executor, conversation translation,
// and non-interactive output. The HTTP server acts as a strict provider so a
// protocol regression fails at the request where it occurs.
func TestProviderToolsCLIContract(t *testing.T) {
	var (
		mu       sync.Mutex
		requests []map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, fmt.Sprintf("unexpected path %q", r.URL.Path), http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer harness-key" {
			http.Error(w, fmt.Sprintf("unexpected authorization %q", got), http.StatusUnauthorized)
			return
		}

		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		mu.Lock()
		requests = append(requests, request)
		requestNumber := len(requests)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch requestNumber {
		case 1:
			writeChatCompletion(t, w, "tool_calls", map[string]any{
				"role":    "assistant",
				"content": nil,
				"tool_calls": []any{map[string]any{
					"id":   "call_harness_1",
					"type": "function",
					"function": map[string]any{
						"name":      "Bash",
						"arguments": `{"cmd":"printf harness-tool-ok"}`,
					},
				}},
			})
		case 2:
			writeChatCompletion(t, w, "stop", map[string]any{
				"role":    "assistant",
				"content": "HARNESS_FINAL_OK",
			})
		default:
			http.Error(w, "unexpected extra completion request", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("OPENAI_BASE_URL", server.URL+"/v1")
	workDir := filepath.Join(tempHome, "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDir) })

	stdoutPath := filepath.Join(tempHome, "stdout")
	stderrPath := filepath.Join(tempHome, "stderr")
	stdout := redirectProcessFile(t, &os.Stdout, stdoutPath)
	stderr := redirectProcessFile(t, &os.Stderr, stderrPath)

	root := newRootCommand()
	root.SetArgs([]string{
		"--provider", "openai",
		"--model", "gpt-4o",
		"--api-key", "harness-key",
		"--loop", "provider-tools",
		"--max-turns", "3",
		"--prompt", "Run the harness tool and report completion.",
	})
	err = root.ExecuteContext(context.Background())

	restoreProcessFile(t, &os.Stdout, stdout)
	restoreProcessFile(t, &os.Stderr, stderr)
	stdoutBytes, readErr := os.ReadFile(stdoutPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	stderrBytes, readErr := os.ReadFile(stderrPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err != nil {
		t.Fatalf("pragma command failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdoutBytes, stderrBytes)
	}
	if !strings.Contains(string(stdoutBytes), "HARNESS_FINAL_OK") {
		t.Fatalf("stdout did not contain final answer:\n%s", stdoutBytes)
	}
	logFiles, err := filepath.Glob(filepath.Join(tempHome, ".pragma", "logs", "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(logFiles) != 1 {
		t.Fatalf("log file count = %d, want 1", len(logFiles))
	}
	logBytes, err := os.ReadFile(logFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logBytes), `"kind":"FlowTrace"`) {
		t.Fatalf("normal run emitted function-level FlowTrace events")
	}
	if len(logBytes) > 32*1024 {
		t.Fatalf("two-turn harness log is %d bytes, want <= 32 KiB", len(logBytes))
	}

	mu.Lock()
	gotRequests := append([]map[string]any(nil), requests...)
	mu.Unlock()
	if len(gotRequests) != 2 {
		t.Fatalf("completion request count = %d, want 2", len(gotRequests))
	}
	assertInitialProviderToolsRequest(t, gotRequests[0])
	assertToolResultRequest(t, gotRequests[1])
}

func writeChatCompletion(t *testing.T, w http.ResponseWriter, finishReason string, message map[string]any) {
	t.Helper()
	err := json.NewEncoder(w).Encode(map[string]any{
		"id":      "chatcmpl-harness",
		"object":  "chat.completion",
		"created": 1,
		"model":   "gpt-4o",
		"choices": []any{map[string]any{
			"index":         0,
			"message":       message,
			"finish_reason": finishReason,
		}},
		"usage": map[string]any{
			"prompt_tokens":     10,
			"completion_tokens": 5,
			"total_tokens":      15,
		},
	})
	if err != nil {
		t.Errorf("encode completion: %v", err)
	}
}

func assertInitialProviderToolsRequest(t *testing.T, request map[string]any) {
	t.Helper()
	tools, ok := request["tools"].([]any)
	if !ok {
		t.Fatalf("first request tools = %#v", request["tools"])
	}
	names := make(map[string]bool)
	for _, rawTool := range tools {
		tool, _ := rawTool.(map[string]any)
		function, _ := tool["function"].(map[string]any)
		name, _ := function["name"].(string)
		names[name] = true
	}
	if !names["Bash"] || !names["apply_patch"] {
		t.Fatalf("first request tool names = %#v, want Bash and apply_patch", names)
	}
}

func assertToolResultRequest(t *testing.T, request map[string]any) {
	t.Helper()
	messages, ok := request["messages"].([]any)
	if !ok {
		t.Fatalf("second request messages = %#v", request["messages"])
	}
	var sawAssistantCall, sawToolResult bool
	for _, rawMessage := range messages {
		message, _ := rawMessage.(map[string]any)
		role, _ := message["role"].(string)
		switch role {
		case "assistant":
			calls, _ := message["tool_calls"].([]any)
			for _, rawCall := range calls {
				call, _ := rawCall.(map[string]any)
				if call["id"] == "call_harness_1" {
					sawAssistantCall = true
				}
			}
		case "tool":
			callID, _ := message["tool_call_id"].(string)
			content, _ := message["content"].(string)
			if callID == "call_harness_1" && strings.Contains(content, "harness-tool-ok") {
				sawToolResult = true
			}
		}
	}
	if !sawAssistantCall || !sawToolResult {
		t.Fatalf("second request missing call/result pairing: assistant_call=%v tool_result=%v messages=%#v", sawAssistantCall, sawToolResult, messages)
	}
}

func redirectProcessFile(t *testing.T, target **os.File, path string) *os.File {
	t.Helper()
	original := *target
	replacement, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	*target = replacement
	return original
}

func restoreProcessFile(t *testing.T, target **os.File, original *os.File) {
	t.Helper()
	replacement := *target
	if err := replacement.Close(); err != nil {
		t.Fatal(err)
	}
	*target = original
}
