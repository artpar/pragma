package tool

import "time"

// ProgressEvent carries intermediate progress from a long-running tool.
// Kind selects the event family: "" or "lifecycle" for lifecycle graph progress,
// "agent" for sub-agent execution progress. Each family uses its own field subset.
type ProgressEvent struct {
	Kind string // "" or "lifecycle" = lifecycle graph; "agent" = sub-agent progress

	// Lifecycle fields (Kind == "" or "lifecycle")
	Step     int
	Node     string
	Nodes    []string      // pending nodes (step_started)
	Status   string        // lifecycle: "step_started","node_completed","transition","completed"; agent: "initializing","running","completed","error"
	Duration time.Duration // node_completed only
	Error    string        // node_completed / completed errors
	FromNode string        // transition only
	ToNode   string        // transition only
	RouteKey string        // transition only

	// Agent fields (Kind == "agent")
	AgentID     string // task ID — unique per agent invocation
	Description string // short description from AgentInput.Description
	ToolCount   int    // cumulative tool_result count
	TokenCount  int    // cumulative: latestInputTokens + cumulativeOutputTokens
	LastTool    string // name of last tool called
	Background  bool   // true for run_in_background agents
}

// ProgressReporter is a write-only channel for emitting progress events.
type ProgressReporter chan<- ProgressEvent

// ProgressSource is an optional interface on StateSnapshot.
// Tools that support progress reporting type-assert for it.
// Follows the provider.TokenCounter pattern — optional, no interface changes.
type ProgressSource interface {
	Progress() ProgressReporter
}
