package taskstop

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/task"
	"github.com/artpar/gogent/internal/tool"
)

type TaskStopInput struct {
	ID string `json:"id" desc:"Task ID to cancel"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
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

func (t *Tool) Name() string                { return "TaskStop" }
func (t *Tool) Description() string          { return "Cancel a running or pending task." }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: false, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, "TaskStop", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in TaskStopInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.ID == "" {
		return tool.InvokeResult{}, fmt.Errorf("id is required")
	}

	if err := t.Tasks.Cancel(in.ID); err != nil {
		return tool.InvokeResult{Content: err.Error()}, nil
	}
	return tool.InvokeResult{Content: fmt.Sprintf("Task %s cancelled", in.ID)}, nil
}
