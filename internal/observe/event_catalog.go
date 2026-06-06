package observe

import (
	"encoding/json"
	"time"

	"github.com/artpar/pragma/internal/model"
)

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
	Model          string             `json:"model"`
	MaxTokens      int                `json:"max_tokens,omitempty"`
	MessageCount   int                `json:"message_count"`
	ToolCount      int                `json:"tool_count"`
	TokenEstimate  int                `json:"token_estimate"`
	Messages       []model.Message    `json:"messages,omitempty"`
	System         string             `json:"system,omitempty"`
	SystemPrompt   model.SystemPrompt `json:"system_prompt,omitempty"`
	Tools          []model.ToolDef    `json:"tools,omitempty"`
	Temperature    *float64           `json:"temperature,omitempty"`
	Thinking       *APIThinkingConfig `json:"thinking,omitempty"`
	ResponseSchema json.RawMessage    `json:"response_schema,omitempty"`
}

func (APIRequestStarted) eventSealed() {}

// APIThinkingConfig is the recorded request form of provider thinking config.
// It lives in observe to avoid importing provider from the observability layer.
type APIThinkingConfig struct {
	Enabled      bool `json:"enabled"`
	BudgetTokens int  `json:"budget_tokens,omitempty"`
}

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
	Content    json.RawMessage  `json:"content,omitempty"`
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
	ToolCallID     string          `json:"tool_call_id"`
	ToolName       string          `json:"tool_name"`
	InputSizeBytes int             `json:"input_size_bytes"`
	Input          json.RawMessage `json:"input,omitempty"`
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

type ToolPermissionPromptStarted struct {
	EventHeader
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
}

func (ToolPermissionPromptStarted) eventSealed() {}

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
	ToolCallID     string          `json:"tool_call_id"`
	ToolName       string          `json:"tool_name"`
	Concurrent     bool            `json:"concurrent"`
	Input          json.RawMessage `json:"input,omitempty"`
	InputSizeBytes int             `json:"input_size_bytes"`
}

func (ToolExecutionStarted) eventSealed() {}

type ToolExecutionCompleted struct {
	EventHeader
	ToolCallID      string `json:"tool_call_id"`
	ToolName        string `json:"tool_name"`
	DurationMs      int64  `json:"duration_ms"`
	OutputSizeBytes int    `json:"output_size_bytes"`
	IsError         bool   `json:"is_error"`
	Output          string `json:"output,omitempty"`
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

type PermissionDecisionFinal struct {
	EventHeader
	ToolCallID   string `json:"tool_call_id"`
	ToolName     string `json:"tool_name"`
	Decision     string `json:"decision"`
	UserDecision string `json:"user_decision,omitempty"`
	Rule         string `json:"rule,omitempty"`
	Source       string `json:"source,omitempty"`
	WasExecuted  bool   `json:"was_executed"`
}

func (PermissionDecisionFinal) eventSealed() {}

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

// --- Hook Events ---

// HookExecuted records a hook command execution with its outcome.
type HookExecuted struct {
	EventHeader
	HookEvent string `json:"hook_event"` // PreToolUse, PostToolUse, Stop, etc.
	Command   string `json:"command"`
	ExitCode  int    `json:"exit_code"`
	Outcome   string `json:"outcome"` // ok, block, error, timeout
	HasJSON   bool   `json:"has_json"`
}

func (HookExecuted) eventSealed() {}

// HookBlocked records when a hook blocks an operation (exit code 2).
type HookBlocked struct {
	EventHeader
	HookEvent string `json:"hook_event"`
	Command   string `json:"command"`
	Message   string `json:"message"` // stderr from the hook
}

func (HookBlocked) eventSealed() {}

// PermissionPersisted records when a permission rule is written to settings.local.json.
type PermissionPersisted struct {
	EventHeader
	ToolName string `json:"tool_name"`
	Content  string `json:"content,omitempty"`
	Decision string `json:"decision"`
}

func (PermissionPersisted) eventSealed() {}

// --- Brief/SendUserMessage Events ---

// BriefAttachment describes a file attached to a BriefMessageSent event.
type BriefAttachment struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsImage bool   `json:"is_image"`
}

// BriefMessageSent records when the LLM sends a message to the user via SendUserMessage.
type BriefMessageSent struct {
	EventHeader
	Message     string            `json:"message"`
	Attachments []BriefAttachment `json:"attachments,omitempty"`
	Status      string            `json:"status"` // "normal" or "proactive"
}

func (BriefMessageSent) eventSealed() {}

// --- MCP OAuth Events ---

// McpOAuthStarted records the start of an OAuth flow for an MCP server.
type McpOAuthStarted struct {
	EventHeader
	ServerName string `json:"server_name"`
	AuthURL    string `json:"auth_url"`
}

func (McpOAuthStarted) eventSealed() {}

// McpOAuthCompleted records the completion of an OAuth flow for an MCP server.
type McpOAuthCompleted struct {
	EventHeader
	ServerName string `json:"server_name"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
}

func (McpOAuthCompleted) eventSealed() {}

// --- Team Events ---

// TeamCreated records when a new multi-agent swarm team is created.
type TeamCreated struct {
	EventHeader
	TeamName    string `json:"team_name"`
	LeadAgentID string `json:"lead_agent_id"`
	MemberCount int    `json:"member_count"`
}

func (TeamCreated) eventSealed() {}

// TeamDeleted records when a team is cleaned up and disbanded.
type TeamDeleted struct {
	EventHeader
	TeamName string `json:"team_name"`
}

func (TeamDeleted) eventSealed() {}

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

// --- Lifecycle events (internal/lifecycle/) ---

// LifecycleStepStarted is emitted before each superstep in a lifecycle graph.
type LifecycleStepStarted struct {
	EventHeader
	Step  int      `json:"step"`
	Nodes []string `json:"nodes"`
}

func (LifecycleStepStarted) eventSealed() {}

// LifecycleNodeCompleted is emitted when a lifecycle node finishes.
type LifecycleNodeCompleted struct {
	EventHeader
	Step     int           `json:"step"`
	Node     string        `json:"node"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
}

func (LifecycleNodeCompleted) eventSealed() {}

// LifecycleTransition is emitted on each lifecycle edge traversal.
type LifecycleTransition struct {
	EventHeader
	Step     int    `json:"step"`
	From     string `json:"from"`
	To       string `json:"to"`
	RouteKey string `json:"route_key,omitempty"`
}

func (LifecycleTransition) eventSealed() {}

// LifecycleCompleted is emitted when a lifecycle execution finishes.
type LifecycleCompleted struct {
	EventHeader
	TotalSteps int    `json:"total_steps"`
	Error      string `json:"error,omitempty"`
}

func (LifecycleCompleted) eventSealed() {}

// --- Orchestration events (internal/orchestration/) ---

type OrchestrationStarted struct {
	EventHeader
	Name    string `json:"name"`
	Initial string `json:"initial"`
}

func (OrchestrationStarted) eventSealed() {}

type OrchestrationStateStarted struct {
	EventHeader
	StateID   string `json:"state_id"`
	PersonaID string `json:"persona_id,omitempty"`
	Control   string `json:"control,omitempty"`
}

func (OrchestrationStateStarted) eventSealed() {}

type OrchestrationStateCompleted struct {
	EventHeader
	StateID    string        `json:"state_id"`
	Duration   time.Duration `json:"duration"`
	DurationMs int64         `json:"duration_ms"`
}

func (OrchestrationStateCompleted) eventSealed() {}

type OrchestrationControl struct {
	EventHeader
	StateID string `json:"state_id"`
	Control string `json:"control"`
	Event   string `json:"event,omitempty"`
}

func (OrchestrationControl) eventSealed() {}

type OrchestrationTransition struct {
	EventHeader
	From  string `json:"from"`
	Event string `json:"event"`
	To    string `json:"to"`
}

func (OrchestrationTransition) eventSealed() {}

type OrchestrationHandoff struct {
	EventHeader
	StateID   string `json:"state_id"`
	Event     string `json:"event"`
	Path      string `json:"path"`
	Direction string `json:"direction"`
}

func (OrchestrationHandoff) eventSealed() {}

type OrchestrationCompleted struct {
	EventHeader
	Name string `json:"name"`
}

func (OrchestrationCompleted) eventSealed() {}
