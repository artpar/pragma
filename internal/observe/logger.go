package observe

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// Level controls log verbosity.
type Level int

const (
	LevelTrace Level = iota // every StreamChunk, every byte
	LevelDebug              // API calls, tool executions, decisions
	LevelInfo               // turns, tool results, costs
	LevelWarn               // retries, slow operations, anomalies
	LevelError              // failures
)

// Format controls log output format.
type Format int

const (
	FormatJSON    Format = iota // structured JSONL
	FormatText                  // human-readable
	FormatCompact               // one-line summaries
)

// Logger is a Subscriber that writes formatted event output.
type Logger struct {
	writer   io.Writer
	level    Level
	topics   map[string]bool // nil = all topics
	format   Format
	mu       sync.Mutex
	writeErr error
}

// NewLogger creates a Logger subscriber.
func NewLogger(writer io.Writer, level Level, format Format, topics map[string]bool) *Logger {
	return &Logger{
		writer: writer,
		level:  level,
		topics: topics,
		format: format,
	}
}

func (l *Logger) HandleEvent(event Event) {
	kind := event.EventKind()

	evLevel := eventLevel(kind)
	if evLevel < l.level {
		return
	}

	if l.topics != nil && !l.topics[eventTopic(kind)] {
		return
	}

	switch l.format {
	case FormatJSON:
		l.writeJSON(event)
	case FormatText:
		l.writeText(event)
	case FormatCompact:
		l.writeCompact(event)
	}
}

func (l *Logger) writeJSON(event Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	data, err := json.Marshal(event)
	if err != nil {
		l.writeErr = err
		return
	}
	data = append(data, '\n')
	if _, err := l.writer.Write(data); err != nil {
		l.writeErr = err
	} else {
		l.writeErr = nil
	}
}

func (l *Logger) writeText(event Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ts := event.EventTimestamp().Format(time.RFC3339)
	line := fmt.Sprintf("[%s] %s trace=%s span=%s\n",
		ts, event.EventKind(), event.EventTraceID(), event.EventSpanID())
	if _, err := l.writer.Write([]byte(line)); err != nil {
		l.writeErr = err
	} else {
		l.writeErr = nil
	}
}

func (l *Logger) writeCompact(event Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	line := fmt.Sprintf("%s %s\n",
		event.EventTimestamp().Format("15:04:05"), event.EventKind())
	if _, err := l.writer.Write([]byte(line)); err != nil {
		l.writeErr = err
	} else {
		l.writeErr = nil
	}
}

// Err returns the first write error encountered, if any.
func (l *Logger) Err() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.writeErr
}

// eventLevel maps event kinds to log levels.
func eventLevel(kind string) Level {
	switch kind {
	case "APIStreamChunk":
		return LevelTrace
	case "ToolExecutionStarted", "APIRequestStarted", "ToolCallReceived",
		"MCPServerConnecting", "MCPToolCallStarted":
		return LevelDebug
	case "ToolExecutionCompleted", "APIRequestCompleted", "MessageAppended",
		"ConversationStarted", "SessionStarted", "SessionSaved", "SessionEnded",
		"MCPServerConnected", "MCPToolCallCompleted", "SubAgentSpawned",
		"SubAgentCompleted", "ToolBatchStarted", "ToolBatchCompleted",
		"ToolPermissionChecked", "ToolPermissionPrompted",
		"PermissionRuleMatched",
		"AgentMDLoaded", "SystemPromptBuilt":
		return LevelInfo
	case "APIRetryScheduled", "CompactionStarted", "CompactionCompleted",
		"MCPHealthCheck", "PermissionEscalated", "AgentMDNotFound":
		return LevelWarn
	case "APIRequestFailed", "ToolExecutionFailed", "CompactionFailed",
		"MCPServerFailed", "MCPServerDisconnected", "SubAgentFailed",
		"ErrorOccurred", "PermissionDenialEnforced":
		return LevelError
	case "ConversationForked":
		return LevelInfo
	default:
		return LevelInfo
	}
}

// eventTopic maps event kinds to topic categories for filtering.
func eventTopic(kind string) string {
	switch kind {
	case "ConversationStarted", "MessageAppended", "ConversationForked":
		return "conversation"
	case "APIRequestStarted", "APIStreamChunk", "APIRequestCompleted",
		"APIRequestFailed", "APIRetryScheduled":
		return "api"
	case "ToolCallReceived", "ToolPermissionChecked", "ToolPermissionPrompted",
		"ToolExecutionStarted", "ToolExecutionCompleted", "ToolExecutionFailed",
		"ToolBatchStarted", "ToolBatchCompleted":
		return "tool"
	case "CompactionStarted", "CompactionCompleted", "CompactionFailed":
		return "compaction"
	case "MCPServerConnecting", "MCPServerConnected", "MCPServerDisconnected",
		"MCPServerFailed", "MCPToolCallStarted", "MCPToolCallCompleted",
		"MCPHealthCheck":
		return "mcp"
	case "SessionStarted", "SessionSaved", "SessionEnded":
		return "session"
	case "SubAgentSpawned", "SubAgentCompleted", "SubAgentFailed":
		return "agent"
	case "PermissionRuleMatched", "PermissionEscalated", "PermissionDenialEnforced":
		return "permission"
	case "ErrorOccurred":
		return "error"
	case "AgentMDLoaded", "AgentMDNotFound", "SystemPromptBuilt":
		return "sysprompt"
	default:
		return "unknown"
	}
}
