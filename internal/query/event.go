package query

import "github.com/artpar/gogent/internal/model"

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
type ToolResultEvent struct {
	Result model.ToolResultPart
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

// ErrorEvent signals an error that terminated the loop.
type ErrorEvent struct {
	Err error
}

func (ErrorEvent) loopEventSealed() {}
