package bash

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type testState struct{ dir string }

func (s testState) WorkDir() string { return s.dir }

func TestBashTool_SimpleCommand(t *testing.T) {
	tool := &Tool{}
	input, _ := json.Marshal(BashInput{Command: "echo hello"})

	result, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content != "hello" {
		t.Errorf("expected 'hello', got: %q", result.Content)
	}
}

func TestBashTool_ExitCode(t *testing.T) {
	tool := &Tool{}
	input, _ := json.Marshal(BashInput{Command: "exit 42"})

	result, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v (result should not be a Go error)", err)
	}

	if !strings.Contains(result.Content, "Exit code 42") {
		t.Errorf("expected 'Exit code 42', got: %q", result.Content)
	}
}

func TestBashTool_StderrMerged(t *testing.T) {
	tool := &Tool{}
	input, _ := json.Marshal(BashInput{Command: "echo out; echo err >&2"})

	result, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "out") {
		t.Errorf("expected stdout in output, got: %q", result.Content)
	}
	if !strings.Contains(result.Content, "err") {
		t.Errorf("expected stderr in output, got: %q", result.Content)
	}
}

func TestBashTool_WorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	tool := &Tool{}
	input, _ := json.Marshal(BashInput{Command: "pwd"})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, dir) {
		t.Errorf("expected working dir %s in output, got: %q", dir, result)
	}
}

func TestBashTool_Timeout(t *testing.T) {
	tool := &Tool{}
	timeout := 500 // 500ms
	input, _ := json.Marshal(BashInput{Command: "sleep 60", Timeout: &timeout})

	result, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "timed out") {
		t.Errorf("expected timeout message, got: %q", result.Content)
	}
}

func TestBashTool_TimeoutWithBackgroundChildHoldingPipe(t *testing.T) {
	tool := &Tool{}
	timeout := 500
	input, _ := json.Marshal(BashInput{Command: "sleep 60 & wait", Timeout: &timeout})

	start := time.Now()
	result, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
	if !strings.Contains(result.Content, "timed out") {
		t.Errorf("expected timeout message, got: %q", result.Content)
	}
}

func TestBashTool_MissingCommand(t *testing.T) {
	tool := &Tool{}
	input, _ := json.Marshal(BashInput{})

	_, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestBashTool_CommandNotFound(t *testing.T) {
	tool := &Tool{}
	input, _ := json.Marshal(BashInput{Command: "nonexistent_command_xyzzy_12345"})

	result, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v (should return as content)", err)
	}

	// Should contain exit code (127 for command not found)
	if !strings.Contains(result.Content, "Exit code") {
		t.Errorf("expected exit code in output, got: %q", result.Content)
	}
}

func TestBashTool_MaxTimeout(t *testing.T) {
	tool := &Tool{}
	timeout := 999999 // exceeds max
	input, _ := json.Marshal(BashInput{Command: "echo ok", Timeout: &timeout})

	result, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content != "ok" {
		t.Errorf("expected 'ok', got: %q", result.Content)
	}
}

func TestBashTool_PipedCommand(t *testing.T) {
	tool := &Tool{}
	input, _ := json.Marshal(BashInput{Command: "echo 'hello world' | tr a-z A-Z"})

	result, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content != "HELLO WORLD" {
		t.Errorf("expected 'HELLO WORLD', got: %q", result.Content)
	}
}

func TestBashTool_MultilineOutput(t *testing.T) {
	tool := &Tool{}
	input, _ := json.Marshal(BashInput{Command: "echo line1; echo line2; echo line3"})

	result, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "line1") || !strings.Contains(result.Content, "line3") {
		t.Errorf("expected all lines, got: %q", result.Content)
	}
}

func TestBashToolPatchModeRejectsSourceMutation(t *testing.T) {
	tool := &Tool{PatchMode: true}
	input, _ := json.Marshal(BashInput{Command: "sed -i '' 's/old/new/' main.go"})

	_, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	if err == nil {
		t.Fatal("expected patch mode to reject sed -i")
	}
	if !strings.Contains(err.Error(), "Use apply_patch") {
		t.Fatalf("error = %v, want apply_patch guidance", err)
	}
}

func TestBashToolPatchModeAllowsShellApplyPatch(t *testing.T) {
	dir := t.TempDir()
	tool := &Tool{PatchMode: true}
	input, _ := json.Marshal(BashInput{Command: "apply_patch <<'PATCH'\n*** Begin Patch\n*** Add File: x.txt\n+x\n*** End Patch\nPATCH"})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "Applied patch successfully") {
		t.Fatalf("result = %q", result.Content)
	}
}
