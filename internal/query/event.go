package query

import (
	"time"

	"github.com/artpar/pragma/internal/model"
)

// LoopEvent is the sealed interface for events emitted by Engine.Run().
// These are query-local events consumed by the caller — distinct from
// observe.Event types which go to the EventBus for logging/recording.
type LoopEvent interface {
	loopEventSealed()
}

// TextEvent carries a streaming text delta from the LLM response.
type TextEvent struct {
	Text string
}

func (TextEvent) loopEventSealed() {}

// ThinkingEvent carries a streaming thinking/reasoning delta.
type ThinkingEvent struct {
	Text string
}

func (ThinkingEvent) loopEventSealed() {}

// ToolCallEvent signals a tool call is about to be executed.
type ToolCallEvent struct {
	Call model.ToolCallPart
}

func (ToolCallEvent) loopEventSealed() {}

// ToolResultEvent carries the result of a tool execution.
// Display carries optional TUI-only rendering content from the tool's InvokeResult.
type ToolResultEvent struct {
	Result  model.ToolResultPart
	Display string
}

func (ToolResultEvent) loopEventSealed() {}

// TurnCompleteEvent signals the agentic loop has finished.
type TurnCompleteEvent struct {
	Response   model.Response
	StopReason model.StopReason
}

func (TurnCompleteEvent) loopEventSealed() {}

// CompactionEvent signals that auto-compaction occurred during the agentic loop.
type CompactionEvent struct {
	PreTokens  int
	PostTokens int
}

func (CompactionEvent) loopEventSealed() {}

// CompactionDisabledEvent signals that auto-compaction circuit breaker tripped.
// The TUI should display a warning — tokens will grow unboundedly (GitHub #24677, #9579).
type CompactionDisabledEvent struct {
	ConsecutiveFailures int
}

func (CompactionDisabledEvent) loopEventSealed() {}

// LifecycleProgressEvent carries intermediate lifecycle graph progress.
// Emitted during LifecycleRun tool execution so the TUI can show step-by-step
// progress instead of a static spinner (addresses GitHub #11036, #30528).
type LifecycleProgressEvent struct {
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

func (LifecycleProgressEvent) loopEventSealed() {}

// ErrorEvent signals an error that terminated the loop.
type ErrorEvent struct {
	Err error
}

func (ErrorEvent) loopEventSealed() {}
