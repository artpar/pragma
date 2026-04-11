package bash

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

const (
	defaultTimeoutMs = 120_000 // 2 minutes
	maxTimeoutMs     = 600_000 // 10 minutes
)

// BashInput defines the parameters for the Bash tool.
type BashInput struct {
	Command     string `json:"command" desc:"The shell command to execute"`
	Timeout     *int   `json:"timeout,omitempty" desc:"Optional timeout in milliseconds (max 600000)"`
	Description string `json:"description,omitempty" desc:"Clear, concise description of what this command does"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["command"],
	"properties": {
		"command": {
			"type": "string",
			"description": "The command to execute"
		},
		"timeout": {
			"type": "number",
			"description": "Optional timeout in milliseconds (max 600000)"
		},
		"description": {
			"type": "string",
			"description": "Clear, concise description of what this command does"
		}
	}
}`)

// Tool implements the Bash tool for shell command execution.
type Tool struct{}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Bash\"")
	return "Bash"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Execute a shell command and return its output.\"")
	return "Execute a shell command and return its output."
}
func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: false, Destructive: true}")
	return tool.ToolFlags{ReadOnly: false, Concurrent: false, Destructive: true}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "bash", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "bash", "Tool.CheckPerm", "exit")
	var in struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.Command == "" {
		observe.TraceCtx(ctx, "bash", "Tool.CheckPerm", "if: err != nil || in.Command == \"\"")
		observe.TraceCtx(ctx, "bash", "Tool.CheckPerm", "return: checker.Check(ctx, \"Bash\", \"\")")
		return checker.Check(ctx, "Bash", "")
	}
	observe.TraceCtx(ctx, "bash", "Tool.CheckPerm", "return: checker.Check(ctx, \"Bash\", in.Command)")
	return checker.Check(ctx, "Bash", in.Command)
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "bash", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "bash", "Tool.Invoke", "exit")
	var in BashInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Command == "" {
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: in.Command == \"\"")
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"command is required\")")
		return tool.InvokeResult{}, fmt.Errorf("command is required")
	}

	timeoutMs := defaultTimeoutMs
	if in.Timeout != nil {
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: in.Timeout != nil")
		timeoutMs = *in.Timeout
		if timeoutMs <= 0 {
			observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: timeoutMs <= 0")
			timeoutMs = defaultTimeoutMs
		}
		if timeoutMs > maxTimeoutMs {
			observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: timeoutMs > maxTimeoutMs")
			timeoutMs = maxTimeoutMs
		}
	}

	timeout := time.Duration(timeoutMs) * time.Millisecond
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "bash", "-c", in.Command)
	cmd.Dir = state.WorkDir()

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	// Capture combined stdout+stderr (merged fd, like TS)
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined

	err := cmd.Run()

	output := strings.TrimRight(combined.String(), "\n")

	output = strings.TrimLeft(output, "\n")

	if err != nil {
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: err != nil")
		if cmdCtx.Err() == context.DeadlineExceeded {
			observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: cmdCtx.Err() == context.DeadlineExceeded")
			if output != "" {
				observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: output != \"\"")
				output += "\n"
			}
			output += fmt.Sprintf("Command timed out after %dms", timeoutMs)
			observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{Content: output}, nil")
			return tool.InvokeResult{Content: output}, nil
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: ok")
			exitCode := exitErr.ExitCode()
			if output != "" {
				observe.TraceCtx(ctx, "bash", "Tool.Invoke", "if: output != \"\"")
				output += "\n"
			}
			output += fmt.Sprintf("Exit code %d", exitCode)
			observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{Content: output}, nil")

			return tool.InvokeResult{Content: output}, nil
		}
		observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"execute command: %w\", err)")

		return tool.InvokeResult{}, fmt.Errorf("execute command: %w", err)
	}
	observe.TraceCtx(ctx, "bash", "Tool.Invoke", "return: tool.InvokeResult{Content: output}, nil")

	return tool.InvokeResult{Content: output}, nil
}
