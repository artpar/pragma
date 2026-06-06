package observe

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"
)

// Event is a sealed interface for all observable events.
// Only types in this package implement it.
type Event interface {
	eventSealed()
	EventKind() string
	EventTimestamp() time.Time
	EventTraceID() string
	EventSpanID() string
	EventParentSpanID() string
}

// EventHeader is embedded by all concrete event types.
// It provides the common fields and accessor implementations.
type EventHeader struct {
	Kind       string    `json:"kind"`
	Time       time.Time `json:"time"`
	Trace      string    `json:"trace_id"`
	Span       string    `json:"span_id"`
	ParentSpan string    `json:"parent_span_id,omitempty"`
	AgentID    string    `json:"agent_id,omitempty"`
}

func (h EventHeader) EventKind() string         { return h.Kind }
func (h EventHeader) EventTimestamp() time.Time { return h.Time }
func (h EventHeader) EventTraceID() string      { return h.Trace }
func (h EventHeader) EventSpanID() string       { return h.Span }
func (h EventHeader) EventParentSpanID() string { return h.ParentSpan }

// NewEventHeader creates an EventHeader with the given kind and trace/span IDs.
func NewEventHeader(kind, traceID, spanID, parentSpanID string) EventHeader {
	return EventHeader{
		Kind:       kind,
		Time:       time.Now(),
		Trace:      traceID,
		Span:       spanID,
		ParentSpan: parentSpanID,
	}
}

// NewSpanID generates a random span ID.
func NewSpanID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return fmt.Sprintf("%x", b[:])
}

// NewTraceID generates a random trace ID.
func NewTraceID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return fmt.Sprintf("%x", b[:])
}

// UnmarshalEvent deserializes an Event from JSON, dispatching on the "kind" field.
func UnmarshalEvent(data []byte) (Event, error) {
	var peek struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(data, &peek); err != nil {
		return nil, fmt.Errorf("unmarshal event kind: %w", err)
	}

	var event Event
	switch peek.Kind {
	// Conversation events
	case "ConversationStarted":
		var e ConversationStarted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "MessageAppended":
		var e MessageAppended
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "UserTurnAccepted":
		var e UserTurnAccepted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "ConversationForked":
		var e ConversationForked
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	// API events
	case "APIRequestStarted":
		var e APIRequestStarted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "APIStreamChunk":
		var e APIStreamChunk
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "APIRequestCompleted":
		var e APIRequestCompleted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "APIRequestFailed":
		var e APIRequestFailed
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "APIRetryScheduled":
		var e APIRetryScheduled
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	// Tool events
	case "ToolCallReceived":
		var e ToolCallReceived
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "ToolPermissionChecked":
		var e ToolPermissionChecked
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "ToolPermissionPromptStarted":
		var e ToolPermissionPromptStarted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "ToolPermissionPrompted":
		var e ToolPermissionPrompted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "AskPromptRequested":
		var e AskPromptRequested
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "AskPromptResolved":
		var e AskPromptResolved
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "AskPromptCancelled":
		var e AskPromptCancelled
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "ToolExecutionStarted":
		var e ToolExecutionStarted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "ToolExecutionCompleted":
		var e ToolExecutionCompleted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "ToolExecutionFailed":
		var e ToolExecutionFailed
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "ToolBatchStarted":
		var e ToolBatchStarted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "ToolBatchCompleted":
		var e ToolBatchCompleted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	// Compaction events
	case "CompactionStarted":
		var e CompactionStarted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "CompactionCompleted":
		var e CompactionCompleted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "CompactionFailed":
		var e CompactionFailed
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	// Slash command events
	case "SlashCommandExecuted":
		var e SlashCommandExecuted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	// MCP events
	case "MCPServerConnecting":
		var e MCPServerConnecting
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "MCPServerConnected":
		var e MCPServerConnected
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "MCPServerDisconnected":
		var e MCPServerDisconnected
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "MCPServerFailed":
		var e MCPServerFailed
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "MCPToolCallStarted":
		var e MCPToolCallStarted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "MCPToolCallCompleted":
		var e MCPToolCallCompleted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "MCPHealthCheck":
		var e MCPHealthCheck
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	// Session events
	case "SessionStarted":
		var e SessionStarted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "SessionSaved":
		var e SessionSaved
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "SessionEnded":
		var e SessionEnded
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	// Agent events
	case "SubAgentSpawned":
		var e SubAgentSpawned
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "SubAgentCompleted":
		var e SubAgentCompleted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "SubAgentFailed":
		var e SubAgentFailed
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	// Error events
	case "ErrorOccurred":
		var e ErrorOccurred
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	// Permission audit events
	case "PermissionRuleMatched":
		var e PermissionRuleMatched
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "PermissionEscalated":
		var e PermissionEscalated
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "PermissionDecisionFinal":
		var e PermissionDecisionFinal
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "PermissionDenialEnforced":
		var e PermissionDenialEnforced
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "AgentMDLoaded":
		var e AgentMDLoaded
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "AgentMDNotFound":
		var e AgentMDNotFound
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "SystemPromptBuilt":
		var e SystemPromptBuilt
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "BriefMessageSent":
		var e BriefMessageSent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "McpOAuthStarted":
		var e McpOAuthStarted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "McpOAuthCompleted":
		var e McpOAuthCompleted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "FlowTrace":
		var e FlowTrace
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	// Hook events
	case "HookExecuted":
		var e HookExecuted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "HookBlocked":
		var e HookBlocked
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	// Permission persistence
	case "PermissionPersisted":
		var e PermissionPersisted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	// Team events
	case "TeamCreated":
		var e TeamCreated
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "TeamDeleted":
		var e TeamDeleted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	// Lifecycle events
	case "LifecycleStepStarted":
		var e LifecycleStepStarted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "LifecycleNodeCompleted":
		var e LifecycleNodeCompleted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "LifecycleTransition":
		var e LifecycleTransition
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "LifecycleCompleted":
		var e LifecycleCompleted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	// Orchestration events
	case "OrchestrationStarted":
		var e OrchestrationStarted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "OrchestrationStateStarted":
		var e OrchestrationStateStarted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "OrchestrationStateCompleted":
		var e OrchestrationStateCompleted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "OrchestrationControl":
		var e OrchestrationControl
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "OrchestrationTransition":
		var e OrchestrationTransition
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "OrchestrationHandoff":
		var e OrchestrationHandoff
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	case "OrchestrationCompleted":
		var e OrchestrationCompleted
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		event = e
	default:
		return nil, fmt.Errorf("unknown event kind: %q", peek.Kind)
	}
	return event, nil
}
