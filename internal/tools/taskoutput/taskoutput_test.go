package taskoutput

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/task"
)

func setupTaskRegistry(t *testing.T) *task.Registry {
	t.Helper()
	bus := observe.NewEventBus(16)
	t.Cleanup(func() { bus.Drain() })
	return task.NewRegistry(bus)
}

func TestTaskOutput_Name(t *testing.T) {
	tl := &Tool{}
	if tl.Name() != "TaskOutput" {
		t.Fatalf("expected TaskOutput, got %s", tl.Name())
	}
}

func TestTaskOutput_Flags(t *testing.T) {
	tl := &Tool{}
	if !tl.Flags().ReadOnly {
		t.Fatal("TaskOutput should be ReadOnly")
	}
}

func TestTaskOutput_Invoke(t *testing.T) {
	reg := setupTaskRegistry(t)

	// Running task.
	running := reg.Create("running task", "desc")
	reg.Update(running.ID, func(tk *task.Task) {
		tk.Status = task.TaskRunning
	})

	// Completed task with result.
	completed := reg.Create("completed task", "desc")
	reg.Update(completed.ID, func(tk *task.Task) {
		tk.Status = task.TaskCompleted
		tk.Result = "all done"
		tk.TokensUsed = 1500
	})

	// Failed task with error.
	failed := reg.Create("failed task", "desc")
	reg.Update(failed.ID, func(tk *task.Task) {
		tk.Status = task.TaskFailed
		tk.Error = "something broke"
	})

	tests := []struct {
		name       string
		input      string
		wantErr    bool
		errMsg     string
		wantStatus string // retrieval_status
		wantResult string // result field in task snapshot
	}{
		{"running task", `{"task_id":"` + running.ID + `"}`, false, "", "not_ready", ""},
		{"completed task", `{"task_id":"` + completed.ID + `"}`, false, "", "success", "all done"},
		{"failed task", `{"task_id":"` + failed.ID + `"}`, false, "", "success", ""},
		{"non-existent", `{"task_id":"task-999"}`, false, "", "not_ready", ""},
		{"empty task_id", `{"task_id":""}`, true, "task_id is required", "", ""},
		{"invalid json", `{bad`, true, "invalid input", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tl := &Tool{Tasks: reg}
			result, err := tl.Invoke(context.Background(), json.RawMessage(tt.input), nil)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.errMsg)
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("expected %q in error, got %q", tt.errMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var out taskOutputResult
			if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if out.RetrievalStatus != tt.wantStatus {
				t.Fatalf("expected retrieval_status %q, got %q", tt.wantStatus, out.RetrievalStatus)
			}
			if tt.wantResult != "" && (out.Task == nil || out.Task.Result != tt.wantResult) {
				t.Fatalf("expected result %q, got %+v", tt.wantResult, out.Task)
			}
		})
	}
}

func TestTaskOutput_FailedTaskHasError(t *testing.T) {
	reg := setupTaskRegistry(t)
	tk := reg.Create("fail", "desc")
	reg.Update(tk.ID, func(tt *task.Task) {
		tt.Status = task.TaskFailed
		tt.Error = "crash"
	})

	tl := &Tool{Tasks: reg}
	result, err := tl.Invoke(context.Background(), json.RawMessage(`{"task_id":"`+tk.ID+`"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out taskOutputResult
	json.Unmarshal([]byte(result.Content), &out)
	if out.Task == nil || out.Task.Error != "crash" {
		t.Fatalf("expected error 'crash', got %+v", out.Task)
	}
}
