package taskoutput

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/task"
	"github.com/artpar/gogent/internal/tool"
)

type taskOutputInput struct {
	TaskID string `json:"task_id" desc:"The task ID to read output from"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["task_id"],
	"properties": {
		"task_id": {
			"type": "string",
			"description": "The task ID to read output from"
		}
	}
}`)

// Tool reads the output/result of a background or foreground task.
type Tool struct {
	Tasks *task.Registry
}

func (t *Tool) Name() string                { return "TaskOutput" }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) Description() string {
	return "Read the output of a task. Returns the current status and result of a background task. Use this to check on async agent results."
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, "TaskOutput", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in taskOutputInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.TaskID == "" {
		return tool.InvokeResult{}, fmt.Errorf("task_id is required")
	}

	tk, ok := t.Tasks.Get(in.TaskID)
	if !ok {
		result := taskOutputResult{
			RetrievalStatus: "not_ready",
		}
		data, _ := json.Marshal(result)
		return tool.InvokeResult{Content: string(data)}, nil
	}

	switch tk.Status {
	case task.TaskCompleted, task.TaskFailed, task.TaskCancelled:
		result := taskOutputResult{
			RetrievalStatus: "success",
			Task: &taskSnapshot{
				ID:         tk.ID,
				Status:     string(tk.Status),
				Result:     tk.Result,
				Error:      tk.Error,
				TokensUsed: tk.TokensUsed,
			},
		}
		data, _ := json.Marshal(result)
		return tool.InvokeResult{Content: string(data)}, nil

	default:
		// Still running or pending — return partial info
		result := taskOutputResult{
			RetrievalStatus: "not_ready",
			Task: &taskSnapshot{
				ID:     tk.ID,
				Status: string(tk.Status),
			},
		}
		data, _ := json.Marshal(result)
		return tool.InvokeResult{Content: string(data)}, nil
	}
}

type taskOutputResult struct {
	RetrievalStatus string        `json:"retrieval_status"`
	Task            *taskSnapshot `json:"task,omitempty"`
}

type taskSnapshot struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Result     string `json:"result,omitempty"`
	Error      string `json:"error,omitempty"`
	TokensUsed int    `json:"tokens_used,omitempty"`
}
