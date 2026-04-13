package task

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/artpar/gogent/internal/observe"
)

// Registry manages the lifecycle of tasks. Thread-safe.
type Registry struct {
	mu    sync.RWMutex
	tasks map[string]*Task
	bus   *observe.EventBus
	seq   atomic.Int64
}

// NewRegistry creates a task Registry.
func NewRegistry(bus *observe.EventBus) *Registry {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Registry{\n\ttasks:\tmake(map[string]*Task),\n\tbus:\tbus,\n}")
	return &Registry{
		tasks: make(map[string]*Task),
		bus:   bus,
	}
}

// Create creates a new task in pending status and returns a snapshot.
func (r *Registry) Create(subject, description string) Task {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	id := fmt.Sprintf("task-%d", r.seq.Add(1))
	now := time.Now()
	t := &Task{
		ID:          id,
		Subject:     subject,
		Description: description,
		Status:      TaskPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	r.mu.Lock()
	r.tasks[id] = t
	r.mu.Unlock()
	observe.GlobalTrace("return: t.snapshot()")

	return t.snapshot()
}

// Get returns a snapshot of a task by ID. The returned Task is a deep copy
// safe to read without synchronization.
func (r *Registry) Get(id string) (Task, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[id]
	if !ok {
		observe.GlobalTrace("return: Task{}, false")
		return Task{}, false
	}
	observe.GlobalTrace("return: t.snapshot(), true")
	return t.snapshot(), true
}

// List returns snapshots of all tasks, optionally filtered by status.
// Tasks are returned in creation order (by ID).
func (r *Registry) List(status *TaskStatus) []Task {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Task, 0, len(r.tasks))
	for _, t := range r.tasks {
		observe.GlobalTrace("range r.tasks")
		if status != nil && t.Status != *status {
			observe.GlobalTrace("if: status != nil && t.Status != *status")
			continue
		}
		result = append(result, t.snapshot())
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	observe.GlobalTrace("return: result")

	return result
}

// Update applies a mutation function to a task. Returns error if task not found.
func (r *Registry) Update(id string, fn func(*Task)) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.Lock()
	defer r.mu.Unlock()

	t, ok := r.tasks[id]
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: fmt.Errorf(\"task %q not found\", id)")
		return fmt.Errorf("task %q not found", id)
	}
	fn(t)
	t.UpdatedAt = time.Now()
	observe.GlobalTrace("return: nil")
	return nil
}

// GetByName returns a snapshot of the first task with a matching AgentName.
func (r *Registry) GetByName(name string) (Task, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.tasks {
		observe.GlobalTrace("range r.tasks")
		if t.AgentName == name {
			observe.GlobalTrace("if: t.AgentName == name")
			observe.GlobalTrace("return: t.snapshot(), true")
			return t.snapshot(), true
		}
	}
	observe.GlobalTrace("return: Task{}, false")
	return Task{}, false
}

// Cancel cancels a running task. Returns error if task not found or not cancellable.
func (r *Registry) Cancel(id string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.Lock()
	defer r.mu.Unlock()

	t, ok := r.tasks[id]
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: fmt.Errorf(\"task %q not found\", id)")
		return fmt.Errorf("task %q not found", id)
	}
	if t.Status != TaskRunning && t.Status != TaskPending {
		observe.GlobalTrace("if: t.Status != TaskRunning && t.Status != TaskPending")
		observe.GlobalTrace("return: fmt.Errorf(\"task %q is %s, cannot cancel\", id, t.Status)")
		return fmt.Errorf("task %q is %s, cannot cancel", id, t.Status)
	}
	if t.Cancel != nil {
		observe.GlobalTrace("if: t.Cancel != nil")
		t.Cancel()
	}
	t.Status = TaskCancelled
	t.UpdatedAt = time.Now()
	observe.GlobalTrace("return: nil")
	return nil
}
