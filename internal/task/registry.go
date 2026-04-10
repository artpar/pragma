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
	return &Registry{
		tasks: make(map[string]*Task),
		bus:   bus,
	}
}

// Create creates a new task in pending status and returns it.
func (r *Registry) Create(subject, description string) *Task {
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

	return t
}

// Get returns a task by ID.
func (r *Registry) Get(id string) (*Task, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[id]
	return t, ok
}

// List returns all tasks, optionally filtered by status.
// Tasks are returned in creation order (by ID).
func (r *Registry) List(status *TaskStatus) []*Task {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Task, 0, len(r.tasks))
	for _, t := range r.tasks {
		if status != nil && t.Status != *status {
			continue
		}
		result = append(result, t)
	}

	// Sort by creation time for stable ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result
}

// Update applies a mutation function to a task. Returns error if task not found.
func (r *Registry) Update(id string, fn func(*Task)) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	t, ok := r.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}
	fn(t)
	t.UpdatedAt = time.Now()
	return nil
}

// Cancel cancels a running task. Returns error if task not found or not cancellable.
func (r *Registry) Cancel(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	t, ok := r.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}
	if t.Status != TaskRunning && t.Status != TaskPending {
		return fmt.Errorf("task %q is %s, cannot cancel", id, t.Status)
	}
	if t.Cancel != nil {
		t.Cancel()
	}
	t.Status = TaskCancelled
	t.UpdatedAt = time.Now()
	return nil
}
