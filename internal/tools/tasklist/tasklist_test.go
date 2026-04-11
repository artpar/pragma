package tasklist

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/task"
)

func setupTaskRegistry(t *testing.T) *task.Registry {
	t.Helper()
	bus := observe.NewEventBus(16)
	t.Cleanup(func() { bus.Drain() })
	return task.NewRegistry(bus)
}

func TestTaskList_Name(t *testing.T) {
	tl := &Tool{}
	if tl.Name() != "TaskList" {
		t.Fatalf("expected TaskList, got %s", tl.Name())
	}
}

func TestTaskList_Invoke_Empty(t *testing.T) {
	reg := setupTaskRegistry(t)
	tl := &Tool{Tasks: reg}
	result, err := tl.Invoke(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var tasks []json.RawMessage
	if err := json.Unmarshal([]byte(result.Content), &tasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestTaskList_Invoke_WithTasks(t *testing.T) {
	reg := setupTaskRegistry(t)
	reg.Create("task1", "desc1")
	tk2 := reg.Create("task2", "desc2")
	// Set tk2 to running.
	reg.Update(tk2.ID, func(tk *task.Task) {
		tk.Status = task.TaskRunning
	})

	tl := &Tool{Tasks: reg}

	tests := []struct {
		name      string
		input     string
		wantCount int
		wantErr   bool
	}{
		{"no filter", `{}`, 2, false},
		{"filter pending", `{"status":"pending"}`, 1, false},
		{"filter running", `{"status":"running"}`, 1, false},
		{"filter completed", `{"status":"completed"}`, 0, false},
		{"invalid json", `{bad`, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tl.Invoke(context.Background(), json.RawMessage(tt.input), nil)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var tasks []json.RawMessage
			if err := json.Unmarshal([]byte(result.Content), &tasks); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(tasks) != tt.wantCount {
				t.Fatalf("expected %d tasks, got %d", tt.wantCount, len(tasks))
			}
		})
	}
}

func TestTaskList_Invoke_InvalidStatus(t *testing.T) {
	reg := setupTaskRegistry(t)
	reg.Create("task1", "desc1")
	tl := &Tool{Tasks: reg}

	// An invalid status should just return no results (acts as a filter).
	result, err := tl.Invoke(context.Background(), json.RawMessage(`{"status":"bogus"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, "[]") {
		var tasks []json.RawMessage
		json.Unmarshal([]byte(result.Content), &tasks)
		if len(tasks) != 0 {
			t.Fatalf("expected 0 tasks for invalid status filter, got %d", len(tasks))
		}
	}
}
