package task

import (
	"context"
	"sync"
	"testing"
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
