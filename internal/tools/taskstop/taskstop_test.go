package taskstop

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

func TestTaskStop_Name(t *testing.T) {
	tl := &Tool{}
	if tl.Name() != "TaskStop" {
		t.Fatalf("expected TaskStop, got %s", tl.Name())
	}
}

func TestTaskStop_Invoke(t *testing.T) {
	reg := setupTaskRegistry(t)
	pendingTask := reg.Create("pending task", "desc")

	runningTask := reg.Create("running task", "desc")
	cancelCalled := false
	reg.Update(runningTask.ID, func(tk *task.Task) {
		tk.Status = task.TaskRunning
		tk.Cancel = func() { cancelCalled = true }
	})

	completedTask := reg.Create("completed task", "desc")
	reg.Update(completedTask.ID, func(tk *task.Task) {
		tk.Status = task.TaskCompleted
	})

	tests := []struct {
		name           string
		input          string
		wantErr        bool
		errMsg         string
		wantContent    string
		checkCancelled bool
	}{
		{"cancel pending", `{"id":"` + pendingTask.ID + `"}`, false, "", "cancelled", false},
		{"cancel running", `{"id":"` + runningTask.ID + `"}`, false, "", "cancelled", true},
		{"cancel completed", `{"id":"` + completedTask.ID + `"}`, false, "", "cannot cancel", false},
		{"non-existent", `{"id":"task-999"}`, false, "", "not found", false},
		{"empty id", `{"id":""}`, true, "id is required", "", false},
		{"invalid json", `{bad`, true, "invalid input", "", false},
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
			if !strings.Contains(result.Content, tt.wantContent) {
				t.Fatalf("expected %q in content, got %q", tt.wantContent, result.Content)
			}
		})
	}

	if !cancelCalled {
		t.Fatal("expected Cancel function to be called for running task")
	}
}

func TestTaskStop_VerifiesStatus(t *testing.T) {
	reg := setupTaskRegistry(t)
	tk := reg.Create("test", "desc")
	reg.Update(tk.ID, func(tt *task.Task) {
		tt.Status = task.TaskRunning
	})

	tl := &Tool{Tasks: reg}
	_, err := tl.Invoke(context.Background(), json.RawMessage(`{"id":"`+tk.ID+`"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated, _ := reg.Get(tk.ID)
	if updated.Status != task.TaskCancelled {
		t.Fatalf("expected cancelled, got %s", updated.Status)
	}
}
