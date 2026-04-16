package todo

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
)

func TestTodoWrite(t *testing.T) {
	newStore := func() *app.StateStore {
		return app.NewStateStore(app.AppState{
			Conversation: model.NewConversation(model.SystemPrompt{}, "test", "test", "/tmp"),
			CWD:          "/tmp",
		})
	}

	t.Run("name and flags", func(t *testing.T) {
		tool := &Tool{Store: newStore()}
		if tool.Name() != "TodoWrite" {
			t.Fatalf("expected TodoWrite, got %s", tool.Name())
		}
		if tool.Flags().ReadOnly {
			t.Fatal("expected not read-only")
		}
	})

	t.Run("write todos from empty", func(t *testing.T) {
		store := newStore()
		tool := &Tool{Store: store}

		input := json.RawMessage(`{"todos": [
			{"content": "Fix bug", "status": "in_progress"},
			{"content": "Write tests", "status": "pending"}
		]}`)

		result, err := tool.Invoke(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify state was updated
		snap := store.Snapshot()
		if len(snap.Todos) != 2 {
			t.Fatalf("expected 2 todos, got %d", len(snap.Todos))
		}
		if snap.Todos[0].Content != "Fix bug" || snap.Todos[0].Status != "in_progress" {
			t.Fatalf("todo 0 mismatch: %+v", snap.Todos[0])
		}
		if snap.Todos[1].Content != "Write tests" || snap.Todos[1].Status != "pending" {
			t.Fatalf("todo 1 mismatch: %+v", snap.Todos[1])
		}

		// Verify result contains JSON with old/new
		var res struct {
			OldTodos []app.TodoItem `json:"old_todos"`
			NewTodos []app.TodoItem `json:"new_todos"`
			Count    int            `json:"count"`
		}
		if err := json.Unmarshal([]byte(result.Content), &res); err != nil {
			t.Fatalf("result not valid JSON: %v", err)
		}
		if len(res.OldTodos) != 0 {
			t.Fatalf("expected 0 old todos, got %d", len(res.OldTodos))
		}
		if res.Count != 2 {
			t.Fatalf("expected count 2, got %d", res.Count)
		}
	})

	t.Run("replace existing todos", func(t *testing.T) {
		store := newStore()
		store.Update(func(s *app.AppState) {
			s.Todos = []app.TodoItem{
				{Content: "Old task", Status: "pending"},
			}
		})
		tool := &Tool{Store: store}

		input := json.RawMessage(`{"todos": [{"content": "New task", "status": "in_progress"}]}`)
		result, err := tool.Invoke(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		snap := store.Snapshot()
		if len(snap.Todos) != 1 {
			t.Fatalf("expected 1 todo, got %d", len(snap.Todos))
		}
		if snap.Todos[0].Content != "New task" {
			t.Fatalf("expected New task, got %s", snap.Todos[0].Content)
		}

		var res struct {
			OldTodos []app.TodoItem `json:"old_todos"`
		}
		if err := json.Unmarshal([]byte(result.Content), &res); err != nil {
			t.Fatalf("result not valid JSON: %v", err)
		}
		if len(res.OldTodos) != 1 || res.OldTodos[0].Content != "Old task" {
			t.Fatalf("old todos mismatch: %+v", res.OldTodos)
		}
	})

	t.Run("invalid status rejected", func(t *testing.T) {
		tool := &Tool{Store: newStore()}
		input := json.RawMessage(`{"todos": [{"content": "X", "status": "invalid"}]}`)
		_, err := tool.Invoke(context.Background(), input, nil)
		if err == nil {
			t.Fatal("expected error for invalid status")
		}
	})

	t.Run("empty content rejected", func(t *testing.T) {
		tool := &Tool{Store: newStore()}
		input := json.RawMessage(`{"todos": [{"content": "", "status": "pending"}]}`)
		_, err := tool.Invoke(context.Background(), input, nil)
		if err == nil {
			t.Fatal("expected error for empty content")
		}
	})

	t.Run("empty list clears todos", func(t *testing.T) {
		store := newStore()
		store.Update(func(s *app.AppState) {
			s.Todos = []app.TodoItem{{Content: "X", Status: "pending"}}
		})
		tool := &Tool{Store: store}
		input := json.RawMessage(`{"todos": []}`)
		_, err := tool.Invoke(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		snap := store.Snapshot()
		if len(snap.Todos) != 0 {
			t.Fatalf("expected 0 todos, got %d", len(snap.Todos))
		}
	})

	t.Run("all completed clears list", func(t *testing.T) {
		store := newStore()
		tool := &Tool{Store: store}
		input := json.RawMessage(`{"todos": [
			{"content": "Task A", "status": "completed"},
			{"content": "Task B", "status": "completed"}
		]}`)
		result, err := tool.Invoke(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		snap := store.Snapshot()
		if len(snap.Todos) != 0 {
			t.Fatalf("expected 0 todos when all completed, got %d", len(snap.Todos))
		}
		var res struct {
			NewTodos []app.TodoItem `json:"new_todos"`
			Count    int            `json:"count"`
		}
		if err := json.Unmarshal([]byte(result.Content), &res); err != nil {
			t.Fatalf("result not valid JSON: %v", err)
		}
		if res.Count != 0 {
			t.Fatalf("expected count 0, got %d", res.Count)
		}
	})

	t.Run("mixed statuses not cleared", func(t *testing.T) {
		store := newStore()
		tool := &Tool{Store: store}
		input := json.RawMessage(`{"todos": [
			{"content": "Done", "status": "completed"},
			{"content": "Not done", "status": "pending"}
		]}`)
		_, err := tool.Invoke(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		snap := store.Snapshot()
		if len(snap.Todos) != 2 {
			t.Fatalf("expected 2 todos, got %d", len(snap.Todos))
		}
	})

	t.Run("schema is valid JSON", func(t *testing.T) {
		tool := &Tool{Store: newStore()}
		var schema map[string]any
		if err := json.Unmarshal(tool.InputSchema(), &schema); err != nil {
			t.Fatalf("invalid schema JSON: %v", err)
		}
	})
}
