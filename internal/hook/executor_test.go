package hook

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/bash\n"+content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExecCommandExit0(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "hook.sh", `echo "hello"`)

	result := ExecCommand(context.Background(), Command{Command: script}, nil, dir, nil)
	if result.ExitCode != 0 {
		t.Errorf("exit code: got %d, want 0", result.ExitCode)
	}
	if result.Outcome() != OutcomeOK {
		t.Errorf("outcome: got %s, want ok", result.Outcome())
	}
	if result.Stdout != "hello\n" {
		t.Errorf("stdout: got %q, want %q", result.Stdout, "hello\n")
	}
}

func TestExecCommandExit2Block(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "hook.sh", `echo "blocked" >&2; exit 2`)

	result := ExecCommand(context.Background(), Command{Command: script}, nil, dir, nil)
	if result.ExitCode != 2 {
		t.Errorf("exit code: got %d, want 2", result.ExitCode)
	}
	if result.Outcome() != OutcomeBlock {
		t.Errorf("outcome: got %s, want block", result.Outcome())
	}
	if result.Stderr != "blocked\n" {
		t.Errorf("stderr: got %q", result.Stderr)
	}
}

func TestExecCommandExitOtherError(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "hook.sh", `echo "oops" >&2; exit 1`)

	result := ExecCommand(context.Background(), Command{Command: script}, nil, dir, nil)
	if result.ExitCode != 1 {
		t.Errorf("exit code: got %d, want 1", result.ExitCode)
	}
	if result.Outcome() != OutcomeError {
		t.Errorf("outcome: got %s, want error", result.Outcome())
	}
}

func TestExecCommandJSONOutput(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "hook.sh", `echo '{"decision":"block","reason":"dangerous"}'`)

	result := ExecCommand(context.Background(), Command{Command: script}, nil, dir, nil)
	if result.JSON == nil {
		t.Fatal("expected JSON output")
	}
	if result.JSON.Decision != "block" {
		t.Errorf("decision: got %q, want block", result.JSON.Decision)
	}
	if result.JSON.Reason != "dangerous" {
		t.Errorf("reason: got %q", result.JSON.Reason)
	}
}

func TestExecCommandInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "hook.sh", `echo "not json"`)

	result := ExecCommand(context.Background(), Command{Command: script}, nil, dir, nil)
	if result.JSON != nil {
		t.Error("expected nil JSON for non-JSON output")
	}
	if result.Stdout != "not json\n" {
		t.Errorf("stdout: got %q", result.Stdout)
	}
}

func TestExecCommandStdinInput(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "hook.sh", `cat`)

	input := []byte(`{"tool_name":"Bash"}`)
	result := ExecCommand(context.Background(), Command{Command: script}, input, dir, nil)
	if result.Stdout != `{"tool_name":"Bash"}` {
		t.Errorf("stdout: got %q, want input echoed back", result.Stdout)
	}
}

func TestExecCommandTimeout(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "hook.sh", `sleep 60`)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	result := ExecCommand(ctx, Command{Command: script, Timeout: 1}, nil, dir, nil)
	if result.Outcome() != OutcomeTimeout {
		t.Errorf("outcome: got %s, want timeout", result.Outcome())
	}
}

func TestExecCommandEnvVars(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "hook.sh", `echo "$PRAGMA_TOOL_NAME"`)

	env := map[string]string{"PRAGMA_TOOL_NAME": "Bash"}
	result := ExecCommand(context.Background(), Command{Command: script}, nil, dir, env)
	if result.Stdout != "Bash\n" {
		t.Errorf("stdout: got %q, want Bash", result.Stdout)
	}
}
