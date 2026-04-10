package tasklist

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/task"
	"github.com/artpar/gogent/internal/tool"
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

func (t *Tool) Name() string                { return "TaskList" }
func (t *Tool) Description() string          { return "List all tasks, optionally filtered by status." }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, "TaskList", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in TaskListInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}

	var statusFilter *task.TaskStatus
	if in.Status != "" {
		s := task.TaskStatus(in.Status)
		statusFilter = &s
	}

	tasks := t.Tasks.List(statusFilter)
	data, err := json.Marshal(tasks)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("marshal tasks: %w", err)
	}
	return tool.InvokeResult{Content: string(data)}, nil
}
