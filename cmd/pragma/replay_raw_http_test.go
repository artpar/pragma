package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/cli"
)

func TestResolveRawHTTPCaptureDir(t *testing.T) {
	root := t.TempDir()
	runDir := filepath.Join(root, "run")
	captureDir := filepath.Join(runDir, "raw-http-pragma")
	turnDir := filepath.Join(captureDir, "000001-abc")
	if err := os.MkdirAll(turnDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := resolveRawHTTPCaptureDir(runDir)
	if err != nil {
		t.Fatalf("resolve run dir: %v", err)
	}
	if got != captureDir {
		t.Fatalf("resolve run dir = %q, want %q", got, captureDir)
	}

	got, err = resolveRawHTTPCaptureDir(captureDir)
	if err != nil {
		t.Fatalf("resolve capture dir: %v", err)
	}
	if got != captureDir {
		t.Fatalf("resolve capture dir = %q, want %q", got, captureDir)
	}
}

func TestDumpRawHTTPCapturesPlainOutput(t *testing.T) {
	root := t.TempDir()
	captureDir := filepath.Join(root, "raw-http-pragma")
	writeCaptureTurn(t, captureDir, "000001-abc", `{
		"model": "test-model",
		"messages": [
			{"role": "system", "content": "system prompt"},
			{"role": "user", "content": "hello"}
		]
	}`, `{
		"choices": [
			{"message": {"content": "world"}, "finish_reason": "stop"}
		]
	}`, `{"sequence":1,"started_at":"2026-06-02T00:00:00Z"}`, `{"sequence":1,"status_code":200,"started_at":"2026-06-02T00:00:01Z","completed_at":"2026-06-02T00:00:02Z"}`)
	writeCaptureTurn(t, captureDir, "000002-def", `{"messages":[]}`, `data: [DONE]`, `{"sequence":2}`, `{"sequence":2,"status_code":200}`)

	outDir := filepath.Join(root, "dump")
	if err := dumpRawHTTPCaptures(captureDir, outDir, false); err != nil {
		t.Fatalf("dump: %v", err)
	}

	assertFileContains(t, filepath.Join(outDir, "README.md"), "plain dump")
	assertFileContains(t, filepath.Join(outDir, "README.md"), "does not infer")
	assertFileContains(t, filepath.Join(outDir, "index.tsv"), "000001-abc")
	assertFileContains(t, filepath.Join(outDir, "index.tsv"), "turn-000001/request.json")
	assertFileContains(t, filepath.Join(outDir, "turn-000001", "request_messages.md"), "## SYSTEM")
	assertFileContains(t, filepath.Join(outDir, "turn-000001", "response_content.md"), "world")

	if _, err := os.Stat(filepath.Join(outDir, "turn-000001", "response.json")); err != nil {
		t.Fatalf("response.json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "turn-000002", "response.raw")); err != nil {
		t.Fatalf("response.raw missing: %v", err)
	}
	for _, name := range []string{
		"turn-000001_contract_analyst",
		"turn-000001_architect",
		"turn-000001_unknown",
	} {
		if _, err := os.Stat(filepath.Join(outDir, name)); !os.IsNotExist(err) {
			t.Fatalf("found inferred directory %s", name)
		}
	}
}

func TestDumpRawHTTPCapturesRequiresOverwrite(t *testing.T) {
	root := t.TempDir()
	captureDir := filepath.Join(root, "raw-http-pragma")
	writeCaptureTurn(t, captureDir, "000001-abc", `{"messages":[]}`, `{"choices":[]}`, `{"sequence":1}`, `{"sequence":1}`)
	outDir := filepath.Join(root, "dump")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	err := dumpRawHTTPCaptures(captureDir, outDir, false)
	if err == nil || !strings.Contains(err.Error(), "--overwrite") {
		t.Fatalf("dump without overwrite err = %v, want --overwrite error", err)
	}
	if err := dumpRawHTTPCaptures(captureDir, outDir, true); err != nil {
		t.Fatalf("dump with overwrite: %v", err)
	}
}

func TestReplayRawHTTPProviderModelAndOut(t *testing.T) {
	root := t.TempDir()
	turnDir := filepath.Join(root, "turn-000001")
	outPath := filepath.Join(root, "live-response.json")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openai/v1/chat/completions" {
			t.Fatalf("path = %q, want /openai/v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer flag-key" {
			t.Fatalf("Authorization = %q, want Bearer flag-key", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got := payload["model"]; got != "override-model" {
			t.Fatalf("model = %q, want override-model", got)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	writeCaptureTurn(t, root, "turn-000001", `{"model":"captured-model","messages":[]}`, `{"choices":[]}`,
		`{"sequence":1,"method":"POST","url":"https://api.getlilac.com/v1/chat/completions"}`,
		`{"sequence":1}`)

	stdout, stderr := executeRawHTTPReplay(t,
		"--provider", "groq",
		"--model", "override-model",
		"--api-key", "flag-key",
		"replay", "raw-http",
		"--base-url", server.URL+"/openai/v1",
		"--out", outPath,
		turnDir,
	)
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty when --out is set", stdout)
	}
	if !strings.Contains(stderr, "HTTP 200 OK") {
		t.Fatalf("stderr missing status: %q", stderr)
	}
	assertFileContains(t, outPath, "ok")
}

func TestReplayRawHTTPFormatRawDefault(t *testing.T) {
	root := t.TempDir()
	turnDir := filepath.Join(root, "turn-000001")
	response := `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	writeCaptureTurn(t, root, "turn-000001", `{"model":"captured-model","messages":[]}`, `{"choices":[]}`,
		`{"sequence":1,"method":"POST","url":"https://api.getlilac.com/v1/chat/completions"}`,
		`{"sequence":1}`)

	stdout, stderr := executeRawHTTPReplay(t,
		"--provider", "lilac",
		"--api-key", "flag-key",
		"replay", "raw-http",
		"--base-url", server.URL+"/v1",
		turnDir,
	)
	if strings.TrimSpace(stdout) != response {
		t.Fatalf("stdout = %q, want raw response %q", stdout, response)
	}
	if !strings.Contains(stderr, `summary: finish_reason="stop" tool_calls=0 text_prefix="ok"`) {
		t.Fatalf("stderr missing raw summary: %q", stderr)
	}
}

func TestReplayRawHTTPFormatContent(t *testing.T) {
	root := t.TempDir()
	turnDir := filepath.Join(root, "turn-000001")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"first"},"finish_reason":"stop"},{"message":{"content":"second"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	writeCaptureTurn(t, root, "turn-000001", `{"model":"captured-model","messages":[]}`, `{"choices":[]}`,
		`{"sequence":1,"method":"POST","url":"https://api.getlilac.com/v1/chat/completions"}`,
		`{"sequence":1}`)

	stdout, stderr := executeRawHTTPReplay(t,
		"--provider", "lilac",
		"--api-key", "flag-key",
		"replay", "raw-http",
		"--base-url", server.URL+"/v1",
		"--format", "content",
		turnDir,
	)
	if stdout != "first\nsecond\n" {
		t.Fatalf("stdout = %q, want content only", stdout)
	}
	if strings.Contains(stderr, "summary:") {
		t.Fatalf("stderr contains summary for content format: %q", stderr)
	}
}

func TestReplayRawHTTPFormatPrettyNoToolCalls(t *testing.T) {
	root := t.TempDir()
	turnDir := filepath.Join(root, "turn-000001")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"model":"test-model",
			"usage":{
				"prompt_tokens":10,
				"prompt_tokens_details":{"cached_tokens":8},
				"completion_tokens":3,
				"total_tokens":13,
				"completion_tokens_details":{"reasoning_tokens":2}
			},
			"choices":[{"message":{"content":"\n\nhello"},"finish_reason":"stop"}]
		}`))
	}))
	defer server.Close()

	writeCaptureTurn(t, root, "turn-000001", `{"model":"captured-model","messages":[]}`, `{"choices":[]}`,
		`{"sequence":1,"method":"POST","url":"https://api.getlilac.com/v1/chat/completions"}`,
		`{"sequence":1}`)

	stdout, stderr := executeRawHTTPReplay(t,
		"--provider", "lilac",
		"--api-key", "flag-key",
		"replay", "raw-http",
		"--base-url", server.URL+"/v1",
		"--format", "pretty",
		turnDir,
	)
	for _, want := range []string{
		"# Raw HTTP Replay Response",
		"HTTP: 200 OK",
		"Model: test-model",
		"Finish reason: stop",
		"Usage: prompt=10 cached=8 completion=3 total=13 reasoning=2",
		"## Assistant Content\n\nhello",
		"## Tool Calls\n\nnone",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("pretty output missing %q\n%s", want, stdout)
		}
	}
	if strings.Contains(stderr, "summary:") {
		t.Fatalf("stderr contains summary for pretty format: %q", stderr)
	}
}

func TestReplayRawHTTPPrettyFlagAlias(t *testing.T) {
	root := t.TempDir()
	turnDir := filepath.Join(root, "turn-000001")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"model":"test-model","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	writeCaptureTurn(t, root, "turn-000001", `{"model":"captured-model","messages":[]}`, `{"choices":[]}`,
		`{"sequence":1,"method":"POST","url":"https://api.getlilac.com/v1/chat/completions"}`,
		`{"sequence":1}`)

	stdout, stderr := executeRawHTTPReplay(t,
		"--provider", "lilac",
		"--api-key", "flag-key",
		"replay", "raw-http",
		"--base-url", server.URL+"/v1",
		"--pretty",
		turnDir,
	)
	if !strings.Contains(stdout, "# Raw HTTP Replay Response") {
		t.Fatalf("stdout missing pretty report: %q", stdout)
	}
	if strings.Contains(stderr, "summary:") {
		t.Fatalf("stderr contains summary for --pretty: %q", stderr)
	}
}

func TestReplayRawHTTPFormatPrettyToolCalls(t *testing.T) {
	root := t.TempDir()
	turnDir := filepath.Join(root, "turn-000001")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"model":"test-model",
			"choices":[{
				"finish_reason":"tool_calls",
				"message":{
					"content":"",
					"tool_calls":[{
						"id":"call_1",
						"type":"function",
						"function":{"name":"lookup","arguments":"{\"query\":\"abc\",\"limit\":2}"}
					}]
				}
			}]
		}`))
	}))
	defer server.Close()

	writeCaptureTurn(t, root, "turn-000001", `{"model":"captured-model","messages":[]}`, `{"choices":[]}`,
		`{"sequence":1,"method":"POST","url":"https://api.getlilac.com/v1/chat/completions"}`,
		`{"sequence":1}`)

	stdout, _ := executeRawHTTPReplay(t,
		"--provider", "lilac",
		"--api-key", "flag-key",
		"replay", "raw-http",
		"--base-url", server.URL+"/v1",
		"--format", "pretty",
		turnDir,
	)
	for _, want := range []string{
		"### Tool Call 1",
		"ID: call_1",
		"Type: function",
		"Function: lookup",
		"```json",
		`"query": "abc"`,
		`"limit": 2`,
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("pretty tool output missing %q\n%s", want, stdout)
		}
	}
}

func TestReplayRawHTTPFormatOutWritesSelectedFormat(t *testing.T) {
	root := t.TempDir()
	turnDir := filepath.Join(root, "turn-000001")
	outPath := filepath.Join(root, "pretty.md")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"model":"test-model","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	writeCaptureTurn(t, root, "turn-000001", `{"model":"captured-model","messages":[]}`, `{"choices":[]}`,
		`{"sequence":1,"method":"POST","url":"https://api.getlilac.com/v1/chat/completions"}`,
		`{"sequence":1}`)

	stdout, _ := executeRawHTTPReplay(t,
		"--provider", "lilac",
		"--api-key", "flag-key",
		"replay", "raw-http",
		"--base-url", server.URL+"/v1",
		"--format", "pretty",
		"--out", outPath,
		turnDir,
	)
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty with --out", stdout)
	}
	assertFileContains(t, outPath, "# Raw HTTP Replay Response")
	assertFileContains(t, outPath, "## Assistant Content\n\nok")
}

func TestReplayRawHTTPFormatRejectsUnknown(t *testing.T) {
	root := t.TempDir()
	turnDir := filepath.Join(root, "turn-000001")
	writeCaptureTurn(t, root, "turn-000001", `{"model":"captured-model","messages":[]}`, `{"choices":[]}`,
		`{"sequence":1,"method":"POST","url":"https://api.getlilac.com/v1/chat/completions"}`,
		`{"sequence":1}`)

	_, _, err := executeRawHTTPReplayWithError(
		"--provider", "lilac",
		"--api-key", "flag-key",
		"replay", "raw-http",
		"--format", "yaml",
		turnDir,
	)
	if err == nil {
		t.Fatal("replay succeeded, want invalid format error")
	}
	if !strings.Contains(err.Error(), `unsupported raw HTTP replay format "yaml"`) {
		t.Fatalf("invalid format error = %v", err)
	}
}

func TestReplayRawHTTPCredentialsFile(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LILAC_API_KEY", "")
	t.Chdir(work)
	writeReplayCredentials(t, home, `providers:
  lilac:
    api_key: creds-key
`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer creds-key" {
			t.Fatalf("Authorization = %q, want Bearer creds-key", got)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	turnDir := filepath.Join(work, "turn-000001")
	writeCaptureTurn(t, work, "turn-000001", `{"model":"captured-model","messages":[]}`, `{"choices":[]}`,
		`{"sequence":1,"method":"POST","url":"https://api.getlilac.com/v1/chat/completions"}`,
		`{"sequence":1}`)

	stdout, stderr := executeRawHTTPReplay(t,
		"replay", "raw-http",
		"--base-url", server.URL+"/v1",
		turnDir,
	)
	if !strings.Contains(stdout, "ok") {
		t.Fatalf("stdout missing response: %q", stdout)
	}
	if !strings.Contains(stderr, "HTTP 200 OK") {
		t.Fatalf("stderr missing status: %q", stderr)
	}
}

func TestReplayRawHTTPTimeoutFlag(t *testing.T) {
	root := t.TempDir()
	turnDir := filepath.Join(root, "turn-000001")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"late"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	writeCaptureTurn(t, root, "turn-000001", `{"model":"captured-model","messages":[]}`, `{"choices":[]}`,
		`{"sequence":1,"method":"POST","url":"https://api.getlilac.com/v1/chat/completions"}`,
		`{"sequence":1}`)

	_, _, err := executeRawHTTPReplayWithError(
		"--provider", "lilac",
		"--api-key", "flag-key",
		"replay", "raw-http",
		"--base-url", server.URL+"/v1",
		"--timeout", "1ms",
		turnDir,
	)
	if err == nil {
		t.Fatal("replay succeeded, want timeout error")
	}
	if !strings.Contains(err.Error(), "Client.Timeout") && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestReplayRawHTTPAuditReportsEvidenceState(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "batch", "good")
	missing := filepath.Join(root, "batch", "missing")
	empty := filepath.Join(root, "batch", "empty")
	for _, dir := range []string{good, missing, empty} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "request.json"), []byte(`{"messages":[]}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(good, "response.raw"), []byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(empty, "response.raw"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr := executeRawHTTPReplay(t,
		"replay", "raw-http", "audit", filepath.Join(root, "batch"),
	)
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	for _, want := range []string{
		"case\trequest_bytes\tresponse_bytes\tstatus\tsummary",
		"/good\t15\t65\tresponse_ok",
		"/missing\t15\t0\tmissing_response",
		"/empty\t15\t0\tempty_response",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("audit output missing %q\n%s", want, stdout)
		}
	}
}

func TestReplayRawHTTPAuditRequireResponses(t *testing.T) {
	root := t.TempDir()
	caseDir := filepath.Join(root, "case")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, "request.json"), []byte(`{"messages":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := executeRawHTTPReplayWithError(
		"replay", "raw-http", "audit", "--require-responses", root,
	)
	if err == nil {
		t.Fatal("audit succeeded, want missing response error")
	}
	if !strings.Contains(err.Error(), "lack usable response evidence") {
		t.Fatalf("audit error = %v", err)
	}
}

func writeCaptureTurn(t *testing.T, captureDir, name, request, response, requestMeta, responseMeta string) {
	t.Helper()
	turnDir := filepath.Join(captureDir, name)
	if err := os.MkdirAll(turnDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"request.json":       request,
		"response.raw":       response,
		"request.meta.json":  requestMeta,
		"response.meta.json": responseMeta,
	}
	for file, content := range files {
		if err := os.WriteFile(filepath.Join(turnDir, file), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("%s does not contain %q\n%s", path, want, string(data))
	}
}

func executeRawHTTPReplay(t *testing.T, args ...string) (string, string) {
	t.Helper()
	stdout, stderr, err := executeRawHTTPReplayWithError(args...)
	if err != nil {
		t.Fatalf("Execute(%v): %v\nstderr:\n%s", args, err, stderr)
	}
	return stdout, stderr
}

func executeRawHTTPReplayWithError(args ...string) (string, string, error) {
	root := &cobra.Command{Use: "pragma"}
	cli.RegisterFlags(root)
	root.AddCommand(replayCmd())
	root.SetArgs(args)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func writeReplayCredentials(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".pragma")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "credentials.yml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
