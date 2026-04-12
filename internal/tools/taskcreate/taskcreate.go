package taskcreate

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/gogent/internal/observe"
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

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"TaskCreate\"")
	observe.GlobalTrace("return: \"TaskCreate\"")
	observe.GlobalTrace("return: \"TaskCreate\"")
	return "TaskCreate"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Create a new task to track work progress. Use proactively...\"")
	observe.GlobalTrace("return: taskCreateDescription")
	observe.GlobalTrace("return: taskCreateDescription")
	return taskCreateDescription
}

const taskCreateDescription = `Create a new task to track work progress. Use proactively to organize complex, multi-step work.

When to use:
- Complex multi-step tasks requiring 3+ distinct steps
- Plan mode — create a task list to track the work
- User provides multiple tasks (numbered or comma-separated)
- After receiving new instructions — capture requirements as tasks

When NOT to use:
- Single, straightforward task completable in <3 trivial steps
- Purely conversational or informational requests

Tips:
- Create tasks with clear, specific subjects in imperative form (e.g., "Fix authentication bug")
- Check TaskList first to avoid creating duplicate tasks`

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
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: true}")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: true}")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: false, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "taskcreate", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "taskcreate", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "taskcreate", "Tool.CheckPerm", "return: checker.Check(ctx, \"TaskCreate\", \"\")")
	observe.TraceCtx(ctx, "taskcreate", "Tool.CheckPerm", "return: checker.Check(ctx, \"TaskCreate\", \"\")")
	observe.TraceCtx(ctx, "taskcreate", "Tool.CheckPerm", "return: checker.Check(ctx, \"TaskCreate\", \"\")")
	return checker.Check(ctx, "TaskCreate", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var in TaskCreateInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Subject == "" {
		observe.GlobalTrace("if: in.Subject == \"\"")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"subject is required\")")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"subject is required\")")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"subject is required\")")
		return tool.InvokeResult{}, fmt.Errorf("subject is required")
	}

	created := t.Tasks.Create(in.Subject, in.Description)
	observe.GlobalTrace("return: tool.InvokeResult{\n\tContent: fmt.Sprintf(\"Task %s created: %s\", created.ID, c...")
	observe.GlobalTrace("return: tool.InvokeResult{\n\tContent: fmt.Sprintf(\"Task %s created: %s\", created.ID, c...")
	observe.GlobalTrace("return: tool.InvokeResult{\n\tContent: fmt.Sprintf(\"Task %s created: %s\", created.ID, c...")
	return tool.InvokeResult{
		Content: fmt.Sprintf("Task %s created: %s", created.ID, created.Subject),
	}, nil
}
