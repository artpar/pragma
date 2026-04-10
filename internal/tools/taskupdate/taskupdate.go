package taskupdate

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/task"
	"github.com/artpar/gogent/internal/tool"
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

func (t *Tool) Name() string                { return "TaskUpdate" }
func (t *Tool) Description() string          { return "Update a task's status, description, or result." }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: false, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, "TaskUpdate", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in TaskUpdateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.ID == "" {
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
		return tool.InvokeResult{Content: err.Error()}, nil
	}
	return tool.InvokeResult{Content: fmt.Sprintf("Task %s updated", in.ID)}, nil
}
