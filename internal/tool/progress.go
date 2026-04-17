package tool

import "time"

// ProgressEvent carries intermediate progress from a long-running tool.
// Used by LifecycleRun to forward executor events to the TUI via the engine.
type ProgressEvent struct {
	Step     int
	Node     string
	Nodes    []string      // pending nodes (step_started)
	Status   string        // "step_started", "node_completed", "transition", "completed"
	Duration time.Duration // node_completed only
	Error    string        // node_completed / completed errors
	FromNode string        // transition only
	ToNode   string        // transition only
	RouteKey string        // transition only
}

// ProgressReporter is a write-only channel for emitting progress events.
type ProgressReporter chan<- ProgressEvent

// ProgressSource is an optional interface on StateSnapshot.
// Tools that support progress reporting type-assert for it.
// Follows the provider.TokenCounter pattern — optional, no interface changes.
type ProgressSource interface {
	Progress() ProgressReporter
}
