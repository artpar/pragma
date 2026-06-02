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
	root := &cobra.Command{Use: "pragma"}
	cli.RegisterFlags(root)
	root.AddCommand(replayCmd())
	root.SetArgs(args)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(%v): %v\nstderr:\n%s", args, err, stderr.String())
	}
	return stdout.String(), stderr.String()
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
