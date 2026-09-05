package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestInspectRawHTTPDumpDirInput(t *testing.T) {
	root := t.TempDir()
	captureDir := filepath.Join(root, "raw-http-pragma")
	writeCaptureTurn(t, captureDir, "000001-aaa", `{"model":"captured-model","messages":[]}`, `{"choices":[{"message":{"content":"dumped"},"finish_reason":"stop"}]}`,
		`{"sequence":1,"method":"POST","url":"https://openrouter.ai/v1/chat/completions"}`,
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
