package tasklist

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/task"
	"github.com/artpar/pragma/internal/tool"
)

type TaskListInput struct {
	Status string `json:"status,omitempty" desc:"Filter by status (pending, running, completed, failed, cancelled)"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"status": {
			"type": "string",
			"enum": ["pending", "running", "completed", "failed", "cancelled"],
			"description": "Optional filter by task status"
		}
	}
}`)

type Tool struct {
	Tasks *task.Registry
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"TaskList\"")
	return "TaskList"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"List all tasks, optionally filtered by status. Returns a summary...\"")
	observe.GlobalTrace("return: taskListDescription")
	return taskListDescription
}

const taskListDescription = `List all tasks, optionally filtered by status. Returns a summary of each task including id, subject, status, and dependencies.

When to use:
- To see what tasks are available to work on
- To check overall progress
- After completing a task, to find newly unblocked work
- Prefer working on tasks in ID order (lowest first) when multiple are available

Use TaskGet with a specific task ID to view full details.`

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
	observe.TraceCtx(ctx, "tasklist", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "tasklist", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "tasklist", "Tool.CheckPerm", "return: checker.Check(ctx, \"TaskList\", \"\")")
	return checker.Check(ctx, "TaskList", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var in TaskListInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}

	var statusFilter *task.TaskStatus
	if in.Status != "" {
		observe.GlobalTrace("if: in.Status != \"\"")
		s := task.TaskStatus(in.Status)
		statusFilter = &s
	}

	tasks := t.Tasks.List(statusFilter)
	data, err := json.Marshal(tasks)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"marshal tasks: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("marshal tasks: %w", err)
	}
	observe.GlobalTrace("return: tool.InvokeResult{Content: string(data)}, nil")
	return tool.InvokeResult{Content: string(data)}, nil
}
