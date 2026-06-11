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
	"additionalProperties": false,
	"required": ["id"],
	"properties": {
		"id": {
			"type": "string",
			"description": "The task ID to update"
		},
		"status": {
			"type": "string",
			"enum": ["pending", "running", "completed", "failed", "cancelled"],
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

Status workflow: pending → running → completed or failed. Cancelled tasks use cancelled.

When to use:
- Mark tasks running BEFORE beginning work
- Mark tasks completed ONLY when fully accomplished
- If blocked, keep as running and create a new task for the blocker
- Never mark a task completed if tests are failing or implementation is partial

Tips:
- Read a task's latest state with TaskGet before updating
- Set status to running when starting, completed when done`

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

	var fields task.UpdateFields
	if in.Status != "" {
		observe.GlobalTrace("if: in.Status != \"\"")
		status, parseErr := task.ParseStatus(in.Status)
		if parseErr != nil {
			observe.GlobalTrace("if: parseErr != nil")
			observe.GlobalTrace("return: tool.InvokeResult{Content: parseErr.Error()}, nil")
			return tool.InvokeResult{Content: parseErr.Error()}, nil
		}
		fields.Status = &status
	}
	if in.Description != "" {
		observe.GlobalTrace("if: in.Description != \"\"")
		fields.Description = &in.Description
	}
	if in.Result != "" {
		observe.GlobalTrace("if: in.Result != \"\"")
		fields.Result = &in.Result
	}
	err := t.Tasks.UpdateFields(in.ID, fields)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{Content: err.Error()}, nil")
		return tool.InvokeResult{Content: err.Error()}, nil
	}
	observe.GlobalTrace("return: tool.InvokeResult{Content: fmt.Sprintf(\"Task %s updated\", in.ID)}, nil")
	return tool.InvokeResult{Content: fmt.Sprintf("Task %s updated", in.ID)}, nil
}
