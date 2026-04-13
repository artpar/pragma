package sleep

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

type sleepInput struct {
	Seconds int `json:"seconds" desc:"Number of seconds to sleep (1-300)"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["seconds"],
	"properties": {
		"seconds": {
			"type": "integer",
			"description": "Number of seconds to sleep (1-300)",
			"minimum": 1,
			"maximum": 300
		}
	}
}`)

// Tool implements an async delay. Context cancellation interrupts the sleep.
type Tool struct{}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Sleep\"")
	return "Sleep"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Wait for a specified duration. The user can interrupt...\"")
	observe.GlobalTrace("return: sleepDescription")
	return sleepDescription
}

const sleepDescription = `Wait for a specified duration. The user can interrupt the sleep at any time.

Use this when the user tells you to sleep or rest, when you have nothing to do, or when you're waiting for something. You can call this concurrently with other tools — it won't interfere with them. Prefer this over Bash(sleep ...) — it doesn't hold a shell process.`

func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "sleep", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "sleep", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "sleep", "Tool.CheckPerm", "return: checker.Check(ctx, \"Sleep\", \"\")")
	return checker.Check(ctx, "Sleep", "")
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "sleep", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "sleep", "Tool.Invoke", "exit")
	var in sleepInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "sleep", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "sleep", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Seconds < 1 {
		observe.TraceCtx(ctx, "sleep", "Tool.Invoke", "if: in.Seconds < 1")
		in.Seconds = 1
	}
	if in.Seconds > 300 {
		observe.TraceCtx(ctx, "sleep", "Tool.Invoke", "if: in.Seconds > 300")
		in.Seconds = 300
	}

	duration := time.Duration(in.Seconds) * time.Second
	select {
	case <-time.After(duration):
		observe.TraceCtx(ctx, "sleep", "Tool.Invoke", "select: <-time.After(duration)")
		unit := "seconds"
		if in.Seconds == 1 {
			unit = "second"
		}
		return tool.InvokeResult{
			Content: fmt.Sprintf("Slept for %d %s.", in.Seconds, unit),
		}, nil
	case <-ctx.Done():
		observe.TraceCtx(ctx, "sleep", "Tool.Invoke", "select: <-ctx.Done()")
		return tool.InvokeResult{
			Content: "Sleep cancelled.",
		}, nil
	}
}
