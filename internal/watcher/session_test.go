package watcher

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/observe"
)

func mustEvent(t *testing.T, line string) observe.Event {
	t.Helper()
	ev, err := observe.UnmarshalEvent([]byte(line))
	if err != nil {
		t.Fatalf("unmarshal event: %v\nline: %s", err, line)
	}
	return ev
}

func TestSessionStateLifecycle(t *testing.T) {
	s := NewSessionState("/tmp/logs/2026-09-24T11-51-08.jsonl")
	if s.Name != "2026-09-24T11-51-08.jsonl" {
		t.Fatalf("name = %q", s.Name)
	}

	now := time.Date(2026, 9, 24, 11, 51, 8, 0, time.Local)
	events := []string{
		`{"kind":"UserTurnAccepted","time":"2026-09-24T11:51:10+05:30","prompt_chars":62}`,
		`{"kind":"MessageAppended","time":"2026-09-24T11:51:10.001+05:30","role":"user","content_types":["text"],"token_estimate":23}`,
		`{"kind":"MCPServerConnected","time":"2026-09-24T11:51:11+05:30","server_name":"agile","tool_count":17,"duration_ms":1063}`,
		`{"kind":"MCPHealthCheck","time":"2026-09-24T11:51:41+05:30","server_name":"agile","status":"connected","pid":123,"memory_mb":12.5}`,
		`{"kind":"APIRequestCompleted","time":"2026-09-24T11:51:20+05:30","stop_reason":"tool_use","usage":{"input_tokens":11646,"output_tokens":176},"duration_ms":37025,"model":"morph-glm53-744b","content":[` +
			`{"type":"thinking","data":{"text":"thinking"}},` +
			`{"type":"tool_call","data":{"id":"call-1","name":"Bash","input":{"cmd":"pwd && ls -la"}}},` +
			`{"type":"tool_call","data":{"id":"call-2","name":"mcp__past-conversations__search_conversations","input":{"query":"harness","limit":20}}}]}`,
		`{"kind":"MessageAppended","time":"2026-09-24T11:51:30+05:30","role":"user","content_types":["tool_result","tool_result"],"token_estimate":1331}`,
		`{"kind":"APIRequestCompleted","time":"2026-09-24T11:51:40+05:30","stop_reason":"end_turn","usage":{"input_tokens":20080,"cache_read_input_tokens":40000,"output_tokens":209},"duration_ms":7538,"model":"morph-glm53-744b","content":[` +
			`{"type":"text","data":{"text":"done"}}]}`,
	}
	for _, line := range events {
		s.Apply(mustEvent(t, line))
	}

	if s.Turns != 1 {
		t.Errorf("Turns = %d, want 1", s.Turns)
	}
	if s.Messages != 2 {
		t.Errorf("Messages = %d, want 2", s.Messages)
	}
	if s.APICalls != 2 {
		t.Errorf("APICalls = %d, want 2", s.APICalls)
	}
	if s.Model != "morph-glm53-744b" {
		t.Errorf("Model = %q", s.Model)
	}
	// Context fill = input + cache read + cache creation.
	if s.ContextFill != 20080+40000 {
		t.Errorf("ContextFill = %d, want %d", s.ContextFill, 20080+40000)
	}
	if s.OutputTokens != 176+209 {
		t.Errorf("OutputTokens = %d, want %d", s.OutputTokens, 176+209)
	}
	if s.ToolCalls != 2 {
		t.Errorf("ToolCalls = %d, want 2", s.ToolCalls)
	}
	if s.ToolCallCounts["Bash"] != 1 || s.ToolCallCounts["mcp__past-conversations__search_conversations"] != 1 {
		t.Errorf("ToolCallCounts = %v", s.ToolCallCounts)
	}
	if len(s.RecentTools) != 2 {
		t.Fatalf("RecentTools = %d entries, want 2", len(s.RecentTools))
	}
	if s.RecentTools[0].Summary != "pwd && ls -la" {
		t.Errorf("RecentTools[0].Summary = %q", s.RecentTools[0].Summary)
	}
	if s.Servers["agile"] != "connected" {
		t.Errorf("Servers[agile] = %q, want connected", s.Servers["agile"])
	}
	if s.InFlight {
		t.Error("InFlight after end_turn, want false")
	}

	// Health checks must not count as conversational progress.
	if s.LastCoreEvent != "APIRequestCompleted" {
		t.Errorf("LastCoreEvent = %q, want APIRequestCompleted", s.LastCoreEvent)
	}
	if s.LastCoreAt.Format("15:04:05") != "11:51:40" {
		t.Errorf("LastCoreAt = %v", s.LastCoreAt)
	}

	status, level := s.Status(now.Add(2 * time.Minute))
	if level != LevelInfo || status != "awaiting user (1m28s)" {
		t.Errorf("Status = (%q, %q), want awaiting user", status, level)
	}
}

func TestSessionStateInFlight(t *testing.T) {
	s := NewSessionState("/tmp/logs/x.jsonl")
	s.Apply(mustEvent(t, `{"kind":"MessageAppended","time":"2026-09-24T11:51:10+05:30","role":"user","content_types":["tool_result"],"token_estimate":100}`))
	if !s.InFlight {
		t.Fatal("InFlight after user append, want true")
	}
	status, _ := s.Status(time.Date(2026, 9, 24, 11, 52, 10, 0, time.Local))
	want := "model call in-flight 1m00s"
	if status != want {
		t.Errorf("Status = %q, want %q", status, want)
	}

	s.StallAfter = 30 * time.Second
	status, level := s.Status(time.Date(2026, 9, 24, 11, 52, 10, 0, time.Local))
	if level != LevelError {
		t.Errorf("stalled level = %q, want error", level)
	}
	if got := status[:7]; got != "STALLED" {
		t.Errorf("stalled status = %q", status)
	}
}

func TestSessionStateExecutingTools(t *testing.T) {
	s := NewSessionState("/tmp/logs/x.jsonl")
	// Model responded with a tool call, message appended, tools now running.
	s.Apply(mustEvent(t, `{"kind":"APIRequestCompleted","time":"2026-09-24T11:59:51+05:30","stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":10},"duration_ms":1,"model":"m","content":[{"type":"tool_call","data":{"id":"c","name":"Bash","input":{"cmd":"ls"}}}]}`))
	s.Apply(mustEvent(t, `{"kind":"MessageAppended","time":"2026-09-24T11:59:51.001+05:30","role":"assistant","content_types":["thinking","tool_call"],"token_estimate":50}`))
	status, level := s.Status(time.Date(2026, 9, 24, 11, 59, 54, 0, time.Local))
	if level != LevelInfo || status != "executing tools 3s" {
		t.Errorf("Status = (%q, %q), want executing tools 3s", status, level)
	}
}

func TestSessionStateCountsStampSizedUserMessages(t *testing.T) {
	s := NewSessionState("/tmp/logs/x.jsonl")
	// META-001: a wall-clock stamp-only companion message (harness
	// boilerplate, ~15 estimated tokens) must be counted as ergonomics
	// noise; real operator content and tool results must not be.
	lines := []string{
		`{"kind":"MessageAppended","time":"2026-09-24T11:59:54+05:30","role":"user","content_types":["text"],"token_estimate":15}`,
		`{"kind":"MessageAppended","time":"2026-09-24T12:00:10+05:30","role":"user","content_types":["text"],"token_estimate":23}`,        // another stamp
		`{"kind":"MessageAppended","time":"2026-09-24T12:00:20+05:30","role":"user","content_types":["text"],"token_estimate":2}`,         // operator "go"
		`{"kind":"MessageAppended","time":"2026-09-24T12:00:30+05:30","role":"user","content_types":["text","text"],"token_estimate":15}`, // multi-part real content
		`{"kind":"MessageAppended","time":"2026-09-24T12:00:40+05:30","role":"user","content_types":["tool_result"],"token_estimate":15}`,
		`{"kind":"MessageAppended","time":"2026-09-24T12:00:50+05:30","role":"assistant","content_types":["text"],"token_estimate":15}`,
	}
	for _, line := range lines {
		s.Apply(mustEvent(t, line))
	}
	if s.StampishUserMsgs != 2 {
		t.Errorf("StampishUserMsgs = %d, want 2 (only the stamp-sized single-text user messages)", s.StampishUserMsgs)
	}
}

func TestSessionStateLoopDetection(t *testing.T) {
	s := NewSessionState("/tmp/logs/x.jsonl")
	completed := func(at string) string {
		return `{"kind":"APIRequestCompleted","time":"` + at + `","stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":10},"duration_ms":1,"model":"m","content":[` +
			`{"type":"tool_call","data":{"id":"c","name":"Bash","input":{"cmd":"ls"}}}]}`
	}
	var alerts []Alert
	for _, at := range []string{"2026-09-24T10:00:01+05:30", "2026-09-24T10:00:02+05:30", "2026-09-24T10:00:03+05:30"} {
		alerts = append(alerts, s.Apply(mustEvent(t, completed(at)))...)
	}
	if len(alerts) != 1 || alerts[0].Kind != "tool_loop" || alerts[0].Level != LevelError {
		t.Fatalf("alerts = %+v, want one tool_loop error", alerts)
	}
	if s.consecSame != 3 {
		t.Errorf("consecSame = %d, want 3", s.consecSame)
	}

	// Different input resets the consecutive counter and re-arms detection.
	variated := `{"kind":"APIRequestCompleted","time":"2026-09-24T10:00:04+05:30","stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":10},"duration_ms":1,"model":"m","content":[` +
		`{"type":"tool_call","data":{"id":"c","name":"Bash","input":{"cmd":"ls -la"}}}]}`
	s.Apply(mustEvent(t, variated))
	if s.consecSame != 1 {
		t.Errorf("consecSame after variation = %d, want 1", s.consecSame)
	}
}

func TestSessionStateContextAlerts(t *testing.T) {
	s := NewSessionState("/tmp/logs/x.jsonl")
	s.ContextWindow = 100000
	completed := func(at string, input int) string {
		return `{"kind":"APIRequestCompleted","time":"` + at + `","stop_reason":"end_turn","usage":{"input_tokens":` +
			strconv.Itoa(input) + `,"output_tokens":5},"duration_ms":1,"model":"m","content":[]}`
	}
	var alerts []Alert
	alerts = append(alerts, s.Apply(mustEvent(t, completed("2026-09-24T10:00:01+05:30", 85000)))...)
	if len(alerts) != 1 || alerts[0].Kind != "context" || alerts[0].Level != LevelWarning {
		t.Fatalf("alerts at 85%% = %+v", alerts)
	}
	alerts = append(alerts, s.Apply(mustEvent(t, completed("2026-09-24T10:00:02+05:30", 96000)))...)
	if len(alerts) != 2 || alerts[1].Kind != "context" || alerts[1].Level != LevelError {
		t.Fatalf("alerts at 96%% = %+v", alerts)
	}
	// Repeat fill must not re-warn.
	alerts = append(alerts, s.Apply(mustEvent(t, completed("2026-09-24T10:00:03+05:30", 97000)))...)
	if len(alerts) != 2 {
		t.Fatalf("alerts after repeat = %+v, want no new", alerts)
	}
}

func TestSessionStateFailuresAndRetries(t *testing.T) {
	s := NewSessionState("/tmp/logs/x.jsonl")
	var alerts []Alert
	alerts = append(alerts, s.Apply(mustEvent(t,
		`{"kind":"APIRequestFailed","time":"2026-09-24T10:00:01+05:30","error_type":"rate_limit","error_message":"429 Too Many Requests","retryable":true,"attempt":1}`))...)
	if len(alerts) != 0 {
		t.Fatalf("retryable failure alerts = %+v, want none", alerts)
	}
	for i := 0; i < 5; i++ {
		alerts = append(alerts, s.Apply(mustEvent(t,
			`{"kind":"APIRetryScheduled","time":"2026-09-24T10:00:02+05:30","attempt":2,"delay_ms":600,"reason":"rate_limit"}`))...)
	}
	if len(alerts) != 1 || alerts[0].Kind != "retry_storm" {
		t.Fatalf("retry storm alerts = %+v", alerts)
	}
	alerts = append(alerts, s.Apply(mustEvent(t,
		`{"kind":"APIRequestFailed","time":"2026-09-24T10:00:03+05:30","error_type":"auth","error_message":"invalid key","retryable":false,"attempt":1}`))...)
	if len(alerts) != 2 || alerts[1].Kind != "api_failed" || alerts[1].Level != LevelError {
		t.Fatalf("unretryable alerts = %+v", alerts)
	}
	if s.LastError == "" || !strings.Contains(s.LastError, "invalid key") {
		t.Errorf("LastError = %q", s.LastError)
	}
	if s.InFlight {
		t.Error("InFlight after unretryable failure, want false")
	}
}
