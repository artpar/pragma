package taskget

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/task"
	"github.com/artpar/gogent/internal/tool"
)

type TaskGetInput struct {
	ID string `json:"id" desc:"Task ID to retrieve"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["id"],
	"properties": {
		"id": {
			"type": "string",
			"description": "The task ID to retrieve"
		}
	}
}`)

type Tool struct {
	Tasks *task.Registry
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"TaskGet\"")
	return "TaskGet"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Get details of a specific task by ID. Returns full task details...\"")
	observe.GlobalTrace("return: taskGetDescription")
	return taskGetDescription
}

const taskGetDescription = `Get details of a specific task by ID. Returns full task details including subject, description, status, and dependencies.

When to use:
- When you need the full description and context before starting work on a task
- To understand task dependencies (what it blocks, what blocks it)
- After being assigned a task, to get complete requirements

Tips:
- After fetching a task, verify its blockedBy list is empty before beginning work
- Use TaskList to see all tasks in summary form`

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
	observe.TraceCtx(ctx, "taskget", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "taskget", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "taskget", "Tool.CheckPerm", "return: checker.Check(ctx, \"TaskGet\", \"\")")
	return checker.Check(ctx, "TaskGet", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var in TaskGetInput
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

	tk, ok := t.Tasks.Get(in.ID)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: tool.InvokeResult{Content: fmt.Sprintf(\"task %q not found\", in.ID)}, nil")
		return tool.InvokeResult{Content: fmt.Sprintf("task %q not found", in.ID)}, nil
	}

	data, err := json.Marshal(tk)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"marshal task: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("marshal task: %w", err)
	}
	observe.GlobalTrace("return: tool.InvokeResult{Content: string(data)}, nil")
	return tool.InvokeResult{Content: string(data)}, nil
}
