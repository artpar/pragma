package taskget

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

func TestTaskGet_Name(t *testing.T) {
	tl := &Tool{}
	if tl.Name() != "TaskGet" {
		t.Fatalf("expected TaskGet, got %s", tl.Name())
	}
}

func TestTaskGet_Flags(t *testing.T) {
	tl := &Tool{}
	if !tl.Flags().ReadOnly {
		t.Fatal("TaskGet should be ReadOnly")
	}
}

func TestTaskGet_Invoke(t *testing.T) {
	reg := setupTaskRegistry(t)
	created := reg.Create("test task", "description")

	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
		wantNF  bool // expect "not found" in content (not error)
	}{
		{"existing task", `{"id":"` + created.ID + `"}`, false, "", false},
		{"non-existent", `{"id":"task-999"}`, false, "", true},
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
			// Should have valid JSON with task data.
			var tk task.Task
			if err := json.Unmarshal([]byte(result.Content), &tk); err != nil {
				t.Fatalf("unmarshal task: %v", err)
			}
			if tk.ID != created.ID {
				t.Fatalf("expected ID %q, got %q", created.ID, tk.ID)
			}
			if tk.Subject != "test task" {
				t.Fatalf("expected subject 'test task', got %q", tk.Subject)
			}
		})
	}
}
