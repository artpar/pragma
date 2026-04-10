package task

import (
	"context"
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
	ID          string             `json:"id"`
	Subject     string             `json:"subject"`
	Description string             `json:"description,omitempty"`
	Status      TaskStatus         `json:"status"`
	Result      string             `json:"result,omitempty"`
	Error       string             `json:"error,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
	TokensUsed  int                `json:"tokens_used,omitempty"`
	Cancel      context.CancelFunc `json:"-"` // not serialized — used to cancel running tasks
}
