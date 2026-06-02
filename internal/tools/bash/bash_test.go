package bash

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestBashTool_BackgroundChildDoesNotHoldToolOpen(t *testing.T) {
	tool := &Tool{}
	input, _ := json.Marshal(BashInput{Command: "sh -c 'sleep 2; echo late' & echo ready"})

	start := time.Now()
	result, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("command waited for background child: %s", elapsed)
	}
	if !strings.Contains(result.Content, "ready") {
		t.Fatalf("expected foreground output, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "Console:") {
		t.Fatalf("expected log paths for backgrounding command, got %q", result.Content)
	}
}

func TestBashTool_AndOrBackgroundJobDoesNotHoldToolOpen(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\nsleep 2\necho late\n"), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	tool := &Tool{}
	command := "chmod +x run.sh && ./run.sh &\nsleep 0.1\necho ready"
	input, _ := json.Marshal(BashInput{Command: command})

	start := time.Now()
	result, err := tool.Invoke(context.Background(), input, testState{dir})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("command waited for background and-or job: %s", elapsed)
	}
	if !strings.Contains(result.Content, "ready") {
		t.Fatalf("expected foreground output, got %q", result.Content)
	}
}

func TestBashTool_SoftWaitReturnsRunningCommand(t *testing.T) {
	oldWait := foregroundWait
	foregroundWait = 100 * time.Millisecond
	t.Cleanup(func() { foregroundWait = oldWait })

	tool := &Tool{}
	timeout := 2_000
	input, _ := json.Marshal(BashInput{Command: "echo before; sleep 1; echo done", Timeout: &timeout})

	start := time.Now()
	result, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("soft wait blocked too long: %s", elapsed)
	}
	for _, want := range []string{"Console tail (last 100 lines):", "before", "Command is still running", "Active processes:", "Status:", "Console:"} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("result missing %q: %s", want, result.Content)
		}
	}

	statusPath := fieldPath(result.Content, "Status:")
	consolePath := fieldPath(result.Content, "Console:")
	if statusPath == "" || consolePath == "" {
		t.Fatalf("missing paths in result: %s", result.Content)
	}

	var status string
	for i := 0; i < 30; i++ {
		data, _ := os.ReadFile(statusPath)
		status = string(data)
		if strings.Contains(status, "exited") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(status, "exited") || !strings.Contains(status, "exit_code=0") {
		t.Fatalf("status = %q", status)
	}
	console, err := os.ReadFile(consolePath)
	if err != nil {
		t.Fatalf("read console: %v", err)
	}
	if !strings.Contains(string(console), "done") {
		t.Fatalf("console = %q, want command to complete after soft wait", console)
	}
}

func TestBashTool_ExplicitBackgroundReturnsStatusAndLogs(t *testing.T) {
	tool := &Tool{}
	input, _ := json.Marshal(BashInput{Command: "sleep 0.1; echo done", Background: true})

	start := time.Now()
	result, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("background command blocked: %s", elapsed)
	}
	for _, want := range []string{"Started background command", "Status:", "Console:"} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("result missing %q: %s", want, result.Content)
		}
	}

	statusPath := fieldPath(result.Content, "Status:")
	consolePath := fieldPath(result.Content, "Console:")
	if statusPath == "" || consolePath == "" {
		t.Fatalf("missing paths in result: %s", result.Content)
	}

	var status string
	for i := 0; i < 20; i++ {
		data, _ := os.ReadFile(statusPath)
		status = string(data)
		if strings.Contains(status, "exited") {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !strings.Contains(status, "exited") || !strings.Contains(status, "exit_code=0") {
		t.Fatalf("status = %q", status)
	}
	console, err := os.ReadFile(consolePath)
	if err != nil {
		t.Fatalf("read console log: %v", err)
	}
	if strings.TrimSpace(string(console)) != "done" {
		t.Fatalf("console log = %q", console)
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

func fieldPath(content, prefix string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}
