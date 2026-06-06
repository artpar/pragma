package selftrace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
)

type selfTraceInput struct {
	Topic      string `json:"topic"       desc:"Event category to query: all, api, tools, errors, permissions, session, agents, mcp, lifecycle, compaction, flow"`
	Kind       string `json:"kind"        desc:"Exact event kind to filter by (e.g. ToolInvoked, APIRequestSent)"`
	TraceID    string `json:"trace_id"    desc:"Filter events belonging to this trace ID"`
	ToolName   string `json:"tool_name"   desc:"Filter events for this tool name"`
	Since      string `json:"since"       desc:"ISO 8601 timestamp; return events at or after this time"`
	Until      string `json:"until"       desc:"ISO 8601 timestamp; return events at or before this time"`
	Contains   string `json:"contains"    desc:"Return only events whose JSON contains this substring"`
	ErrorsOnly bool   `json:"errors_only" desc:"When true, return only events with an error field set"`
	Page       int    `json:"page"        desc:"Page number for paginated results (1-based)"`
	PageSize   int    `json:"page_size"   desc:"Number of events per page (default 50)"`
	Summary    bool   `json:"summary"     desc:"When true, return a statistical summary instead of raw events"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
	"properties": {
		"topic": {
			"type": "string",
			"enum": ["all", "api", "tools", "errors", "permissions", "session", "agents", "mcp", "lifecycle", "compaction", "flow"],
			"description": "Event category to query. Default: all (excludes FlowTrace noise unless topic=flow)"
		},
		"kind": {
			"type": "string",
			"description": "Exact event kind filter (e.g. APIRequestCompleted, ToolExecutionCompleted)"
		},
		"trace_id": {
			"type": "string",
			"description": "Filter by distributed trace ID to follow a full request chain"
		},
		"tool_name": {
			"type": "string",
			"description": "Filter tool events by exact tool name"
		},
		"since": {
			"type": "string",
			"description": "Start time filter: HH:MM:SS or relative like -5m"
		},
		"until": {
			"type": "string",
			"description": "End time filter: HH:MM:SS or relative like -1m"
		},
		"contains": {
			"type": "string",
			"description": "Text search across all string fields in the raw event JSON"
		},
		"errors_only": {
			"type": "boolean",
			"description": "Only show events with errors or failures"
		},
		"page": {
			"type": "integer",
			"description": "Page number (1-based, default 1)"
		},
		"page_size": {
			"type": "integer",
			"description": "Events per page (default 50, max 200)"
		},
		"summary": {
			"type": "boolean",
			"description": "Return aggregated summary: token totals, tool stats, error counts, timing"
		}
	}
}`)

const toolDescription = `Introspect your own execution trace for this session.

Query the JSONL event log to understand what you've done, how long things took, what failed, and how tokens were spent. Use this for self-reflection, debugging, and decision-making.

Topics: api (LLM calls), tools (tool executions), errors (failures), permissions (access decisions), session (messages), agents (sub-agents), mcp (MCP servers), lifecycle (workflow graphs), compaction (context compression), flow (branch-level traces).

Examples:
- Summary of the session so far: {"summary": true}
- What tools have I called: {"topic": "tools"}
- Show me all errors: {"errors_only": true}
- Follow a request chain: {"trace_id": "abc123"}
- What happened in the last 2 minutes: {"since": "-2m"}
- Search for a specific term: {"contains": "SPEC.md"}`

// Tool implements the SelfTrace tool for reflective log introspection.
//
// IMPORTANT: This file is excluded from AST auto-instrumentation (ADR-028).
// SelfTrace reads the same JSONL log file that observe.GlobalTrace writes to.
// Adding instrumentation here creates an infinite feedback loop: each line read
// triggers GlobalTrace calls that append new lines, which the scanner then reads,
// causing exponential log growth and tool hangs.
type Tool struct {
	LogFilePath string
}

func (t *Tool) Name() string        { return "SelfTrace" }
func (t *Tool) Description() string { return toolDescription }
func (t *Tool) InputSchema() json.RawMessage {
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, "SelfTrace", "")
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in selfTraceInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}

	q, err := parseQuery(in)
	if err != nil {
		return tool.InvokeResult{}, err
	}

	if t.LogFilePath == "" {
		return tool.InvokeResult{Content: "No log file available for this session."}, nil
	}

	if q.Summary {
		return t.invokeSummary(ctx, t.LogFilePath, q)
	}
	return t.invokeQuery(ctx, t.LogFilePath, q)
}

func (t *Tool) invokeQuery(ctx context.Context, logPath string, q *Query) (tool.InvokeResult, error) {
	var matched int
	var page []observe.Event
	startIdx := (q.Page - 1) * q.PageSize
	endIdx := startIdx + q.PageSize

	if err := observe.ScanEventLog(logPath, func(line observe.EventLogLine) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if q.Topic != "flow" && q.Kind != "FlowTrace" && isFlowTraceLine(line.Raw) {
			return nil
		}
		ev, err := observe.UnmarshalEvent(line.Raw)
		if err != nil {
			return fmt.Errorf("line %d: unmarshal event: %w", line.Number, err)
		}
		if !q.Matches(ev, line.Raw) {
			return nil
		}
		if matched >= startIdx && matched < endIdx {
			page = append(page, ev)
		}
		matched++
		return nil
	}); err != nil {
		return tool.InvokeResult{}, err
	}

	if matched == 0 {
		return tool.InvokeResult{Content: "No events match the query."}, nil
	}

	var b strings.Builder
	for _, ev := range page {
		b.WriteString(formatEvent(ev))
		b.WriteByte('\n')
	}

	totalPages := (matched + q.PageSize - 1) / q.PageSize
	fmt.Fprintf(&b, "--- page %d/%d, showing %d of %d matching events ---",
		q.Page, totalPages, len(page), matched)

	return tool.InvokeResult{Content: b.String()}, nil
}

func (t *Tool) invokeSummary(ctx context.Context, logPath string, _ *Query) (tool.InvokeResult, error) {
	s := &summaryAgg{}
	if err := observe.ScanEventLog(logPath, func(line observe.EventLogLine) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if isFlowTraceLine(line.Raw) {
			return nil
		}
		ev, err := observe.UnmarshalEvent(line.Raw)
		if err != nil {
			return fmt.Errorf("line %d: unmarshal event: %w", line.Number, err)
		}
		s.add(ev)
		return nil
	}); err != nil {
		return tool.InvokeResult{}, err
	}

	return tool.InvokeResult{Content: s.String()}, nil
}

// parseQuery converts raw input into a validated Query.
func parseQuery(in selfTraceInput) (*Query, error) {
	q := &Query{
		Topic:      in.Topic,
		Kind:       in.Kind,
		TraceID:    in.TraceID,
		ToolName:   in.ToolName,
		ErrorsOnly: in.ErrorsOnly,
		Summary:    in.Summary,
		Page:       in.Page,
		PageSize:   in.PageSize,
	}
	if q.Topic == "" {
		q.Topic = "all"
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 50
	}
	if q.PageSize > 200 {
		q.PageSize = 200
	}

	now := time.Now()
	if in.Since != "" {
		t, err := parseTimeRef(in.Since, now)
		if err != nil {
			return nil, fmt.Errorf("invalid 'since': %w", err)
		}
		q.Since = t
	}
	if in.Until != "" {
		t, err := parseTimeRef(in.Until, now)
		if err != nil {
			return nil, fmt.Errorf("invalid 'until': %w", err)
		}
		q.Until = t
	}
	if in.Contains != "" {
		q.ContainsBytes = []byte(in.Contains)
	}

	q.topicKinds = topicToKinds(q.Topic)
	return q, nil
}

// parseTimeRef parses "HH:MM:SS" or relative "-Nm" / "-Ns".
func parseTimeRef(s string, now time.Time) (time.Time, error) {
	if strings.HasPrefix(s, "-") {
		s = s[1:]
		dur, err := time.ParseDuration(s)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse duration %q: %w", s, err)
		}
		return now.Add(-dur), nil
	}

	t, err := time.Parse("15:04:05", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: expected HH:MM:SS or -Nm", s)
	}
	y, m, d := now.Date()
	return time.Date(y, m, d, t.Hour(), t.Minute(), t.Second(), 0, now.Location()), nil
}

func topicToKinds(topic string) map[string]bool {
	var kinds []string
	switch topic {
	case "api":
		kinds = []string{"APIRequestStarted", "APIRequestCompleted", "APIRequestFailed", "APIRetryScheduled"}
	case "tools":
		kinds = []string{"ToolCallReceived", "ToolExecutionStarted", "ToolExecutionCompleted", "ToolExecutionFailed", "ToolBatchStarted", "ToolBatchCompleted"}
	case "errors":
		kinds = []string{"ErrorOccurred", "APIRequestFailed", "ToolExecutionFailed", "CompactionFailed", "SubAgentFailed", "MCPServerFailed"}
	case "permissions":
		kinds = []string{"ToolPermissionChecked", "ToolPermissionPrompted", "PermissionRuleMatched", "PermissionEscalated", "PermissionDenialEnforced", "PermissionPersisted"}
	case "session":
		kinds = []string{"ConversationStarted", "MessageAppended", "ConversationForked", "SessionStarted", "SessionSaved", "SessionEnded"}
	case "agents":
		kinds = []string{"SubAgentSpawned", "SubAgentCompleted", "SubAgentFailed", "ConversationForked"}
	case "mcp":
		kinds = []string{"MCPServerConnecting", "MCPServerConnected", "MCPServerDisconnected", "MCPServerFailed", "MCPToolCallStarted", "MCPToolCallCompleted", "MCPHealthCheck", "McpOAuthStarted", "McpOAuthCompleted"}
	case "lifecycle":
		kinds = []string{"LifecycleStepStarted", "LifecycleNodeCompleted", "LifecycleTransition", "LifecycleCompleted"}
	case "compaction":
		kinds = []string{"CompactionStarted", "CompactionCompleted", "CompactionFailed"}
	case "flow":
		kinds = []string{"FlowTrace"}
	default:
		return nil
	}
	m := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		m[k] = true
	}
	return m
}

// Query holds parsed and validated filter parameters.
type Query struct {
	Topic         string
	Kind          string
	TraceID       string
	ToolName      string
	Since         time.Time
	Until         time.Time
	ContainsBytes []byte
	ErrorsOnly    bool
	Page          int
	PageSize      int
	Summary       bool

	topicKinds map[string]bool
}

// Matches returns true if the event passes all filters.
func (q *Query) Matches(ev observe.Event, rawLine []byte) bool {
	kind := ev.EventKind()

	if q.Topic == "all" && kind == "FlowTrace" {
		return false
	}
	if q.topicKinds != nil && !q.topicKinds[kind] {
		return false
	}
	if q.Kind != "" && kind != q.Kind {
		return false
	}
	if q.TraceID != "" && ev.EventTraceID() != q.TraceID {
		return false
	}
	if q.ToolName != "" && !matchesToolName(ev, q.ToolName) {
		return false
	}

	ts := ev.EventTimestamp()
	if !q.Since.IsZero() && ts.Before(q.Since) {
		return false
	}
	if !q.Until.IsZero() && ts.After(q.Until) {
		return false
	}
	if len(q.ContainsBytes) > 0 && !bytes.Contains(rawLine, q.ContainsBytes) {
		return false
	}
	if q.ErrorsOnly && !isErrorEvent(kind) {
		return false
	}

	return true
}

func matchesToolName(ev observe.Event, name string) bool {
	switch e := ev.(type) {
	case observe.ToolCallReceived:
		return e.ToolName == name
	case observe.ToolExecutionStarted:
		return e.ToolName == name
	case observe.ToolExecutionCompleted:
		return e.ToolName == name
	case observe.ToolExecutionFailed:
		return e.ToolName == name
	case observe.ToolPermissionChecked:
		return e.ToolName == name
	case observe.ToolPermissionPrompted:
		return e.ToolName == name
	case observe.MCPToolCallStarted:
		return e.ToolName == name
	case observe.MCPToolCallCompleted:
		return e.ToolName == name
	}
	return false
}

func isErrorEvent(kind string) bool {
	switch kind {
	case "ErrorOccurred", "APIRequestFailed", "ToolExecutionFailed",
		"CompactionFailed", "SubAgentFailed", "MCPServerFailed":
		return true
	}
	return false
}

// flowTracePrefix is used for fast byte-level skip of FlowTrace lines.
// FlowTrace events are 99%+ of log volume (131K+ per session) and must
// be skipped without JSON parsing for acceptable performance on large logs.
var flowTracePrefix = []byte(`"kind":"FlowTrace"`)

// isFlowTraceLine checks if a raw JSONL line is a FlowTrace event
// using byte-level comparison — no JSON parsing needed.
func isFlowTraceLine(line []byte) bool {
	return bytes.Contains(line, flowTracePrefix)
}
