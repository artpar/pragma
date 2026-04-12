package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	timeout := cmd.Timeout
	if timeout <= 0 {
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
		if cmdCtx.Err() == context.DeadlineExceeded {
			result.Err = fmt.Errorf("hook timed out after %ds", timeout)
			return result
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Err = fmt.Errorf("hook execution failed: %w", err)
			return result
		}
	}

	// Try to parse stdout as JSON
	trimmed := strings.TrimSpace(result.Stdout)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var jsonOut JSONOutput
		if err := json.Unmarshal([]byte(trimmed), &jsonOut); err == nil {
			result.JSON = &jsonOut
		}
		// If JSON parse fails, leave JSON as nil — treat stdout as plain text
	}

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
	env := os.Environ()
	for k, v := range vars {
		env = append(env, k+"="+v)
	}
	return env
}
