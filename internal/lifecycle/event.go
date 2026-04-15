package lifecycle

// Event types for the lifecycle framework are defined in
// internal/observe/event_catalog.go (LifecycleStepStarted,
// LifecycleNodeCompleted, LifecycleTransition, LifecycleCompleted)
// because they implement the sealed observe.Event interface.
//
// This file defines only the streaming event type used by Executor.Stream().

// ExecutionEvent is the streaming event type for Executor.Stream().
type ExecutionEvent struct {
	Type  string   `json:"type"` // "step_started", "node_completed", "transition", "completed"
	Step  int      `json:"step"`
	Node  string   `json:"node,omitempty"`
	Nodes []string `json:"nodes,omitempty"` // pending nodes for step_started
	State State    `json:"state,omitempty"`
	Err   error    `json:"-"`
}
