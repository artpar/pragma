package taskupdate

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

func TestTaskUpdate_Name(t *testing.T) {
	tl := &Tool{}
	if tl.Name() != "TaskUpdate" {
		t.Fatalf("expected TaskUpdate, got %s", tl.Name())
	}
}

func TestTaskUpdate_Invoke(t *testing.T) {
	reg := setupTaskRegistry(t)
	created := reg.Create("test task", "original desc")

	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
		wantNF  bool // expect not-found in content
	}{
		{"update status", `{"id":"` + created.ID + `","status":"running"}`, false, "", false},
		{"update description", `{"id":"` + created.ID + `","description":"new desc"}`, false, "", false},
		{"update result", `{"id":"` + created.ID + `","result":"done"}`, false, "", false},
		{"non-existent task", `{"id":"task-999"}`, false, "", true},
		{"empty id", `{"id":""}`, true, "id is required", false},
		{"invalid json", `{bad`, true, "invalid input", false},
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
			if tt.wantNF {
				if !strings.Contains(result.Content, "not found") {
					t.Fatalf("expected 'not found' in content, got %q", result.Content)
				}
				return
			}
			if !strings.Contains(result.Content, "updated") {
				t.Fatalf("expected 'updated' in content, got %q", result.Content)
			}
		})
	}
}

func TestTaskUpdate_Invoke_VerifyMutation(t *testing.T) {
	reg := setupTaskRegistry(t)
	created := reg.Create("test", "original")
	tl := &Tool{Tasks: reg}

	_, err := tl.Invoke(context.Background(), json.RawMessage(`{"id":"`+created.ID+`","status":"completed","description":"new","result":"success"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tk, ok := reg.Get(created.ID)
	if !ok {
		t.Fatal("task not found after update")
	}
	if tk.Status != task.TaskCompleted {
		t.Fatalf("expected completed, got %s", tk.Status)
	}
	if tk.Description != "new" {
		t.Fatalf("expected 'new' description, got %q", tk.Description)
	}
	if tk.Result != "success" {
		t.Fatalf("expected 'success' result, got %q", tk.Result)
	}
}
