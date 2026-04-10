package taskcreate

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/task"
	"github.com/artpar/gogent/internal/tool"
)

type TaskCreateInput struct {
	Subject     string `json:"subject" desc:"Brief task title"`
	Description string `json:"description" desc:"What needs to be done"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["subject", "description"],
	"properties": {
		"subject": {
			"type": "string",
			"description": "A brief title for the task"
		},
		"description": {
			"type": "string",
			"description": "A detailed description of what needs to be done"
		}
	}
}`)

type Tool struct {
	Tasks *task.Registry
}

func (t *Tool) Name() string                { return "TaskCreate" }
func (t *Tool) Description() string          { return "Create a new task to track work progress." }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: false, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, "TaskCreate", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in TaskCreateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Subject == "" {
		return tool.InvokeResult{}, fmt.Errorf("subject is required")
	}

	created := t.Tasks.Create(in.Subject, in.Description)
	return tool.InvokeResult{
		Content: fmt.Sprintf("Task %s created: %s", created.ID, created.Subject),
	}, nil
}
