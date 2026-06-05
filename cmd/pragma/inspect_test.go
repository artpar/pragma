package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestInspectRawHTTPTimelineMarkdown(t *testing.T) {
	root := t.TempDir()
	writeCaptureTurn(t, root, "000001-aaa", `{"model":"captured-model","messages":[{"role":"user","content":"hello"}]}`, `{"model":"captured-model","usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13},"choices":[{"message":{"content":"done"},"finish_reason":"stop"}]}`,
		`{"sequence":1,"method":"POST","url":"https://api.getlilac.com/v1/chat/completions","started_at":"2026-06-04T08:00:00Z","request_bytes":111}`,
		`{"sequence":1,"status":"200 OK","status_code":200,"started_at":"2026-06-04T08:00:01Z","completed_at":"2026-06-04T08:00:02Z","response_bytes":222}`)
	writeCaptureTurn(t, root, "000002-bbb", `{"model":"captured-model","messages":[],"tools":[{"type":"function"}]}`, `{"choices":[{"message":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
		`{"sequence":2,"method":"POST","url":"https://api.getlilac.com/v1/chat/completions","started_at":"2026-06-04T08:00:03Z"}`,
		`{"sequence":2,"status":"200 OK","status_code":200,"started_at":"2026-06-04T08:00:04Z","completed_at":"2026-06-04T08:00:05Z"}`)

	stdout, stderr, err := executeInspect(t, "inspect", "raw-http", root)
	if err != nil {
		t.Fatalf("inspect: %v\nstderr:\n%s", err, stderr)
	}
	for _, want := range []string{
		"# Raw HTTP Conversation Inspect",
		"Turns: 2",
		"Tool-call turns: 1",
		"Usage: prompt=10 completion=3 total=13",
		"TURN",
		"000001",
		"done",
		"000002",
		"tool calls: lookup",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("inspect output missing %q\n%s", want, stdout)
		}
	}
}

func TestInspectRawHTTPTurnMarkdownSSE(t *testing.T) {
	root := t.TempDir()
	response := strings.Join([]string{
		`data: {"id":"chunk_1","object":"chat.completion.chunk","model":"stream-model","choices":[{"index":0,"delta":{"reasoning":"think ","content":"hel"},"finish_reason":null}]}`,
		"",
		`data: {"choices":[{"index":0,"delta":{"content":"lo","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":4,"total_tokens":9}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	writeCaptureTurn(t, root, "000009-stream", `{"model":"stream-model","messages":[]}`, response,
		`{"sequence":9,"method":"POST","url":"https://api.getlilac.com/v1/chat/completions"}`,
		`{"sequence":9,"status":"200 OK","status_code":200}`)

	stdout, stderr, err := executeInspect(t, "inspect", "raw-http", root, "--turn", "9")
	if err != nil {
		t.Fatalf("inspect turn: %v\nstderr:\n%s", err, stderr)
	}
	for _, want := range []string{
		"# Raw HTTP Turn 000009",
		"Status: 200 OK",
		"Model: stream-model",
		"Finish reason: tool_calls",
		"Usage: prompt=5 completion=4 total=9",
		"## Reasoning\n\nthink",
		"## Assistant Content\n\nhello",
		"- lookup",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("inspect turn output missing %q\n%s", want, stdout)
		}
	}
}

func TestInspectRawHTTPFiltersAndFormats(t *testing.T) {
	root := t.TempDir()
	writeCaptureTurn(t, root, "000001-ok", `{"model":"m","messages":[]}`, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`,
		`{"sequence":1,"method":"POST","url":"https://api.getlilac.com/v1/chat/completions"}`,
		`{"sequence":1,"status":"200 OK","status_code":200}`)
	writeCaptureTurn(t, root, "000002-error", `{"model":"m","messages":[]}`, `{"error":{"message":"bad model"}}`,
		`{"sequence":2,"method":"POST","url":"https://api.getlilac.com/v1/chat/completions"}`,
		`{"sequence":2,"status":"404 Not Found","status_code":404}`)

	stdout, stderr, err := executeInspect(t, "inspect", "raw-http", root, "--errors", "--format", "json")
	if err != nil {
		t.Fatalf("inspect json: %v\nstderr:\n%s", err, stderr)
	}
	if strings.Contains(stdout, `"turn": "000001"`) {
		t.Fatalf("json errors output included ok turn\n%s", stdout)
	}
	for _, want := range []string{`"turn": "000002"`, `"status_code": 404`, `bad model`} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("json errors output missing %q\n%s", want, stdout)
		}
	}
}

func TestInspectRawHTTPDumpDirInput(t *testing.T) {
	root := t.TempDir()
	captureDir := filepath.Join(root, "raw-http-pragma")
	writeCaptureTurn(t, captureDir, "000001-aaa", `{"model":"captured-model","messages":[]}`, `{"choices":[{"message":{"content":"dumped"},"finish_reason":"stop"}]}`,
		`{"sequence":1,"method":"POST","url":"https://api.getlilac.com/v1/chat/completions"}`,
		`{"sequence":1,"status":"200 OK","status_code":200}`)
	outDir := filepath.Join(root, "turn-payloads")
	if err := dumpRawHTTPCaptures(captureDir, outDir, false); err != nil {
		t.Fatalf("dump: %v", err)
	}

	stdout, stderr, err := executeInspect(t, "inspect", "raw-http", outDir, "--format", "tsv")
	if err != nil {
		t.Fatalf("inspect dump: %v\nstderr:\n%s", err, stderr)
	}
	for _, want := range []string{"turn", "000001", "captured-model", "dumped"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("inspect dump output missing %q\n%s", want, stdout)
		}
	}
}

func executeInspect(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := &cobra.Command{Use: "pragma"}
	root.AddCommand(inspectCmd())
	root.SetArgs(args)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}
