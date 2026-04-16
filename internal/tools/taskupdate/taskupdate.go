package taskupdate

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/task"
	"github.com/artpar/pragma/internal/tool"
)

type TaskUpdateInput struct {
	ID          string `json:"id" desc:"Task ID to update"`
	Status      string `json:"status,omitempty" desc:"New status"`
	Description string `json:"description,omitempty" desc:"New description"`
	Result      string `json:"result,omitempty" desc:"Task result content"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["id"],
	"properties": {
		"id": {
			"type": "string",
			"description": "The task ID to update"
		},
		"status": {
			"type": "string",
			"enum": ["pending", "running", "completed", "failed"],
			"description": "New task status"
		},
		"description": {
			"type": "string",
			"description": "New task description"
		},
		"result": {
			"type": "string",
			"description": "Task result content"
		}
	}
}`)

type Tool struct {
	Tasks *task.Registry
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"TaskUpdate\"")
	return "TaskUpdate"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Update a task's status, description, or result...\"")
	observe.GlobalTrace("return: taskUpdateDescription")
	return taskUpdateDescription
}

const taskUpdateDescription = `Update a task's status, description, or result.

Status workflow: pending → in_progress → completed. Use 'deleted' to permanently remove.

When to use:
- Mark tasks in_progress BEFORE beginning work
- Mark tasks completed ONLY when fully accomplished
- If blocked, keep as in_progress and create a new task for the blocker
- Never mark a task completed if tests are failing or implementation is partial

Tips:
- Read a task's latest state with TaskGet before updating
- Set status to in_progress when starting, completed when done`

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
	observe.TraceCtx(ctx, "taskupdate", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "taskupdate", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "taskupdate", "Tool.CheckPerm", "return: checker.Check(ctx, \"TaskUpdate\", \"\")")
	return checker.Check(ctx, "TaskUpdate", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var in TaskUpdateInput
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

	err := t.Tasks.Update(in.ID, func(tk *task.Task) {
		if in.Status != "" {
			tk.Status = task.TaskStatus(in.Status)
		}
		if in.Description != "" {
			tk.Description = in.Description
		}
		if in.Result != "" {
			tk.Result = in.Result
		}
	})
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{Content: err.Error()}, nil")
		return tool.InvokeResult{Content: err.Error()}, nil
	}
	observe.GlobalTrace("return: tool.InvokeResult{Content: fmt.Sprintf(\"Task %s updated\", in.ID)}, nil")
	return tool.InvokeResult{Content: fmt.Sprintf("Task %s updated", in.ID)}, nil
}
