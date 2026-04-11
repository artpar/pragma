package observe

import "github.com/artpar/gogent/internal/model"

// --- Conversation Events ---

type ConversationStarted struct {
	EventHeader
	ConversationID string `json:"conversation_id"`
	Model          string `json:"model"`
	Provider       string `json:"provider"`
	WorkDir        string `json:"work_dir"`
}

func (ConversationStarted) eventSealed() {}

type MessageAppended struct {
	EventHeader
	MessageID     string   `json:"message_id"`
	Role          string   `json:"role"`
	ContentTypes  []string `json:"content_types"`
	TokenEstimate int      `json:"token_estimate"`
}

func (MessageAppended) eventSealed() {}

type ConversationForked struct {
	EventHeader
	ParentConvID string `json:"parent_conv_id"`
	ChildConvID  string `json:"child_conv_id"`
	AgentName    string `json:"agent_name"`
}

func (ConversationForked) eventSealed() {}

// --- API Events ---

type APIRequestStarted struct {
	EventHeader
	Model         string `json:"model"`
	MessageCount  int    `json:"message_count"`
	ToolCount     int    `json:"tool_count"`
	TokenEstimate int    `json:"token_estimate"`
}

func (APIRequestStarted) eventSealed() {}

type APIStreamChunk struct {
	EventHeader
	ChunkType  string `json:"chunk_type"`
	BytesDelta int    `json:"bytes_delta"`
}

func (APIStreamChunk) eventSealed() {}

type APIRequestCompleted struct {
	EventHeader
	StopReason model.StopReason `json:"stop_reason"`
	Usage      model.TokenUsage `json:"usage"`
	DurationMs int64            `json:"duration_ms"`
	Model      string           `json:"model"`
}

func (APIRequestCompleted) eventSealed() {}

type APIRequestFailed struct {
	EventHeader
	ErrorType    string `json:"error_type"`
	ErrorMessage string `json:"error_message"`
	Retryable    bool   `json:"retryable"`
	Attempt      int    `json:"attempt"`
}

func (APIRequestFailed) eventSealed() {}

type APIRetryScheduled struct {
	EventHeader
	Attempt int    `json:"attempt"`
	DelayMs int64  `json:"delay_ms"`
	Reason  string `json:"reason"`
}

func (APIRetryScheduled) eventSealed() {}

// --- Tool Events ---

type ToolCallReceived struct {
	EventHeader
	ToolCallID     string `json:"tool_call_id"`
	ToolName       string `json:"tool_name"`
	InputSizeBytes int    `json:"input_size_bytes"`
}

func (ToolCallReceived) eventSealed() {}

type ToolPermissionChecked struct {
	EventHeader
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	Decision   string `json:"decision"`
	Rule       string `json:"rule"`
	Source     string `json:"source"`
}

func (ToolPermissionChecked) eventSealed() {}

type ToolPermissionPrompted struct {
	EventHeader
	ToolCallID   string `json:"tool_call_id"`
	ToolName     string `json:"tool_name"`
	UserDecision string `json:"user_decision"`
	DurationMs   int64  `json:"duration_ms"`
}

func (ToolPermissionPrompted) eventSealed() {}

type ToolExecutionStarted struct {
	EventHeader
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	Concurrent bool   `json:"concurrent"`
}

func (ToolExecutionStarted) eventSealed() {}

type ToolExecutionCompleted struct {
	EventHeader
	ToolCallID      string `json:"tool_call_id"`
	ToolName        string `json:"tool_name"`
	DurationMs      int64  `json:"duration_ms"`
	OutputSizeBytes int    `json:"output_size_bytes"`
	IsError         bool   `json:"is_error"`
}

func (ToolExecutionCompleted) eventSealed() {}

type ToolExecutionFailed struct {
	EventHeader
	ToolCallID   string `json:"tool_call_id"`
	ToolName     string `json:"tool_name"`
	ErrorType    string `json:"error_type"`
	ErrorMessage string `json:"error_message"`
}

func (ToolExecutionFailed) eventSealed() {}

type ToolBatchStarted struct {
	EventHeader
	ConcurrentCount int `json:"concurrent_count"`
	SerialCount     int `json:"serial_count"`
	TotalCount      int `json:"total_count"`
}

func (ToolBatchStarted) eventSealed() {}

type ToolBatchCompleted struct {
	EventHeader
	TotalDurationMs      int64 `json:"total_duration_ms"`
	ConcurrentDurationMs int64 `json:"concurrent_duration_ms"`
	SerialDurationMs     int64 `json:"serial_duration_ms"`
}

func (ToolBatchCompleted) eventSealed() {}

// --- Compaction Events ---

type CompactionStarted struct {
	EventHeader
	PreTokenCount int `json:"pre_token_count"`
	BudgetTokens  int `json:"budget_tokens"`
	MessageCount  int `json:"message_count"`
}

func (CompactionStarted) eventSealed() {}

type CompactionCompleted struct {
	EventHeader
	PostTokenCount  int   `json:"post_token_count"`
	SummarizedCount int   `json:"summarized_count"`
	DurationMs      int64 `json:"duration_ms"`
}

func (CompactionCompleted) eventSealed() {}

type CompactionFailed struct {
	EventHeader
	ErrorType    string `json:"error_type"`
	ErrorMessage string `json:"error_message"`
}

func (CompactionFailed) eventSealed() {}

// --- MCP Events ---

type MCPServerConnecting struct {
	EventHeader
	ServerName string `json:"server_name"`
	Transport  string `json:"transport"`
}

func (MCPServerConnecting) eventSealed() {}

type MCPServerConnected struct {
	EventHeader
	ServerName string `json:"server_name"`
	ToolCount  int    `json:"tool_count"`
	DurationMs int64  `json:"duration_ms"`
}

func (MCPServerConnected) eventSealed() {}

type MCPServerDisconnected struct {
	EventHeader
	ServerName string `json:"server_name"`
	Reason     string `json:"reason"`
}

func (MCPServerDisconnected) eventSealed() {}

type MCPServerFailed struct {
	EventHeader
	ServerName   string `json:"server_name"`
	ErrorType    string `json:"error_type"`
	ErrorMessage string `json:"error_message"`
}

func (MCPServerFailed) eventSealed() {}

type MCPToolCallStarted struct {
	EventHeader
	ServerName     string `json:"server_name"`
	ToolName       string `json:"tool_name"`
	InputSizeBytes int    `json:"input_size_bytes"`
}

func (MCPToolCallStarted) eventSealed() {}

type MCPToolCallCompleted struct {
	EventHeader
	ServerName      string `json:"server_name"`
	ToolName        string `json:"tool_name"`
	DurationMs      int64  `json:"duration_ms"`
	OutputSizeBytes int    `json:"output_size_bytes"`
}

func (MCPToolCallCompleted) eventSealed() {}

type MCPHealthCheck struct {
	EventHeader
	ServerName string  `json:"server_name"`
	Status     string  `json:"status"`
	PID        int     `json:"pid"`
	MemoryMB   float64 `json:"memory_mb"`
}

func (MCPHealthCheck) eventSealed() {}

// --- Session Events ---

type SessionStarted struct {
	EventHeader
	SessionID   string `json:"session_id"`
	ResumedFrom string `json:"resumed_from,omitempty"`
}

func (SessionStarted) eventSealed() {}

type SessionSaved struct {
	EventHeader
	SessionID     string `json:"session_id"`
	MessageCount  int    `json:"message_count"`
	FileSizeBytes int64  `json:"file_size_bytes"`
}

func (SessionSaved) eventSealed() {}

type SessionEnded struct {
	EventHeader
	SessionID    string  `json:"session_id"`
	DurationMs   int64   `json:"duration_ms"`
	TurnCount    int     `json:"turn_count"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

func (SessionEnded) eventSealed() {}

// --- Agent Events ---

type SubAgentSpawned struct {
	EventHeader
	SubAgentID    string `json:"sub_agent_id"`
	AgentName     string `json:"agent_name"`
	Model         string `json:"model"`
	Provider      string `json:"provider"`
	ParentAgentID string `json:"parent_agent_id,omitempty"`
}

func (SubAgentSpawned) eventSealed() {}

type SubAgentCompleted struct {
	EventHeader
	SubAgentID string           `json:"sub_agent_id"`
	DurationMs int64            `json:"duration_ms"`
	TurnCount  int              `json:"turn_count"`
	Usage      model.TokenUsage `json:"usage"`
}

func (SubAgentCompleted) eventSealed() {}

type SubAgentFailed struct {
	EventHeader
	SubAgentID   string `json:"sub_agent_id"`
	ErrorType    string `json:"error_type"`
	ErrorMessage string `json:"error_message"`
}

func (SubAgentFailed) eventSealed() {}

// --- Error Events ---

type ErrorOccurred struct {
	EventHeader
	Severity     string `json:"severity"`
	Component    string `json:"component"`
	ErrorType    string `json:"error_type"`
	ErrorMessage string `json:"error_message"`
	Stack        string `json:"stack,omitempty"`
}

func (ErrorOccurred) eventSealed() {}

// --- Permission Audit Events ---

type PermissionRuleMatched struct {
	EventHeader
	ToolName string `json:"tool_name"`
	Pattern  string `json:"pattern"`
	Source   string `json:"source"`
	Decision string `json:"decision"`
}

func (PermissionRuleMatched) eventSealed() {}

type PermissionEscalated struct {
	EventHeader
	ToolName     string `json:"tool_name"`
	FromDecision string `json:"from_decision"`
	ToDecision   string `json:"to_decision"`
	Reason       string `json:"reason"`
}

func (PermissionEscalated) eventSealed() {}

type PermissionDenialEnforced struct {
	EventHeader
	ToolCallID  string `json:"tool_call_id"`
	ToolName    string `json:"tool_name"`
	WasExecuted bool   `json:"was_executed"`
}

func (PermissionDenialEnforced) eventSealed() {}

// --- Slash Command Events ---

type SlashCommandExecuted struct {
	EventHeader
	CommandName string `json:"command_name"`
	Args        string `json:"args,omitempty"`
	DurationMs  int64  `json:"duration_ms"`
	Success     bool   `json:"success"`
}

func (SlashCommandExecuted) eventSealed() {}

// --- System Prompt Events ---

type AgentMDLoaded struct {
	EventHeader
	Path  string `json:"path"`
	Scope string `json:"scope"`
	Bytes int    `json:"bytes"`
}

func (AgentMDLoaded) eventSealed() {}

type AgentMDNotFound struct {
	EventHeader
	Path  string `json:"path"`
	Scope string `json:"scope"`
}

func (AgentMDNotFound) eventSealed() {}

type SystemPromptBuilt struct {
	EventHeader
	BlockCount int `json:"block_count"`
	TotalBytes int `json:"total_bytes"`
}

func (SystemPromptBuilt) eventSealed() {}

// --- Flow Trace Events ---

// FlowTrace captures a decision point, branch, or loop iteration in the code.
// Use this for fine-grained tracing without creating a new event type per decision.
type FlowTrace struct {
	EventHeader
	Component string `json:"component"` // e.g. "query", "orchestrator", "permission", "tui"
	Function  string `json:"function"`  // e.g. "runLoop", "executeSingle", "Check"
	Message   string `json:"message"`   // e.g. "stop_reason=tool_use, executing 3 tools"
}

func (FlowTrace) eventSealed() {}

// Trace is a convenience method on EventBus for emitting FlowTrace events.
func (bus *EventBus) Trace(component, function, msg string) {
	if bus == nil {
		return
	}
	bus.Emit(FlowTrace{
		EventHeader: NewEventHeader("FlowTrace", "", "", ""),
		Component:   component,
		Function:    function,
		Message:     msg,
	})
}
