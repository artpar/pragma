package taskcreate

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

func TestTaskCreate_Name(t *testing.T) {
	tl := &Tool{}
	if tl.Name() != "TaskCreate" {
		t.Fatalf("expected TaskCreate, got %s", tl.Name())
	}
}

func TestTaskCreate_Flags(t *testing.T) {
	tl := &Tool{}
	if tl.Flags().ReadOnly {
		t.Fatal("TaskCreate should not be ReadOnly")
	}
}

func TestTaskCreate_Invoke(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{"valid", `{"subject":"test task","description":"do something"}`, false, ""},
		{"missing subject", `{"subject":"","description":"do something"}`, true, "subject is required"},
		{"no description", `{"subject":"test"}`, false, ""},
		{"invalid json", `{bad`, true, "invalid input"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := setupTaskRegistry(t)
			tl := &Tool{Tasks: reg}
			result, err := tl.Invoke(context.Background(), json.RawMessage(tt.input), nil)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.errMsg)
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(result.Content, "task-") {
				t.Fatalf("expected task ID in result, got %q", result.Content)
			}
		})
	}
}

func TestTaskCreate_CreatesRealTask(t *testing.T) {
	reg := setupTaskRegistry(t)
	tl := &Tool{Tasks: reg}

	_, err := tl.Invoke(context.Background(), json.RawMessage(`{"subject":"my task","description":"desc"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tasks := reg.List(nil)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Subject != "my task" {
		t.Fatalf("expected subject 'my task', got %q", tasks[0].Subject)
	}
	if tasks[0].Status != task.TaskPending {
		t.Fatalf("expected pending status, got %s", tasks[0].Status)
	}
}
