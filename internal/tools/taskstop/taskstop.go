package taskstop

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/task"
	"github.com/artpar/pragma/internal/tool"
)

type TaskStopInput struct {
	ID string `json:"id" desc:"Task ID to cancel"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
	"required": ["id"],
	"properties": {
		"id": {
			"type": "string",
			"description": "The task ID to cancel"
		}
	}
}`)

type Tool struct {
	Tasks *task.Registry
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"TaskStop\"")
	return "TaskStop"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Cancel a running or pending task by its ID...\"")
	observe.GlobalTrace("return: \"Cancel a running or pending task by its ID. Stops background agents and retu...")
	return "Cancel a running or pending task by its ID. Stops background agents and returns a success or failure status. Use this when you need to terminate a long-running background task."
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
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: false, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "taskstop", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "taskstop", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "taskstop", "Tool.CheckPerm", "return: checker.Check(ctx, \"TaskStop\", \"\")")
	return checker.Check(ctx, "TaskStop", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var in TaskStopInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.ID == "" {
		observe.GlobalTrace("if: in.ID == \"\"")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"id is required\")")
		return tool.InvokeResult{}, fmt.Errorf("id is required")
	}

	result, err := t.Tasks.ApplyLifecycleCommand(in.ID, task.LifecycleCommandKill)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{Content: err.Error()}, nil")
		return tool.InvokeResult{Content: err.Error()}, nil
	}
	observe.GlobalTrace("return: tool.InvokeResult{Content: result.Message}, nil")
	return tool.InvokeResult{Content: result.Message}, nil
}
