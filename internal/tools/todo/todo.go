package todo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

type todoInput struct {
	Todos []todoItemInput `json:"todos" desc:"The complete list of todos to set"`
}

type todoItemInput struct {
	Content string `json:"content" desc:"Task content/description"`
	Status  string `json:"status" desc:"Status: pending, in_progress, or completed"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["todos"],
	"properties": {
		"todos": {
			"type": "array",
			"description": "The complete list of todos to set (replaces existing)",
			"items": {
				"type": "object",
				"required": ["content", "status"],
				"properties": {
					"content": {
						"type": "string",
						"description": "Task content/description"
					},
					"status": {
						"type": "string",
						"enum": ["pending", "in_progress", "completed"],
						"description": "Task status"
					}
				}
			}
		}
	}
}`)

// Tool manages the session task checklist stored in AppState.
type Tool struct {
	Store *app.StateStore
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"TodoWrite\"")
	return "TodoWrite"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: todoDescription")
	return todoDescription
}

const todoDescription = `Use this tool to create and manage a structured task list for your current coding session. This helps you track progress, organize complex tasks, and demonstrate thoroughness to the user. It also helps the user understand the progress of the task and overall progress of their requests.

## When to Use This Tool

1. Complex multi-step tasks — when a task requires 3 or more distinct steps or actions
2. Non-trivial tasks — tasks that require careful planning or multiple operations
3. User explicitly requests a todo list
4. User provides multiple tasks — numbered or comma-separated list
5. After receiving new instructions — immediately capture requirements as todos
6. When you start working on a task — mark it as in_progress BEFORE beginning work. Only have one todo as in_progress at a time
7. After completing a task — mark it as completed and add any new follow-up tasks discovered during implementation

## When NOT to Use This Tool

1. There is only a single, straightforward task
2. The task is trivial and tracking provides no organizational benefit
3. The task can be completed in fewer than 3 trivial steps
4. The task is purely conversational or informational

NOTE: do not use this tool if there is only one trivial task. Just do the task directly.

## Important

- Each call REPLACES the entire todo list — always include all items (completed and pending)
- Statuses: "pending", "in_progress", "completed"
- When all items are marked "completed", the list is automatically cleared`

func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: false}")
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "todo", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "todo", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "todo", "Tool.CheckPerm", "return: checker.Check(ctx, \"TodoWrite\", \"\")")
	return checker.Check(ctx, "TodoWrite", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var in todoInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}

	for i, item := range in.Todos {
		observe.GlobalTrace("range in.Todos")
		switch item.Status {
		case "pending", "in_progress", "completed":
			observe.GlobalTrace("case: \"pending\", \"in_progress\", \"completed\"")
		default:
			observe.GlobalTrace("default")
			return tool.InvokeResult{}, fmt.Errorf("invalid status %q for todo %d", item.Status, i)
		}
		if item.Content == "" {
			observe.GlobalTrace("if: item.Content == \"\"")
			observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"empty content for todo %d\", i)")
			return tool.InvokeResult{}, fmt.Errorf("empty content for todo %d", i)
		}
	}

	snap := t.Store.Snapshot()
	oldTodos := snap.Todos

	allDone := len(in.Todos) > 0
	for _, item := range in.Todos {
		observe.GlobalTrace("range in.Todos")
		if item.Status != "completed" {
			observe.GlobalTrace("if: item.Status != \"completed\"")
			allDone = false
			break
		}
	}
	var newTodos []app.TodoItem
	if allDone {
		observe.GlobalTrace("if: allDone")
		newTodos = nil
	} else {
		observe.GlobalTrace("else: allDone")
		newTodos = make([]app.TodoItem, len(in.Todos))
		for i, item := range in.Todos {
			observe.GlobalTrace("range in.Todos")
			newTodos[i] = app.TodoItem{
				Content: item.Content,
				Status:  item.Status,
			}
		}
	}
	t.Store.Update(func(s *app.AppState) {
		s.Todos = newTodos
	})

	// Return JSON diff
	type result struct {
		OldTodos []app.TodoItem `json:"old_todos"`
		NewTodos []app.TodoItem `json:"new_todos"`
		Count    int            `json:"count"`
	}
	out, _ := json.Marshal(result{
		OldTodos: oldTodos,
		NewTodos: newTodos,
		Count:    len(newTodos),
	})
	observe.GlobalTrace("return: tool.InvokeResult{Content: string(out)}, nil")
	return tool.InvokeResult{Content: string(out)}, nil
}
