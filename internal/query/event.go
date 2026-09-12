package query

import (
	"encoding/json"
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

// ModelRequestEvent signals that a provider request is starting.
type ModelRequestEvent struct {
	Model   string
	Attempt int
}

func (ModelRequestEvent) loopEventSealed() {}

// ModelResponseEvent signals that a provider request returned.
type ModelResponseEvent struct {
	Model      string
	StopReason model.StopReason
}

func (ModelResponseEvent) loopEventSealed() {}

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

// ToolOutputEvent carries incremental output of a still-running tool
// call (TUI-004): emitted while a foreground Bash command runs so the
// operator sees what the command is doing instead of a bare spinner.
// The tail is bounded (RunningOutputLines); the final ToolResultEvent
// supersedes it.
type ToolOutputEvent struct {
	ToolCallID string
	Output     string
	Running    bool
}

func (ToolOutputEvent) loopEventSealed() {}

// StructuredOutputEvent carries a runtime-validated structured final output.
type StructuredOutputEvent struct {
	ToolCallID string
	JSON       json.RawMessage
}

func (StructuredOutputEvent) loopEventSealed() {}

// UserMessageEvent carries a tool-requested user-visible message.
type UserMessageEvent struct {
	ToolCallID string
	Message    string
	Status     string
}

func (UserMessageEvent) loopEventSealed() {}

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

// CompactionFailedEvent signals that a single auto-compaction attempt failed.
// Emitted before the circuit breaker check so the TUI can show per-attempt warnings.
type CompactionFailedEvent struct {
	Attempt  int    // 1-based failure count
	MaxRetry int    // circuit breaker threshold
	ErrorMsg string // compaction error message
}

func (CompactionFailedEvent) loopEventSealed() {}

// CompactionDisabledEvent signals that auto-compaction circuit breaker tripped.
// The TUI should display a warning — tokens will grow unboundedly (GitHub #24677, #9579).
type CompactionDisabledEvent struct {
	ConsecutiveFailures int
}

func (CompactionDisabledEvent) loopEventSealed() {}

// CompactionStartedEvent signals that auto-compaction is starting.
// The TUI should show a spinner/status to avoid the silent 10-20s gap (GitHub #30115, #48740).
type CompactionStartedEvent struct{}

func (CompactionStartedEvent) loopEventSealed() {}

// OrchestrationStartedEvent signals the start of an orchestration FSM run.
type OrchestrationStartedEvent struct {
	Name    string
	Initial string
}

func (OrchestrationStartedEvent) loopEventSealed() {}

// OrchestrationStateStartedEvent signals that a state is starting.
type OrchestrationStateStartedEvent struct {
	StateID   string
	PersonaID string
	Control   string
}

func (OrchestrationStateStartedEvent) loopEventSealed() {}

// OrchestrationStateCompletedEvent signals that a persona state completed.
type OrchestrationStateCompletedEvent struct {
	StateID  string
	Duration time.Duration
}

func (OrchestrationStateCompletedEvent) loopEventSealed() {}

// OrchestrationControlEvent signals a control state action or emitted event.
type OrchestrationControlEvent struct {
	StateID string
	Control string
	Event   string
}

func (OrchestrationControlEvent) loopEventSealed() {}

// OrchestrationTransitionEvent signals a state transition.
type OrchestrationTransitionEvent struct {
	From  string
	Event string
	To    string
}

func (OrchestrationTransitionEvent) loopEventSealed() {}

// OrchestrationHandoffEvent records a handoff prompt path read/write.
type OrchestrationHandoffEvent struct {
	StateID    string
	From       string
	Event      string
	To         string
	ArtifactID string
	Path       string
	Direction  string
	Bytes      int64
	SHA256     string
}

func (OrchestrationHandoffEvent) loopEventSealed() {}

// OrchestrationCompletedEvent signals the FSM reached a terminal state.
type OrchestrationCompletedEvent struct {
	Name string
}

func (OrchestrationCompletedEvent) loopEventSealed() {}

// OrchestrationSnapshotEvent carries the orchestration-owned workflow projection.
type OrchestrationSnapshotEvent struct {
	Snapshot OrchestrationSnapshot
}

func (OrchestrationSnapshotEvent) loopEventSealed() {}

type OrchestrationSnapshot struct {
	Name        string                                 `json:"name,omitempty"`
	Initial     string                                 `json:"initial,omitempty"`
	Current     string                                 `json:"current,omitempty"`
	Completed   bool                                   `json:"completed,omitempty"`
	States      map[string]*OrchestrationStateSnapshot `json:"states"`
	Transitions []OrchestrationTransitionSnapshot      `json:"transitions"`
	Handoffs    []OrchestrationHandoffSnapshot         `json:"handoffs"`
}

type OrchestrationStateSnapshot struct {
	ID        string    `json:"id"`
	Persona   string    `json:"persona,omitempty"`
	Control   string    `json:"control,omitempty"`
	Status    string    `json:"status,omitempty"`
	LastEvent string    `json:"last_event,omitempty"`
	Started   time.Time `json:"started,omitempty"`
	Completed time.Time `json:"completed,omitempty"`
	Duration  string    `json:"duration,omitempty"`
}

type OrchestrationTransitionSnapshot struct {
	From  string `json:"from"`
	Event string `json:"event"`
	To    string `json:"to"`
}

type OrchestrationHandoffSnapshot struct {
	StateID    string `json:"state_id"`
	From       string `json:"from,omitempty"`
	Event      string `json:"event"`
	To         string `json:"to,omitempty"`
	ArtifactID string `json:"artifact_id,omitempty"`
	Path       string `json:"path"`
	Direction  string `json:"direction"`
	Bytes      int64  `json:"bytes,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
}

// AgentProgressEvent carries intermediate agent execution progress.
// Emitted during Agent tool execution so the TUI can show per-agent
// tool count, token count, and current activity instead of a static spinner.
// Addresses GitHub #11036, #30528, #27916, #3978.
type AgentProgressEvent struct {
	AgentID     string
	Description string
	ToolCount   int    // cumulative tool results received
	TokenCount  int    // cumulative tokens (input + output + cache)
	LastTool    string // name of last tool called
	Status      string // "initializing", "running", "completed", "error"
	Background  bool   // true for background agents
	Error       string
}

func (AgentProgressEvent) loopEventSealed() {}

// RetryEvent signals a retryable error is being retried.
// Emitted before the delay begins so the TUI can show a countdown.
// Addresses GitHub #2047 (exponential backoff), #26699 (session stuck on rate limit).
type RetryEvent struct {
	Attempt     int           // 1-based attempt number that just failed
	MaxAttempts int           // total allowed attempts (maxRetries+1)
	Delay       time.Duration // delay until next attempt
	Kind        ErrorKind     // rate_limit, overloaded, connection, server_error
	ErrorMsg    string        // full error text for verbose display
}

func (RetryEvent) loopEventSealed() {}

// ErrorEvent signals an error that terminated the loop.
// Kind and Guidance are populated for classified API errors;
// zero-value for legacy/non-API errors (backward compatible).
type ErrorEvent struct {
	Err      error
	Kind     ErrorKind // empty for legacy/unclassified errors
	Guidance string    // actionable hint, empty if none
}

func (ErrorEvent) loopEventSealed() {}
