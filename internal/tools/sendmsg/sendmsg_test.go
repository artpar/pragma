package sendmsg

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

func TestSendMessage_Name(t *testing.T) {
	tl := &Tool{}
	if tl.Name() != "SendMessage" {
		t.Fatalf("expected SendMessage, got %s", tl.Name())
	}
}

func TestSendMessage_Invoke(t *testing.T) {
	reg := setupTaskRegistry(t)

	// Create a running task with an agent name.
	running := reg.Create("running agent", "desc")
	reg.Update(running.ID, func(tk *task.Task) {
		tk.Status = task.TaskRunning
		tk.AgentName = "myagent"
	})

	// Create a completed task.
	completed := reg.Create("completed agent", "desc")
	reg.Update(completed.ID, func(tk *task.Task) {
		tk.Status = task.TaskCompleted
		tk.AgentName = "doneagent"
	})

	tests := []struct {
		name        string
		input       string
		wantErr     bool
		errMsg      string
		wantContent string
	}{
		{"send by ID", `{"to":"` + running.ID + `","message":"hello"}`, false, "", "success"},
		{"send by name", `{"to":"myagent","message":"hello by name"}`, false, "", "success"},
		{"no such agent", `{"to":"nonexistent","message":"hello"}`, false, "", "No agent found"},
		{"completed agent", `{"to":"doneagent","message":"hello"}`, false, "", "not running"},
		{"empty to", `{"to":"","message":"hello"}`, true, "to is required", ""},
		{"empty message", `{"to":"myagent","message":""}`, true, "message is required", ""},
		{"invalid json", `{bad`, true, "invalid input", ""},
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
}

func TestSendMessage_MessageDelivered(t *testing.T) {
	reg := setupTaskRegistry(t)
	tk := reg.Create("agent", "desc")
	reg.Update(tk.ID, func(tt *task.Task) {
		tt.Status = task.TaskRunning
		tt.AgentName = "testagent"
	})

	tl := &Tool{Tasks: reg}
	_, err := tl.Invoke(context.Background(), json.RawMessage(`{"to":"testagent","message":"hello world"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated, ok := reg.Get(tk.ID)
	if !ok {
		t.Fatal("task not found")
	}
	if len(updated.PendingMessages) != 1 {
		t.Fatalf("expected 1 pending message, got %d", len(updated.PendingMessages))
	}
	if updated.PendingMessages[0] != "hello world" {
		t.Fatalf("expected 'hello world', got %q", updated.PendingMessages[0])
	}
}

func TestSendMessage_MultipleMessages(t *testing.T) {
	reg := setupTaskRegistry(t)
	tk := reg.Create("agent", "desc")
	reg.Update(tk.ID, func(tt *task.Task) {
		tt.Status = task.TaskRunning
	})

	tl := &Tool{Tasks: reg}
	tl.Invoke(context.Background(), json.RawMessage(`{"to":"`+tk.ID+`","message":"msg1"}`), nil)
	tl.Invoke(context.Background(), json.RawMessage(`{"to":"`+tk.ID+`","message":"msg2"}`), nil)

	updated, _ := reg.Get(tk.ID)
	if len(updated.PendingMessages) != 2 {
		t.Fatalf("expected 2 pending messages, got %d", len(updated.PendingMessages))
	}
}
