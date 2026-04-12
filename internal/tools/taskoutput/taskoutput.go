package taskoutput

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/gogent/internal/observe"
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

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"TaskOutput\"")
	observe.GlobalTrace("return: \"TaskOutput\"")
	observe.GlobalTrace("return: \"TaskOutput\"")
	return "TaskOutput"
}
func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	observe.GlobalTrace("return: inputSchema")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Read the output of a task. Returns the current status and result of a backgr...")
	observe.GlobalTrace("return: \"Read the output of a task. Returns the current status and result of a backgr...")
	observe.GlobalTrace("return: \"Read the output of a task. Returns the current status and result of a backgr...")
	return "Read the output of a task. Returns the current status and result of a background task. Use this to check on async agent results."
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "taskoutput", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "taskoutput", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "taskoutput", "Tool.CheckPerm", "return: checker.Check(ctx, \"TaskOutput\", \"\")")
	observe.TraceCtx(ctx, "taskoutput", "Tool.CheckPerm", "return: checker.Check(ctx, \"TaskOutput\", \"\")")
	observe.TraceCtx(ctx, "taskoutput", "Tool.CheckPerm", "return: checker.Check(ctx, \"TaskOutput\", \"\")")
	return checker.Check(ctx, "TaskOutput", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var in taskOutputInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.TaskID == "" {
		observe.GlobalTrace("if: in.TaskID == \"\"")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"task_id is required\")")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"task_id is required\")")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"task_id is required\")")
		return tool.InvokeResult{}, fmt.Errorf("task_id is required")
	}

	tk, ok := t.Tasks.Get(in.TaskID)
	if !ok {
		observe.GlobalTrace("if: !ok")
		result := taskOutputResult{
			RetrievalStatus: "not_ready",
		}
		data, _ := json.Marshal(result)
		observe.GlobalTrace("return: tool.InvokeResult{Content: string(data)}, nil")
		observe.GlobalTrace("return: tool.InvokeResult{Content: string(data)}, nil")
		observe.GlobalTrace("return: tool.InvokeResult{Content: string(data)}, nil")
		return tool.InvokeResult{Content: string(data)}, nil
	}

	switch tk.Status {
	case task.TaskCompleted, task.TaskFailed, task.TaskCancelled:
		observe.GlobalTrace("case: task.TaskCompleted, task.TaskFailed, task.TaskCancelled")
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
		observe.GlobalTrace("default")

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
