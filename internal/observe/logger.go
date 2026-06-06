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
	detail := eventDetail(event)
	var line string
	if detail != "" {
		line = fmt.Sprintf("[%s] %s %s\n", ts, event.EventKind(), detail)
	} else {
		line = fmt.Sprintf("[%s] %s\n", ts, event.EventKind())
	}
	if _, err := l.writer.Write([]byte(line)); err != nil {
		l.writeErr = err
	} else {
		l.writeErr = nil
	}
}

// eventDetail extracts a human-readable summary of an event's key fields.
func eventDetail(event Event) string {
	switch e := event.(type) {
	case APIRequestStarted:
		return fmt.Sprintf("model=%s msgs=%d tools=%d tokens~%d", e.Model, e.MessageCount, e.ToolCount, e.TokenEstimate)
	case APIRequestCompleted:
		return fmt.Sprintf("stop=%s in=%d out=%d cache_create=%d cache_read=%d dur=%dms",
			e.StopReason, e.Usage.InputTokens, e.Usage.OutputTokens,
			e.Usage.CacheCreationInputTokens, e.Usage.CacheReadInputTokens, e.DurationMs)
	case APIRequestFailed:
		return fmt.Sprintf("type=%s retryable=%v attempt=%d err=%s", e.ErrorType, e.Retryable, e.Attempt, e.ErrorMessage)
	case APIRetryScheduled:
		return fmt.Sprintf("attempt=%d delay=%dms reason=%s", e.Attempt, e.DelayMs, e.Reason)
	case ToolExecutionStarted:
		return fmt.Sprintf("tool=%s call=%s input_bytes=%d", e.ToolName, e.ToolCallID, e.InputSizeBytes)
	case ToolExecutionCompleted:
		return fmt.Sprintf("tool=%s dur=%dms error=%v", e.ToolName, e.DurationMs, e.IsError)
	case ToolExecutionFailed:
		return fmt.Sprintf("tool=%s err=%s", e.ToolName, e.ErrorMessage)
	case ToolPermissionChecked:
		return fmt.Sprintf("tool=%s decision=%s", e.ToolName, e.Decision)
	case AskPromptRequested:
		return fmt.Sprintf("tool=%s call=%s ask=%s questions=%d", e.ToolName, e.ToolCallID, e.AskID, len(e.Questions))
	case AskPromptResolved:
		return fmt.Sprintf("tool=%s call=%s ask=%s answers=%d dur=%dms", e.ToolName, e.ToolCallID, e.AskID, e.AnswerCount, e.DurationMs)
	case AskPromptCancelled:
		return fmt.Sprintf("tool=%s call=%s ask=%s dur=%dms err=%s", e.ToolName, e.ToolCallID, e.AskID, e.DurationMs, e.ErrorMessage)
	case ErrorOccurred:
		return fmt.Sprintf("[%s] %s: %s", e.Severity, e.Component, e.ErrorMessage)
	case SlashCommandExecuted:
		return fmt.Sprintf("cmd=/%s success=%v dur=%dms", e.CommandName, e.Success, e.DurationMs)
	case CompactionCompleted:
		return fmt.Sprintf("post=%d summarized=%d dur=%dms", e.PostTokenCount, e.SummarizedCount, e.DurationMs)
	case MCPServerConnected:
		return fmt.Sprintf("server=%s tools=%d", e.ServerName, e.ToolCount)
	case MCPServerFailed:
		return fmt.Sprintf("server=%s err=%s", e.ServerName, e.ErrorMessage)
	case AgentMDLoaded:
		return fmt.Sprintf("path=%s scope=%s bytes=%d", e.Path, e.Scope, e.Bytes)
	case AgentMDNotFound:
		return fmt.Sprintf("path=%s scope=%s", e.Path, e.Scope)
	case SystemPromptBuilt:
		return fmt.Sprintf("blocks=%d bytes=%d", e.BlockCount, e.TotalBytes)
	case SubAgentSpawned:
		return fmt.Sprintf("agent=%s model=%s", e.AgentName, e.Model)
	case SubAgentCompleted:
		return fmt.Sprintf("id=%s turns=%d dur=%dms", e.SubAgentID, e.TurnCount, e.DurationMs)
	case SessionStarted:
		return fmt.Sprintf("id=%s", e.SessionID)
	case SessionSaved:
		return fmt.Sprintf("id=%s msgs=%d", e.SessionID, e.MessageCount)
	case FlowTrace:
		return fmt.Sprintf("[%s.%s] %s", e.Component, e.Function, e.Message)
	default:
		return ""
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
	case "APIStreamChunk", "FlowTrace":
		return LevelTrace
	case "ToolExecutionStarted", "APIRequestStarted", "ToolCallReceived",
		"MCPServerConnecting", "MCPToolCallStarted":
		return LevelDebug
	case "ToolExecutionCompleted", "APIRequestCompleted", "MessageAppended",
		"ConversationStarted", "SessionStarted", "SessionSaved", "SessionEnded",
		"MCPServerConnected", "MCPToolCallCompleted", "SubAgentSpawned",
		"SubAgentCompleted", "ToolBatchStarted", "ToolBatchCompleted",
		"ToolPermissionChecked", "ToolPermissionPromptStarted", "ToolPermissionPrompted",
		"AskPromptRequested", "AskPromptResolved", "AskPromptCancelled",
		"PermissionRuleMatched", "SlashCommandExecuted",
		"AgentMDLoaded", "SystemPromptBuilt",
		"BriefMessageSent", "McpOAuthCompleted":
		return LevelInfo
	case "APIRetryScheduled", "CompactionStarted", "CompactionCompleted",
		"MCPHealthCheck", "PermissionEscalated", "AgentMDNotFound",
		"McpOAuthStarted":
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
	case "ToolCallReceived", "ToolPermissionChecked", "ToolPermissionPromptStarted", "ToolPermissionPrompted",
		"AskPromptRequested", "AskPromptResolved", "AskPromptCancelled",
		"ToolExecutionStarted", "ToolExecutionCompleted", "ToolExecutionFailed",
		"ToolBatchStarted", "ToolBatchCompleted":
		return "tool"
	case "CompactionStarted", "CompactionCompleted", "CompactionFailed":
		return "compaction"
	case "SlashCommandExecuted":
		return "command"
	case "MCPServerConnecting", "MCPServerConnected", "MCPServerDisconnected",
		"MCPServerFailed", "MCPToolCallStarted", "MCPToolCallCompleted",
		"MCPHealthCheck", "McpOAuthStarted", "McpOAuthCompleted":
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
	case "BriefMessageSent":
		return "brief"
	case "FlowTrace":
		return "trace"
	default:
		return "unknown"
	}
}
