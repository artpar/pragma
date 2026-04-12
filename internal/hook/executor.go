package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/artpar/gogent/internal/observe"
	"os"
	"os/exec"
	"strings"
	"time"
)

const defaultTimeoutSec = 600 // 10 minutes, matching TS default

// ExecCommand runs a shell command with JSON input on stdin.
// Returns Result with stdout, stderr, exit code.
// Respects context cancellation and per-hook timeout.
func ExecCommand(ctx context.Context, cmd Command, input []byte, workDir string, envVars map[string]string) Result {
	observe.TraceCtx(ctx, "hook", "ExecCommand", "enter")
	defer observe.TraceCtx(ctx, "hook", "ExecCommand", "exit")
	timeout := cmd.Timeout
	if timeout <= 0 {
		observe.TraceCtx(ctx, "hook", "ExecCommand", "if: timeout <= 0")
		timeout = defaultTimeoutSec
	}
	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	shellCmd := exec.CommandContext(cmdCtx, "bash", "-c", cmd.Command)
	shellCmd.Dir = workDir
	shellCmd.Stdin = bytes.NewReader(input)
	shellCmd.Env = buildEnv(envVars)

	var stdout, stderr bytes.Buffer
	shellCmd.Stdout = &stdout
	shellCmd.Stderr = &stderr

	err := shellCmd.Run()

	result := Result{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		observe.TraceCtx(ctx, "hook", "ExecCommand", "if: err != nil")
		if cmdCtx.Err() == context.DeadlineExceeded {
			observe.TraceCtx(ctx, "hook", "ExecCommand", "if: cmdCtx.Err() == context.DeadlineExceeded")
			result.Err = fmt.Errorf("hook timed out after %ds", timeout)
			observe.TraceCtx(ctx, "hook", "ExecCommand", "return: result")
			return result
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			observe.TraceCtx(ctx, "hook", "ExecCommand", "if: ok")
			result.ExitCode = exitErr.ExitCode()
		} else {
			observe.TraceCtx(ctx, "hook", "ExecCommand", "else: ok")
			result.Err = fmt.Errorf("hook execution failed: %w", err)
			observe.TraceCtx(ctx, "hook", "ExecCommand", "return: result")
			return result
		}
	}

	trimmed := strings.TrimSpace(result.Stdout)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		observe.TraceCtx(ctx, "hook", "ExecCommand", "if: len(trimmed) > 0 && trimmed[0] == '{'")
		var jsonOut JSONOutput
		if err := json.Unmarshal([]byte(trimmed), &jsonOut); err == nil {
			observe.TraceCtx(ctx, "hook", "ExecCommand", "if: err == nil")
			result.JSON = &jsonOut
		}

	}
	observe.TraceCtx(ctx, "hook", "ExecCommand", "return: result")

	return result
}

// buildEnv creates the environment for hook execution.
// Inherits parent env and adds hook-specific variables.
//
// Security note: GOGENT_TOOL_INPUT and GOGENT_TOOL_NAME are passed as-is.
// Hook scripts MUST quote these variables (e.g., "$GOGENT_TOOL_INPUT") to
// prevent shell metacharacter expansion. Sanitizing here would break
// legitimate JSON payloads containing special characters.
func buildEnv(vars map[string]string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	env := os.Environ()
	for k, v := range vars {
		observe.GlobalTrace("range vars")
		env = append(env, k+"="+v)
	}
	observe.GlobalTrace("return: env")
	return env
}
