package powershell

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
)

const (
	defaultTimeoutMs = 120_000 // 2 minutes
	maxTimeoutMs     = 600_000 // 10 minutes
)

// utf8Preamble forces PowerShell to output UTF-8 (#46486: CJK/Big5 encoding fix).
const utf8Preamble = `[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; $OutputEncoding = [System.Text.Encoding]::UTF8; `

// PowerShellInput defines the parameters for the PowerShell tool.
type PowerShellInput struct {
	Command     string `json:"command" desc:"The PowerShell command to execute"`
	Timeout     *int   `json:"timeout,omitempty" desc:"Optional timeout in milliseconds (max 600000)"`
	Description string `json:"description,omitempty" desc:"Clear, concise description of what this command does"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
	"required": ["command"],
	"properties": {
		"command": {
			"type": "string",
			"description": "The PowerShell command to execute"
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

// psDetection caches the detected PowerShell executable path.
var (
	psOnce sync.Once
	psExe  string
	psErr  error
)

// detectPowerShell finds pwsh (cross-platform, PS Core 7+) or powershell.exe (Windows 5.1).
// Result is cached after first call. Issue #45963: pwsh runs on macOS/Linux too.
func detectPowerShell() (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	psOnce.Do(func() {

		if path, err := exec.LookPath("pwsh"); err == nil {
			psExe = path
			return
		}

		if runtime.GOOS == "windows" {
			if path, err := exec.LookPath("powershell.exe"); err == nil {
				psExe = path
				return
			}
		}
		psErr = fmt.Errorf("PowerShell not found on this system (install pwsh: https://aka.ms/powershell)")
	})
	observe.GlobalTrace("return: psExe, psErr")
	return psExe, psErr
}

// Tool implements the PowerShell tool for command execution.
type Tool struct{}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"PowerShell\"")
	return "PowerShell"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: psDescription")
	return psDescription
}

const psDescription = `Executes a PowerShell command and returns its output.

PowerShell Core (pwsh) is cross-platform and available on macOS, Linux, and Windows. On Windows, falls back to Windows PowerShell 5.1 if pwsh is not installed.

# Important Notes
 - Use dedicated tools (Glob, Grep, Read, Edit, Write) instead of PowerShell equivalents when possible.
 - You may specify an optional timeout in milliseconds (up to 600000ms / 10 minutes). Default is 120000ms (2 minutes).
 - Write a clear, concise description of what your command does.

# PowerShell Syntax
 - For paths with spaces, use the call operator: & "C:\Program Files\App\app.exe" arg1 arg2
 - Here-strings: @'<newline>content<newline>'@ (literal) or @"<newline>content<newline>"@ (interpolated)
 - The closing '@ or "@ must be at column 0 (no indent)
 - Stop-parsing token for special chars: --% (e.g., git log --% --format=%H)
 - PowerShell 5.1 does NOT support: && (use ;), || (use if), ternary operator, null-coalescing (??)
 - PowerShell 7+ supports: &&, ||, ternary, ?? (null-coalescing), ?. (null-conditional)

# Avoid
 - Do NOT use interactive cmdlets: Read-Host, Get-Credential, Out-GridView, Show-Command
 - Do NOT use Start-Sleep for delays >= 2 seconds. If you need to wait, explain why.
 - Do NOT use PowerShell equivalents of dedicated tools (e.g., Select-String instead of Grep)`

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
	observe.TraceCtx(ctx, "powershell", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "powershell", "Tool.CheckPerm", "exit")
	var in struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.Command == "" {
		observe.TraceCtx(ctx, "powershell", "Tool.CheckPerm", "if: err != nil || in.Command == \"\"")
		observe.TraceCtx(ctx, "powershell", "Tool.CheckPerm", "return: checker.Check(ctx, \"PowerShell\", \"\")")
		return checker.Check(ctx, "PowerShell", "")
	}

	cmdlet := extractCmdlet(in.Command)
	if cmdlet != "" && readOnlyCmdlets[strings.ToLower(cmdlet)] {
		observe.TraceCtx(ctx, "powershell", "Tool.CheckPerm", "if: cmdlet != \"\" && readOnlyCmdlets[strings.ToLower(cmdlet)]")
		observe.TraceCtx(ctx, "powershell", "Tool.CheckPerm", "return: permission.CheckResult{Decision: permission.DecisionAllow, Reason: \"read-only...")
		return permission.CheckResult{Decision: permission.DecisionAllow, Reason: "read-only cmdlet"}
	}
	observe.TraceCtx(ctx, "powershell", "Tool.CheckPerm", "return: checker.Check(ctx, \"PowerShell\", in.Command)")

	return checker.Check(ctx, "PowerShell", in.Command)
}

// extractCmdlet returns the first cmdlet name from a PowerShell command.
// Splits on whitespace, pipe, and semicolon to isolate the cmdlet.
func extractCmdlet(command string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		observe.GlobalTrace("if: cmd == \"\"")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	for i, r := range cmd {
		observe.GlobalTrace("range cmd")
		if r == ' ' || r == '\t' || r == '|' || r == ';' {
			observe.GlobalTrace("if: r == ' ' || r == '\\t' || r == '|' || r == ';'")
			observe.GlobalTrace("return: cmd[:i]")
			return cmd[:i]
		}
	}
	observe.GlobalTrace("return: cmd")
	return cmd
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "exit")

	var in PowerShellInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Command == "" {
		observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "if: in.Command == \"\"")
		observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"command is required\")")
		return tool.InvokeResult{}, fmt.Errorf("command is required")
	}

	exe, err := detectPowerShell()
	if err != nil {
		observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "return: tool.InvokeResult{}, err")
		return tool.InvokeResult{}, err
	}

	timeoutMs := defaultTimeoutMs
	if in.Timeout != nil {
		observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "if: in.Timeout != nil")
		timeoutMs = *in.Timeout
		if timeoutMs <= 0 {
			observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "if: timeoutMs <= 0")
			timeoutMs = defaultTimeoutMs
		}
		if timeoutMs > maxTimeoutMs {
			observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "if: timeoutMs > maxTimeoutMs")
			timeoutMs = maxTimeoutMs
		}
	}

	timeout := time.Duration(timeoutMs) * time.Millisecond
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fullCmd := utf8Preamble + in.Command

	cmd := exec.CommandContext(cmdCtx, exe, "-NoProfile", "-NonInteractive", "-Command", fullCmd)
	cmd.Dir = state.WorkDir()

	setProcAttr(cmd)

	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined

	err = cmd.Run()

	output := strings.TrimRight(combined.String(), "\n")
	output = strings.TrimLeft(output, "\n")

	if err != nil {
		observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "if: err != nil")
		if cmdCtx.Err() == context.DeadlineExceeded {
			observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "if: cmdCtx.Err() == context.DeadlineExceeded")
			if output != "" {
				observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "if: output != \"\"")
				output += "\n"
			}
			output += fmt.Sprintf("Command timed out after %dms", timeoutMs)
			observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "return: tool.InvokeResult{Content: output}, nil")
			return tool.InvokeResult{Content: output}, nil
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "if: ok")
			exitCode := exitErr.ExitCode()
			if output != "" {
				observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "if: output != \"\"")
				output += "\n"
			}
			output += fmt.Sprintf("Exit code %d", exitCode)
			observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "return: tool.InvokeResult{Content: output}, nil")
			return tool.InvokeResult{Content: output}, nil
		}
		observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"execute command: %w\", err)")

		return tool.InvokeResult{}, fmt.Errorf("execute command: %w", err)
	}
	observe.TraceCtx(ctx, "powershell", "Tool.Invoke", "return: tool.InvokeResult{Content: output}, nil")

	return tool.InvokeResult{Content: output}, nil
}
