package slash

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/artpar/gogent/internal/observe"
)

// Block pattern: ```! command ```
var blockPattern = regexp.MustCompile("(?s)```!\\s*\n?([\\s\\S]*?)\n?```")

// Inline pattern: !`command` — requires whitespace or start-of-line before !
var inlinePattern = regexp.MustCompile("(?m)(?:^|\\s)!`([^`]+)`")

// ExecShellInPrompt replaces !`command` and ```! command ``` patterns in text
// with their stdout output. Each command runs with the given timeout.
// On error, replaces with a descriptive error string — never fails the whole prompt.
// Matches TS executeShellCommandsInPrompt but uses sequential execution
// and inline error replacement for resilience.
func ExecShellInPrompt(ctx context.Context, text string, timeout time.Duration) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	text = blockPattern.ReplaceAllStringFunc(text, func(match string) string {
		groups := blockPattern.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		cmd := strings.TrimSpace(groups[1])
		if cmd == "" {
			return match
		}
		return execShellCommand(ctx, cmd, timeout)
	})

	if !strings.Contains(text, "!`") {
		observe.TraceCtx(ctx, "slash", "ExecShellInPrompt", "if: !strings.Contains(text, \"!`\")")
		observe.TraceCtx(ctx, "slash", "ExecShellInPrompt", "return: text")
		return text
	}

	text = inlinePattern.ReplaceAllStringFunc(text, func(match string) string {
		groups := inlinePattern.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		cmd := strings.TrimSpace(groups[1])
		if cmd == "" {
			return match
		}

		prefix := ""
		if len(match) > 0 && (match[0] == ' ' || match[0] == '\t' || match[0] == '\n') {
			prefix = string(match[0])
		}
		return prefix + execShellCommand(ctx, cmd, timeout)
	})
	observe.TraceCtx(ctx, "slash", "ExecShellInPrompt", "return: text")

	return text
}

// execShellCommand runs a single shell command with timeout and returns its output.
// On error, returns a descriptive error string instead of failing.
func execShellCommand(ctx context.Context, command string, timeout time.Duration) string {
	observe.TraceCtx(ctx, "slash", "execShellCommand", "enter: "+command)
	defer observe.TraceCtx(ctx, "slash", "execShellCommand", "exit")

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "sh", "-c", command).CombinedOutput()
	if err != nil {
		observe.TraceCtx(ctx, "slash", "execShellCommand", "error: "+err.Error())
		observe.TraceCtx(ctx, "slash", "execShellCommand", "return: fmt.Sprintf(\"(error running %s: %v)\", command, err)")
		return fmt.Sprintf("(error running %s: %v)", command, err)
	}
	observe.TraceCtx(ctx, "slash", "execShellCommand", "return: strings.TrimRight(string(out), \"\\n\")")
	return strings.TrimRight(string(out), "\n")
}
