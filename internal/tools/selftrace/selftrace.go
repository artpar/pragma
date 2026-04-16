package selftrace

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
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
			"description": "Filter tool events by tool name (e.g. Read, Bash, LifecycleRun)"
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
type Tool struct {
	LogFilePath string
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"SelfTrace\"")
	return "SelfTrace"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: toolDescription")
	return toolDescription
}
func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "selftrace", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "selftrace", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "selftrace", "Tool.CheckPerm", "return: checker.Check(ctx, \"SelfTrace\", \"\")")
	return checker.Check(ctx, "SelfTrace", "")
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "selftrace", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "selftrace", "Tool.Invoke", "exit")

	var in selfTraceInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "selftrace", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "selftrace", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}

	q, err := parseQuery(in)
	if err != nil {
		observe.TraceCtx(ctx, "selftrace", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "selftrace", "Tool.Invoke", "return: tool.InvokeResult{}, err")
		return tool.InvokeResult{}, err
	}

	if t.LogFilePath == "" {
		observe.TraceCtx(ctx, "selftrace", "Tool.Invoke", "if: t.LogFilePath == \"\"")
		observe.TraceCtx(ctx, "selftrace", "Tool.Invoke", "return: tool.InvokeResult{Content: \"No log file available for this session.\"}, nil")
		return tool.InvokeResult{Content: "No log file available for this session."}, nil
	}

	f, err := os.Open(t.LogFilePath)
	if err != nil {
		observe.TraceCtx(ctx, "selftrace", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "selftrace", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Cannot open log file: %v\", err)}, nil")
		return tool.InvokeResult{Content: fmt.Sprintf("Cannot open log file: %v", err)}, nil
	}
	defer f.Close()

	if q.Summary {
		observe.TraceCtx(ctx, "selftrace", "Tool.Invoke", "if: q.Summary")
		observe.TraceCtx(ctx, "selftrace", "Tool.Invoke", "return: t.invokeSummary(ctx, f, q)")
		return t.invokeSummary(ctx, f, q)
	}
	observe.TraceCtx(ctx, "selftrace", "Tool.Invoke", "return: t.invokeQuery(ctx, f, q)")
	return t.invokeQuery(ctx, f, q)
}

func (t *Tool) invokeQuery(ctx context.Context, f *os.File, q *Query) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "selftrace", "Tool.invokeQuery", "enter")
	defer observe.TraceCtx(ctx, "selftrace", "Tool.invokeQuery", "exit")
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var matched int
	var page []observe.Event
	startIdx := (q.Page - 1) * q.PageSize
	endIdx := startIdx + q.PageSize

	for scanner.Scan() {
		observe.TraceCtx(ctx, "selftrace", "Tool.invokeQuery", "for: scanner.Scan()")
		if ctx.Err() != nil {
			observe.TraceCtx(ctx, "selftrace", "Tool.invokeQuery", "if: ctx.Err() != nil")
			break
		}
		line := scanner.Bytes()

		if q.Topic != "flow" && q.Kind != "FlowTrace" && isFlowTraceLine(line) {
			observe.TraceCtx(ctx, "selftrace", "Tool.invokeQuery", "if: q.Topic != \"flow\" && q.Kind != \"FlowTrace\" && isFlowTraceLine(line)")
			continue
		}
		ev, err := observe.UnmarshalEvent(line)
		if err != nil {
			observe.TraceCtx(ctx, "selftrace", "Tool.invokeQuery", "if: err != nil")
			continue
		}
		if !q.Matches(ev, line) {
			observe.TraceCtx(ctx, "selftrace", "Tool.invokeQuery", "if: !q.Matches(ev, line)")
			continue
		}
		if matched >= startIdx && matched < endIdx {
			observe.TraceCtx(ctx, "selftrace", "Tool.invokeQuery", "if: matched >= startIdx && matched < endIdx")
			page = append(page, ev)
		}
		matched++
	}

	if matched == 0 {
		observe.TraceCtx(ctx, "selftrace", "Tool.invokeQuery", "if: matched == 0")
		observe.TraceCtx(ctx, "selftrace", "Tool.invokeQuery", "return: tool.InvokeResult{Content: \"No events match the query.\"}, nil")
		return tool.InvokeResult{Content: "No events match the query."}, nil
	}

	var b strings.Builder
	for _, ev := range page {
		observe.TraceCtx(ctx, "selftrace", "Tool.invokeQuery", "range page")
		b.WriteString(formatEvent(ev))
		b.WriteByte('\n')
	}

	totalPages := (matched + q.PageSize - 1) / q.PageSize
	fmt.Fprintf(&b, "--- page %d/%d, showing %d of %d matching events ---",
		q.Page, totalPages, len(page), matched)
	observe.TraceCtx(ctx, "selftrace", "Tool.invokeQuery", "return: tool.InvokeResult{Content: b.String()}, nil")

	return tool.InvokeResult{Content: b.String()}, nil
}

func (t *Tool) invokeSummary(ctx context.Context, f *os.File, q *Query) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "selftrace", "Tool.invokeSummary", "enter")
	defer observe.TraceCtx(ctx, "selftrace", "Tool.invokeSummary", "exit")
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	s := &summaryAgg{}
	for scanner.Scan() {
		observe.TraceCtx(ctx, "selftrace", "Tool.invokeSummary", "for: scanner.Scan()")
		if ctx.Err() != nil {
			observe.TraceCtx(ctx, "selftrace", "Tool.invokeSummary", "if: ctx.Err() != nil")
			break
		}
		line := scanner.Bytes()

		if isFlowTraceLine(line) {
			observe.TraceCtx(ctx, "selftrace", "Tool.invokeSummary", "if: isFlowTraceLine(line)")
			continue
		}
		ev, err := observe.UnmarshalEvent(line)
		if err != nil {
			observe.TraceCtx(ctx, "selftrace", "Tool.invokeSummary", "if: err != nil")
			continue
		}
		s.add(ev)
	}
	observe.TraceCtx(ctx, "selftrace", "Tool.invokeSummary", "return: tool.InvokeResult{Content: s.String()}, nil")

	return tool.InvokeResult{Content: s.String()}, nil
}

// parseQuery converts raw input into a validated Query.
func parseQuery(in selfTraceInput) (*Query, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
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
		observe.GlobalTrace("if: q.Topic == \"\"")
		q.Topic = "all"
	}
	if q.Page < 1 {
		observe.GlobalTrace("if: q.Page < 1")
		q.Page = 1
	}
	if q.PageSize < 1 {
		observe.GlobalTrace("if: q.PageSize < 1")
		q.PageSize = 50
	}
	if q.PageSize > 200 {
		observe.GlobalTrace("if: q.PageSize > 200")
		q.PageSize = 200
	}

	now := time.Now()
	if in.Since != "" {
		observe.GlobalTrace("if: in.Since != \"\"")
		t, err := parseTimeRef(in.Since, now)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"invalid 'since': %w\", err)")
			return nil, fmt.Errorf("invalid 'since': %w", err)
		}
		q.Since = t
	}
	if in.Until != "" {
		observe.GlobalTrace("if: in.Until != \"\"")
		t, err := parseTimeRef(in.Until, now)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"invalid 'until': %w\", err)")
			return nil, fmt.Errorf("invalid 'until': %w", err)
		}
		q.Until = t
	}
	if in.Contains != "" {
		observe.GlobalTrace("if: in.Contains != \"\"")
		q.ContainsBytes = []byte(in.Contains)
	}

	q.topicKinds = topicToKinds(q.Topic)
	observe.GlobalTrace("return: q, nil")
	return q, nil
}

// parseTimeRef parses "HH:MM:SS" or relative "-Nm" / "-Ns".
func parseTimeRef(s string, now time.Time) (time.Time, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if strings.HasPrefix(s, "-") {
		observe.GlobalTrace("if: strings.HasPrefix(s, \"-\")")
		s = s[1:]
		var dur time.Duration
		var err error

		dur, err = time.ParseDuration(s)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: time.Time{}, fmt.Errorf(\"parse duration %q: %w\", s, err)")
			return time.Time{}, fmt.Errorf("parse duration %q: %w", s, err)
		}
		observe.GlobalTrace("return: now.Add(-dur), nil")
		return now.Add(-dur), nil
	}

	t, err := time.Parse("15:04:05", s)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: time.Time{}, fmt.Errorf(\"parse time %q: expected HH:MM:SS or -Nm\", s)")
		return time.Time{}, fmt.Errorf("parse time %q: expected HH:MM:SS or -Nm", s)
	}
	y, m, d := now.Date()
	observe.GlobalTrace("return: time.Date(y, m, d, t.Hour(), t.Minute(), t.Second(), 0, now.Location()), nil")
	return time.Date(y, m, d, t.Hour(), t.Minute(), t.Second(), 0, now.Location()), nil
}

func topicToKinds(topic string) map[string]bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var kinds []string
	switch topic {
	case "api":
		observe.GlobalTrace("case: \"api\"")
		kinds = []string{"APIRequestStarted", "APIRequestCompleted", "APIRequestFailed", "APIRetryScheduled"}
	case "tools":
		observe.GlobalTrace("case: \"tools\"")
		kinds = []string{"ToolCallReceived", "ToolExecutionStarted", "ToolExecutionCompleted", "ToolExecutionFailed", "ToolBatchStarted", "ToolBatchCompleted"}
	case "errors":
		observe.GlobalTrace("case: \"errors\"")
		kinds = []string{"ErrorOccurred", "APIRequestFailed", "ToolExecutionFailed", "CompactionFailed", "SubAgentFailed", "MCPServerFailed"}
	case "permissions":
		observe.GlobalTrace("case: \"permissions\"")
		kinds = []string{"ToolPermissionChecked", "ToolPermissionPrompted", "PermissionRuleMatched", "PermissionEscalated", "PermissionDenialEnforced", "PermissionPersisted"}
	case "session":
		observe.GlobalTrace("case: \"session\"")
		kinds = []string{"ConversationStarted", "MessageAppended", "ConversationForked", "SessionStarted", "SessionSaved", "SessionEnded"}
	case "agents":
		observe.GlobalTrace("case: \"agents\"")
		kinds = []string{"SubAgentSpawned", "SubAgentCompleted", "SubAgentFailed", "ConversationForked"}
	case "mcp":
		observe.GlobalTrace("case: \"mcp\"")
		kinds = []string{"MCPServerConnecting", "MCPServerConnected", "MCPServerDisconnected", "MCPServerFailed", "MCPToolCallStarted", "MCPToolCallCompleted", "MCPHealthCheck", "McpOAuthStarted", "McpOAuthCompleted"}
	case "lifecycle":
		observe.GlobalTrace("case: \"lifecycle\"")
		kinds = []string{"LifecycleStepStarted", "LifecycleNodeCompleted", "LifecycleTransition", "LifecycleCompleted"}
	case "compaction":
		observe.GlobalTrace("case: \"compaction\"")
		kinds = []string{"CompactionStarted", "CompactionCompleted", "CompactionFailed"}
	case "flow":
		observe.GlobalTrace("case: \"flow\"")
		kinds = []string{"FlowTrace"}
	default:
		observe.GlobalTrace("default")
		return nil
	}
	m := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		observe.GlobalTrace("range kinds")
		m[k] = true
	}
	observe.GlobalTrace("return: m")
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	kind := ev.EventKind()

	if q.Topic == "all" && kind == "FlowTrace" {
		observe.GlobalTrace("if: q.Topic == \"all\" && kind == \"FlowTrace\"")
		observe.GlobalTrace("return: false")
		return false
	}

	if q.topicKinds != nil && !q.topicKinds[kind] {
		observe.GlobalTrace("if: q.topicKinds != nil && !q.topicKinds[kind]")
		observe.GlobalTrace("return: false")
		return false
	}

	if q.Kind != "" && kind != q.Kind {
		observe.GlobalTrace("if: q.Kind != \"\" && kind != q.Kind")
		observe.GlobalTrace("return: false")
		return false
	}

	if q.TraceID != "" && ev.EventTraceID() != q.TraceID {
		observe.GlobalTrace("if: q.TraceID != \"\" && ev.EventTraceID() != q.TraceID")
		observe.GlobalTrace("return: false")
		return false
	}

	if q.ToolName != "" && !matchesToolName(ev, q.ToolName) {
		observe.GlobalTrace("if: q.ToolName != \"\" && !matchesToolName(ev, q.ToolName)")
		observe.GlobalTrace("return: false")
		return false
	}

	ts := ev.EventTimestamp()
	if !q.Since.IsZero() && ts.Before(q.Since) {
		observe.GlobalTrace("if: !q.Since.IsZero() && ts.Before(q.Since)")
		observe.GlobalTrace("return: false")
		return false
	}
	if !q.Until.IsZero() && ts.After(q.Until) {
		observe.GlobalTrace("if: !q.Until.IsZero() && ts.After(q.Until)")
		observe.GlobalTrace("return: false")
		return false
	}

	if len(q.ContainsBytes) > 0 && !bytes.Contains(rawLine, q.ContainsBytes) {
		observe.GlobalTrace("if: len(q.ContainsBytes) > 0 && !bytes.Contains(rawLine, q.ContainsBytes)")
		observe.GlobalTrace("return: false")
		return false
	}

	if q.ErrorsOnly && !isErrorEvent(kind) {
		observe.GlobalTrace("if: q.ErrorsOnly && !isErrorEvent(kind)")
		observe.GlobalTrace("return: false")
		return false
	}
	observe.GlobalTrace("return: true")

	return true
}

func matchesToolName(ev observe.Event, name string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch e := ev.(type) {
	case observe.ToolCallReceived:
		observe.GlobalTrace("typecase: observe.ToolCallReceived")
		return e.ToolName == name
	case observe.ToolExecutionStarted:
		observe.GlobalTrace("typecase: observe.ToolExecutionStarted")
		return e.ToolName == name
	case observe.ToolExecutionCompleted:
		observe.GlobalTrace("typecase: observe.ToolExecutionCompleted")
		return e.ToolName == name
	case observe.ToolExecutionFailed:
		observe.GlobalTrace("typecase: observe.ToolExecutionFailed")
		return e.ToolName == name
	case observe.ToolPermissionChecked:
		observe.GlobalTrace("typecase: observe.ToolPermissionChecked")
		return e.ToolName == name
	case observe.ToolPermissionPrompted:
		observe.GlobalTrace("typecase: observe.ToolPermissionPrompted")
		return e.ToolName == name
	case observe.MCPToolCallStarted:
		observe.GlobalTrace("typecase: observe.MCPToolCallStarted")
		return e.ToolName == name
	case observe.MCPToolCallCompleted:
		observe.GlobalTrace("typecase: observe.MCPToolCallCompleted")
		return e.ToolName == name
	}
	observe.GlobalTrace("return: false")
	return false
}

func isErrorEvent(kind string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch kind {
	case "ErrorOccurred", "APIRequestFailed", "ToolExecutionFailed",
		"CompactionFailed", "SubAgentFailed", "MCPServerFailed":
		observe.GlobalTrace("case: \"ErrorOccurred\", \"APIRequestFailed\", \"ToolExecutionFailed\", \"CompactionFailed...")
		return true
	}
	observe.GlobalTrace("return: false")
	return false
}

// flowTracePrefix is used for fast byte-level skip of FlowTrace lines.
// FlowTrace events are 99%+ of log volume (131K+ per session) and must
// be skipped without JSON parsing for acceptable performance on large logs.
var flowTracePrefix = []byte(`"kind":"FlowTrace"`)

// isFlowTraceLine checks if a raw JSONL line is a FlowTrace event
// using byte-level comparison — no JSON parsing needed.
func isFlowTraceLine(line []byte) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: bytes.Contains(line, flowTracePrefix)")
	return bytes.Contains(line, flowTracePrefix)
}
