package task

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRegistryCreateAndGet(t *testing.T) {
	reg := NewRegistry(nil)
	tk := reg.Create("test task", "description")
	if tk.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if tk.Status != TaskPending {
		t.Errorf("status: got %s, want pending", tk.Status)
	}

	got, ok := reg.Get(tk.ID)
	if !ok {
		t.Fatal("task not found")
	}
	if got.Subject != "test task" {
		t.Errorf("subject: got %q", got.Subject)
	}
}

func TestRegistryList(t *testing.T) {
	reg := NewRegistry(nil)
	reg.Create("task-1", "")
	tk2 := reg.Create("task-2", "")
	reg.Update(tk2.ID, func(tk *Task) { tk.Status = TaskRunning })

	all := reg.List(nil)
	if len(all) != 2 {
		t.Fatalf("got %d tasks, want 2", len(all))
	}

	running := TaskRunning
	filtered := reg.List(&running)
	if len(filtered) != 1 {
		t.Fatalf("got %d running tasks, want 1", len(filtered))
	}
	if filtered[0].Subject != "task-2" {
		t.Errorf("subject: got %q", filtered[0].Subject)
	}
}

func TestRegistryUpdate(t *testing.T) {
	reg := NewRegistry(nil)
	tk := reg.Create("task", "")

	err := reg.Update(tk.ID, func(tk *Task) {
		tk.Status = TaskCompleted
		tk.Result = "done"
	})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := reg.Get(tk.ID)
	if got.Status != TaskCompleted {
		t.Errorf("status: got %s", got.Status)
	}
	if got.Result != "done" {
		t.Errorf("result: got %q", got.Result)
	}
}

func TestRegistryCancel(t *testing.T) {
	reg := NewRegistry(nil)
	tk := reg.Create("task", "")

	ctx, cancel := context.WithCancel(context.Background())
	reg.Update(tk.ID, func(tk *Task) {
		tk.Status = TaskRunning
		tk.Cancel = cancel
	})

	err := reg.Cancel(tk.ID)
	if err != nil {
		t.Fatal(err)
	}

	got, _ := reg.Get(tk.ID)
	if got.Status != TaskCancelled {
		t.Errorf("status: got %s", got.Status)
	}

	// Context should be cancelled
	select {
	case <-ctx.Done():
		// good
	default:
		t.Error("context not cancelled")
	}
}

func TestRegistryCancelNotFound(t *testing.T) {
	reg := NewRegistry(nil)
	err := reg.Cancel("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestNotifyChannel(t *testing.T) {
	reg := NewRegistry(nil)
	tk := reg.Create("test", "")

	// Notify should be available
	ch := reg.GetNotifyChannel(tk.ID)
	if ch == nil {
		t.Fatal("expected non-nil notify channel")
	}

	// NotifyTask should signal the channel
	reg.NotifyTask(tk.ID)
	select {
	case <-ch:
		// good
	default:
		t.Error("expected notification on channel")
	}

	// Double notify should not block (buffered channel)
	reg.NotifyTask(tk.ID)
	reg.NotifyTask(tk.ID) // should not panic or block
}

func TestDrainPendingMessages(t *testing.T) {
	reg := NewRegistry(nil)
	tk := reg.Create("test", "")

	// No messages initially
	msgs := reg.DrainPendingMessages(tk.ID)
	if msgs != nil {
		t.Errorf("expected nil, got %v", msgs)
	}

	// Add messages
	reg.Update(tk.ID, func(tt *Task) {
		tt.PendingMessages = append(tt.PendingMessages, "hello", "world")
	})

	// Drain should return and clear
	msgs = reg.DrainPendingMessages(tk.ID)
	if len(msgs) != 2 || msgs[0] != "hello" || msgs[1] != "world" {
		t.Errorf("got %v, want [hello world]", msgs)
	}

	// Should be empty now
	msgs = reg.DrainPendingMessages(tk.ID)
	if msgs != nil {
		t.Errorf("expected nil after drain, got %v", msgs)
	}
}

func TestListRunningTeammates(t *testing.T) {
	reg := NewRegistry(nil)

	// Create tasks with various states
	tk1 := reg.Create("worker", "")
	reg.Update(tk1.ID, func(tt *Task) {
		tt.Status = TaskRunning
		tt.AgentName = "charlie"
	})

	tk2 := reg.Create("reviewer", "")
	reg.Update(tk2.ID, func(tt *Task) {
		tt.Status = TaskRunning
		tt.AgentName = "alice"
	})

	// Not a teammate (no AgentName)
	tk3 := reg.Create("background", "")
	reg.Update(tk3.ID, func(tt *Task) {
		tt.Status = TaskRunning
	})

	// Completed teammate
	tk4 := reg.Create("done", "")
	reg.Update(tk4.ID, func(tt *Task) {
		tt.Status = TaskCompleted
		tt.AgentName = "bob"
	})

	teammates := reg.ListRunningTeammates()
	if len(teammates) != 2 {
		t.Fatalf("got %d teammates, want 2", len(teammates))
	}
	// Should be sorted alphabetically
	if teammates[0].AgentName != "alice" {
		t.Errorf("first teammate: got %q, want alice", teammates[0].AgentName)
	}
	if teammates[1].AgentName != "charlie" {
		t.Errorf("second teammate: got %q, want charlie", teammates[1].AgentName)
	}
}

func TestShutdown(t *testing.T) {
	reg := NewRegistry(nil)
	tk := reg.Create("teammate", "")

	_, cancelFn := context.WithCancel(context.Background())
	reg.Update(tk.ID, func(tt *Task) {
		tt.Status = TaskRunning
		tt.AgentName = "test-mate"
		tt.Cancel = cancelFn
	})

	// Simulate teammate completing on shutdown request
	go func() {
		// Wait for shutdown signal
		ch := reg.GetNotifyChannel(tk.ID)
		<-ch
		if reg.IsShutdownRequested(tk.ID) {
			reg.Update(tk.ID, func(tt *Task) {
				tt.Status = TaskCompleted
			})
		}
	}()

	err := reg.Shutdown(tk.ID, 5*time.Second)
	if err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	got, _ := reg.Get(tk.ID)
	if got.Status != TaskCompleted {
		t.Errorf("status: got %s, want completed", got.Status)
	}
}

func TestShutdownTimeout(t *testing.T) {
	reg := NewRegistry(nil)
	tk := reg.Create("stuck-teammate", "")

	ctx, cancelFn := context.WithCancel(context.Background())
	reg.Update(tk.ID, func(tt *Task) {
		tt.Status = TaskRunning
		tt.AgentName = "stuck"
		tt.Cancel = cancelFn
	})

	// Don't respond to shutdown — should force cancel after timeout
	err := reg.Shutdown(tk.ID, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	got, _ := reg.Get(tk.ID)
	if got.Status != TaskCancelled {
		t.Errorf("status: got %s, want cancelled", got.Status)
	}

	// Context should be cancelled
	select {
	case <-ctx.Done():
		// good
	default:
		t.Error("context not cancelled after timeout")
	}
}

func TestIsShutdownRequested(t *testing.T) {
	reg := NewRegistry(nil)
	tk := reg.Create("test", "")

	if reg.IsShutdownRequested(tk.ID) {
		t.Error("should not be shutdown requested initially")
	}

	reg.Update(tk.ID, func(tt *Task) {
		tt.ShutdownRequested = true
	})

	if !reg.IsShutdownRequested(tk.ID) {
		t.Error("should be shutdown requested after setting")
	}
}

func TestRegistryConcurrentAccess(t *testing.T) {
	reg := NewRegistry(nil)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tk := reg.Create("task", "")
			reg.Update(tk.ID, func(tk *Task) {
				tk.Status = TaskRunning
			})
			reg.List(nil)
			reg.Get(tk.ID)
		}()
	}
	wg.Wait()

	if len(reg.List(nil)) != 100 {
		t.Errorf("got %d tasks, want 100", len(reg.List(nil)))
	}
}
