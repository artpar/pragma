package todo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
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
	"additionalProperties": false,
	"required": ["todos"],
	"properties": {
		"todos": {
			"type": "array",
			"description": "The complete list of todos to set (replaces existing)",
			"items": {
				"type": "object",
	"additionalProperties": false,
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
	observe.GlobalTrace("return: \"update_plan\"")
	return "update_plan"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: todoDescription")
	return todoDescription
}

const todoDescription = `Updates the task plan.
Provide a list of plan items, each with a step and status.
At most one step can be in_progress at a time.

Use this to keep an up-to-date, step-by-step plan for non-trivial tasks. Do not use it for simple or single-step work.

Each call replaces the entire plan. Statuses are pending, in_progress, and completed.`

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
	observe.TraceCtx(ctx, "todo", "Tool.CheckPerm", "return: checker.Check(ctx, \"update_plan\", \"\")")
	return checker.Check(ctx, "update_plan", "")
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
	inProgressCount := 0
	for _, item := range in.Todos {
		if item.Status == "in_progress" {
			inProgressCount++
		}
	}
	if inProgressCount > 1 {
		return tool.InvokeResult{}, fmt.Errorf("only one plan item can be in_progress")
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
