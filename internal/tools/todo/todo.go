package todo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/gogent/internal/app"
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

func (t *Tool) Name() string                { return "TodoWrite" }
func (t *Tool) Description() string          { return "Update the session task checklist. Replaces the full list." }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, "TodoWrite", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in todoInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}

	for i, item := range in.Todos {
		switch item.Status {
		case "pending", "in_progress", "completed":
		default:
			return tool.InvokeResult{}, fmt.Errorf("invalid status %q for todo %d", item.Status, i)
		}
		if item.Content == "" {
			return tool.InvokeResult{}, fmt.Errorf("empty content for todo %d", i)
		}
	}

	// Read old state
	snap := t.Store.Snapshot()
	oldTodos := snap.Todos

	// Build new list; clear if all completed (matches TS behavior)
	allDone := len(in.Todos) > 0
	for _, item := range in.Todos {
		if item.Status != "completed" {
			allDone = false
			break
		}
	}
	var newTodos []app.TodoItem
	if allDone {
		newTodos = nil
	} else {
		newTodos = make([]app.TodoItem, len(in.Todos))
		for i, item := range in.Todos {
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
	return tool.InvokeResult{Content: string(out)}, nil
}
