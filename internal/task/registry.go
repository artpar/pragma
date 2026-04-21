package task

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/artpar/pragma/internal/observe"
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
		Notify:      make(chan struct{}, 1),
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

// ListRunningTeammates returns snapshots of running tasks that have an AgentName,
// sorted alphabetically by name. Matches TS getRunningTeammatesSorted().
func (r *Registry) ListRunningTeammates() []Task {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []Task
	for _, t := range r.tasks {
		observe.GlobalTrace("range r.tasks")
		if t.Status == TaskRunning && t.AgentName != "" {
			observe.GlobalTrace("if: t.Status == TaskRunning && t.AgentName != \"\"")
			result = append(result, t.snapshot())
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].AgentName < result[j].AgentName
	})
	observe.GlobalTrace("return: result")
	return result
}

// ListAllTeammates returns snapshots of all tasks that have an AgentName,
// regardless of status. Running tasks sort first, then completed, then others.
// Used by the /teams dialog to show full visibility including recently finished work.
func (r *Registry) ListAllTeammates() []Task {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []Task
	for _, t := range r.tasks {
		observe.GlobalTrace("range r.tasks")
		if t.AgentName != "" {
			observe.GlobalTrace("if: t.AgentName != \"\"")
			result = append(result, t.snapshot())
		}
	}
	sort.Slice(result, func(i, j int) bool {
		ri, rj := statusRank(result[i].Status), statusRank(result[j].Status)
		if ri != rj {
			return ri < rj
		}
		return result[i].AgentName < result[j].AgentName
	})
	observe.GlobalTrace("return: result")
	return result
}

func statusRank(s TaskStatus) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch s {
	case TaskRunning:
		observe.GlobalTrace("case: TaskRunning")
		return 0
	case TaskPending:
		observe.GlobalTrace("case: TaskPending")
		return 1
	case TaskCompleted:
		observe.GlobalTrace("case: TaskCompleted")
		return 2
	default:
		observe.GlobalTrace("default")
		return 3
	}
}

// NotifyTask signals a task's Notify channel (non-blocking).
// Used by SendMessage after appending to PendingMessages.
func (r *Registry) NotifyTask(id string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[id]
	if !ok || t.Notify == nil {
		observe.GlobalTrace("if: !ok || t.Notify == nil")
		return
	}
	select {
	case t.Notify <- struct{}{}:
		observe.GlobalTrace("select: t.Notify <- struct{}{}")
	default:
		observe.GlobalTrace("select: default")
	}
}

// DrainPendingMessages atomically reads and clears PendingMessages for a task.
// Returns nil if task not found or no messages pending.
func (r *Registry) DrainPendingMessages(id string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok || len(t.PendingMessages) == 0 {
		observe.GlobalTrace("if: !ok || len(t.PendingMessages) == 0")
		observe.GlobalTrace("return: nil")
		return nil
	}
	msgs := t.PendingMessages
	t.PendingMessages = nil
	observe.GlobalTrace("return: msgs")
	return msgs
}

// Shutdown requests graceful shutdown of a teammate task.
// Sets ShutdownRequested, signals Notify, waits up to timeout for completion.
// If timeout expires, force-cancels the task.
func (r *Registry) Shutdown(id string, timeout time.Duration) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.Lock()
	t, ok := r.tasks[id]
	if !ok {
		observe.GlobalTrace("if: !ok")
		r.mu.Unlock()
		observe.GlobalTrace("return: fmt.Errorf(\"task %q not found\", id)")
		return fmt.Errorf("task %q not found", id)
	}
	if t.Status != TaskRunning && t.Status != TaskPending {
		observe.GlobalTrace("if: t.Status != TaskRunning && t.Status != TaskPending")
		r.mu.Unlock()
		observe.GlobalTrace("return: fmt.Errorf(\"task %q is %s, cannot shutdown\", id, t.Status)")
		return fmt.Errorf("task %q is %s, cannot shutdown", id, t.Status)
	}
	t.ShutdownRequested = true
	t.UpdatedAt = time.Now()

	if t.Notify != nil {
		observe.GlobalTrace("if: t.Notify != nil")
		select {
		case t.Notify <- struct{}{}:
			observe.GlobalTrace("select: t.Notify <- struct{}{}")
		default:
			observe.GlobalTrace("select: default")
		}
	}
	r.mu.Unlock()

	deadline := time.After(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		observe.GlobalTrace("for: true")
		select {
		case <-deadline:
			observe.GlobalTrace("select: <-deadline")

			return r.Cancel(id)
		case <-ticker.C:
			observe.GlobalTrace("select: <-ticker.C")
			snap, exists := r.Get(id)
			if !exists {
				observe.GlobalTrace("return: nil")
				return nil
			}
			if snap.Status == TaskCompleted || snap.Status == TaskFailed || snap.Status == TaskCancelled {
				observe.GlobalTrace("return: nil")
				return nil
			}
		}
	}
}

// GetNotifyChannel returns the Notify channel for a task, or nil if not found.
// Used by teammate loop to select on message arrival.
func (r *Registry) GetNotifyChannel(id string) <-chan struct{} {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[id]
	if !ok || t.Notify == nil {
		observe.GlobalTrace("if: !ok || t.Notify == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: t.Notify")
	return t.Notify
}

// IsShutdownRequested checks if a task has been requested to shut down.
func (r *Registry) IsShutdownRequested(id string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[id]
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: false")
		return false
	}
	observe.GlobalTrace("return: t.ShutdownRequested")
	return t.ShutdownRequested
}

// Heartbeat updates the LastHeartbeat time for a running task.
// Called by the engine loop each iteration when running as a sub-agent.
func (r *Registry) Heartbeat(id string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok || t.Status != TaskRunning {
		observe.GlobalTrace("if: !ok || t.Status != TaskRunning")
		return
	}
	t.LastHeartbeat = time.Now()
}

// ReapDead marks running tasks as failed if their last heartbeat is older
// than timeout. Returns the IDs of reaped tasks. Tasks that never heartbeated
// (LastHeartbeat is zero) are not reaped — they may still be initializing.
func (r *Registry) ReapDead(timeout time.Duration) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	var reaped []string
	for id, t := range r.tasks {
		observe.GlobalTrace("range r.tasks")
		if t.Status != TaskRunning {
			observe.GlobalTrace("if: t.Status != TaskRunning")
			continue
		}
		if t.LastHeartbeat.IsZero() {
			observe.GlobalTrace("if: t.LastHeartbeat.IsZero()")
			continue
		}
		if now.Sub(t.LastHeartbeat) > timeout {
			observe.GlobalTrace("if: now.Sub(t.LastHeartbeat) > timeout")
			t.Status = TaskFailed
			t.Error = fmt.Sprintf("agent unresponsive (no heartbeat for %s)", timeout)
			t.UpdatedAt = now
			reaped = append(reaped, id)
		}
	}
	observe.GlobalTrace("return: reaped")
	return reaped
}
