package taskget

import (
	"context"
	"encoding/json"
	"fmt"

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

func (t *Tool) Name() string                { return "TaskGet" }
func (t *Tool) Description() string          { return "Get details of a specific task by ID." }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, "TaskGet", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in TaskGetInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.ID == "" {
		return tool.InvokeResult{}, fmt.Errorf("id is required")
	}

	tk, ok := t.Tasks.Get(in.ID)
	if !ok {
		return tool.InvokeResult{Content: fmt.Sprintf("task %q not found", in.ID)}, nil
	}

	data, err := json.Marshal(tk)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("marshal task: %w", err)
	}
	return tool.InvokeResult{Content: string(data)}, nil
}
