package task

import (
	"context"
	"github.com/artpar/pragma/internal/observe"
	"time"
)

// TaskStatus represents the lifecycle state of a task.
type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskCancelled TaskStatus = "cancelled"
)

// Task represents a trackable unit of work (e.g., a background sub-agent).
type Task struct {
	ID                string             `json:"id"`
	Subject           string             `json:"subject"`
	Description       string             `json:"description,omitempty"`
	Status            TaskStatus         `json:"status"`
	Result            string             `json:"result,omitempty"`
	Error             string             `json:"error,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
	TokensUsed        int                `json:"tokens_used,omitempty"`
	Cancel            context.CancelFunc `json:"-"`                    // not serialized — used to cancel running tasks
	PendingMessages   []string           `json:"-"`                    // messages from SendMessage, consumed between turns
	AgentName         string             `json:"agent_name,omitempty"` // for name-based lookup by SendMessage
	Notify            chan struct{}       `json:"-"`                    // signaled when PendingMessages updated; buffered(1)
	ShutdownRequested bool               `json:"-"`                    // set by Shutdown(); teammate loop checks this
}

// snapshot returns a deep copy of the Task safe for reading outside the registry lock.
// Cancel is nil in the copy — it's internal to the registry's Cancel method.
func (t *Task) snapshot() Task {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cp := *t
	cp.Cancel = nil
	cp.Notify = nil
	if len(t.PendingMessages) > 0 {
		observe.GlobalTrace("if: len(t.PendingMessages) > 0")
		cp.PendingMessages = make([]string, len(t.PendingMessages))
		copy(cp.PendingMessages, t.PendingMessages)
	}
	observe.GlobalTrace("return: cp")
	return cp
}
