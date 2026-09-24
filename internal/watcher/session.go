// Package watcher accumulates the observable state of running pragma
// sessions from their event logs (~/.pragma/logs/*.jsonl) and derives
// status, health alerts, and summaries for a live observer.
//
// It is a read-only observer: it never writes pragma state, never
// interacts with providers, and only follows the log files that the
// pragma processes themselves produce through internal/observe.
package watcher

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/observe"
)

// Alert levels ordered by severity.
const (
	LevelInfo    = "info"
	LevelWarning = "warning"
	LevelError   = "error"
)

// Alert is a condition detected while observing a session.
type Alert struct {
	Session string    `json:"session"` // log file name
	Kind    string    `json:"kind"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

// ToolCall is one tool invocation requested by the model.
type ToolCall struct {
	Name    string    `json:"name"`
	Summary string    `json:"summary,omitempty"`
	At      time.Time `json:"at"`
}

// SessionState accumulates events from one pragma event log file.
// Apply is the single mutation path; every displayed field is derived.
type SessionState struct {
	LogPath string `json:"log_path"`
	Name    string `json:"name"` // log file base name
	PID     int    `json:"pid,omitempty"`

	FirstEventAt   time.Time `json:"first_event_at"`
	LastEventAt    time.Time `json:"last_event_at"`
	LastEventKind  string    `json:"last_event_kind"`
	LastCoreEvent  string    `json:"last_core_event"`  // last non-heartbeat event kind
	LastCoreAt     time.Time `json:"last_core_at"`     // time of last non-heartbeat event
	LastStopReason string    `json:"last_stop_reason"` // stop reason of last completed API call
	LastRole       string    `json:"last_role"`        // role of last appended message

	Model    string `json:"model,omitempty"`
	Turns    int    `json:"turns"`    // UserTurnAccepted count
	Messages int    `json:"messages"` // MessageAppended count
	APICalls int    `json:"api_calls"`
	Retries  int    `json:"retries"`
	Failures int    `json:"failures"`
	MaxHits  int    `json:"max_token_hits"` // stop_reason == max_tokens count

	LastError string `json:"last_error,omitempty"`

	// Token accounting. ContextFill is the context size of the most
	// recent request (input + cache reads + cache writes).
	ContextFill  int `json:"context_fill"`
	ContextPeak  int `json:"context_peak"` // largest context fill observed
	OutputTokens int `json:"output_tokens"`
	InputTokens  int `json:"input_tokens"` // cumulative non-cached input
	// StopReasonCounts is a histogram of response stop reasons.
	StopReasonCounts map[string]int `json:"stop_reasons"`

	ToolCalls      int            `json:"tool_calls"`
	ToolErrors     int            `json:"tool_errors"`
	ToolCallCounts map[string]int `json:"tool_call_counts"`
	RecentTools    []ToolCall     `json:"recent_tools"`

	// In-flight request tracking. A user-role MessageAppended (operator
	// prompt or tool results) is followed by exactly one API request;
	// the request is considered open until an APIRequestCompleted (or a
	// terminal failure) is observed.
	InFlight      bool      `json:"in_flight"`
	InFlightSince time.Time `json:"in_flight_since,omitempty"`

	// MCP server status: name -> last known status.
	Servers map[string]string `json:"mcp_servers,omitempty"`

	// Ergonomics (META-001): user text-only messages with stamp-sized
	// token estimates are almost always harness-injected boilerplate
	// (wall-clock stamps, notices) rather than operator content. High
	// counts signal conversation noise the operator pays for in context
	// and attention. Post-CLK-002 this should read zero.
	StampishUserMsgs int `json:"stampish_user_msgs"`

	// Detected alerts, newest last. Bounded by maxAlerts.
	Alerts []Alert `json:"alerts"`

	// Configuration knobs set by the observer.
	ContextWindow int           // 0 = unknown; show raw token counts only
	StallAfter    time.Duration // in-flight threshold for stall alerts

	// internal loop-detection state
	consecPrint   string // fingerprint of previous response's tool set
	consecSame    int    // consecutive identical tool-set responses
	loopAlerted   bool
	retryAlerted  bool
	warnedContext map[string]bool
}

const (
	maxRecentTools = 12
	maxAlerts      = 32
)

// NewSessionState creates state for the log at path.
func NewSessionState(path string) *SessionState {
	name := path
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		name = path[i+1:]
	}
	return &SessionState{
		LogPath:          path,
		Name:             name,
		ToolCallCounts:   map[string]int{},
		StopReasonCounts: map[string]int{},
		Servers:          map[string]string{},
		StallAfter:       10 * time.Minute,
		warnedContext:    map[string]bool{},
	}
}

// Apply folds one parsed event into the state and returns any new alerts.
// Unknown or irrelevant events update timestamps only.
func (s *SessionState) Apply(ev observe.Event) []Alert {
	if ev == nil {
		return nil
	}
	now := ev.EventTimestamp()
	if s.FirstEventAt.IsZero() {
		s.FirstEventAt = now
	}
	s.LastEventAt = now
	s.LastEventKind = ev.EventKind()

	var alerts []Alert
	switch e := ev.(type) {
	case observe.UserTurnAccepted:
		s.Turns++
	case observe.MessageAppended:
		s.Messages++
		s.LastRole = e.Role
		if e.Role == "user" && len(e.ContentTypes) == 1 && e.ContentTypes[0] == "text" &&
			e.TokenEstimate >= 8 && e.TokenEstimate <= 25 {
			s.StampishUserMsgs++
		}
		if e.Role == "user" {
			// Operator prompt or tool results: the next API request
			// is (about to be) in flight.
			s.InFlight = true
			s.InFlightSince = now
		}
	case observe.APIRequestCompleted:
		s.APICalls++
		s.Model = e.Model
		s.LastStopReason = string(e.StopReason)
		s.StopReasonCounts[string(e.StopReason)]++
		s.InFlight = false
		s.ContextFill = e.Usage.InputTokens + e.Usage.CacheReadInputTokens + e.Usage.CacheCreationInputTokens
		if s.ContextFill > s.ContextPeak {
			s.ContextPeak = s.ContextFill
		}
		s.InputTokens += e.Usage.InputTokens
		s.OutputTokens += e.Usage.OutputTokens
		if e.StopReason == "max_tokens" {
			s.MaxHits++
			alerts = append(alerts, s.alert(now, "max_tokens", LevelWarning,
				fmt.Sprintf("response hit max_tokens (output truncated); %d so far", s.MaxHits)))
		}
		alerts = append(alerts, s.applyToolCalls(e)...)
		alerts = append(alerts, s.checkContext(now)...)
	case observe.APIRequestFailed:
		s.Failures++
		s.LastError = fmt.Sprintf("%s: %s", e.ErrorType, truncate(e.ErrorMessage, 160))
		if !e.Retryable {
			s.InFlight = false
			alerts = append(alerts, s.alert(now, "api_failed", LevelError,
				fmt.Sprintf("unretryable API failure: %s", s.LastError)))
		}
	case observe.APIRetryScheduled:
		s.Retries++
		if s.Retries >= 5 && !s.retryAlerted {
			s.retryAlerted = true
			alerts = append(alerts, s.alert(now, "retry_storm", LevelError,
				fmt.Sprintf("retry storm: %d retries scheduled", s.Retries)))
		}
	case observe.ToolExecutionCompleted:
		if e.IsError {
			s.ToolErrors++
		}
	case observe.ToolExecutionFailed:
		s.ToolErrors++
	case observe.MCPServerConnected:
		s.Servers[e.ServerName] = "connected"
	case observe.MCPServerDisconnected:
		s.Servers[e.ServerName] = "disconnected"
		alerts = append(alerts, s.alert(now, "mcp_down", LevelWarning,
			fmt.Sprintf("MCP server %q disconnected: %s", e.ServerName, e.Reason)))
	case observe.MCPHealthCheck:
		s.Servers[e.ServerName] = e.Status
		// Periodic heartbeat; does not indicate conversational progress.
		return alerts
	case observe.ErrorOccurred:
		alerts = append(alerts, s.alert(now, "error", levelFor(e.Severity),
			fmt.Sprintf("%s: %s", e.Component, truncate(e.ErrorMessage, 160))))
	}

	s.LastCoreEvent = s.LastEventKind
	s.LastCoreAt = now
	s.appendAlerts(alerts)
	return alerts
}

// applyToolCalls extracts the tool calls requested in a completed response,
// records them, and performs loop detection across consecutive responses.
func (s *SessionState) applyToolCalls(e observe.APIRequestCompleted) []Alert {
	var content []struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if len(e.Content) > 0 {
		if err := json.Unmarshal(e.Content, &content); err != nil {
			content = nil
		}
	}

	var prints []string
	var alerts []Alert
	for _, item := range content {
		if item.Type != "tool_call" {
			continue
		}
		var call struct {
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		}
		if err := json.Unmarshal(item.Data, &call); err != nil {
			continue
		}
		prints = append(prints, call.Name+"\x00"+string(call.Input))

		s.ToolCalls++
		s.ToolCallCounts[call.Name]++
		s.RecentTools = append(s.RecentTools, ToolCall{
			Name:    call.Name,
			Summary: SummarizeInput(call.Input),
			At:      e.EventTimestamp(),
		})
		if len(s.RecentTools) > maxRecentTools {
			s.RecentTools = s.RecentTools[len(s.RecentTools)-maxRecentTools:]
		}
	}

	// Loop detection: the model is stuck when consecutive responses
	// request the exact same tool set (same names, same inputs, same order).
	fingerprint := strings.Join(prints, "\x01")
	if len(prints) > 0 && fingerprint == s.consecPrint {
		s.consecSame++
	} else {
		s.consecSame = 1
		s.consecPrint = fingerprint
	}
	if s.consecSame >= 3 && !s.loopAlerted {
		s.loopAlerted = true
		name := "tools"
		if len(prints) > 0 {
			name = strings.SplitN(prints[0], "\x00", 2)[0]
		}
		alerts = append(alerts, s.alert(e.EventTimestamp(), "tool_loop", LevelError,
			fmt.Sprintf("suspected loop: identical tool set (%s) requested in %d consecutive responses", name, s.consecSame)))
	} else if s.consecSame < 3 {
		s.loopAlerted = false
	}
	return alerts
}

// checkContext emits context-window pressure alerts when a window is known.
func (s *SessionState) checkContext(now time.Time) []Alert {
	if s.ContextWindow <= 0 || s.ContextFill <= 0 {
		return nil
	}
	pct := 100 * float64(s.ContextFill) / float64(s.ContextWindow)
	var alerts []Alert
	if pct >= 95 && !s.warnedContext["95"] {
		s.warnedContext["95"] = true
		alerts = append(alerts, s.alert(now, "context", LevelError,
			fmt.Sprintf("context fill %.0f%% of %s window", pct, HumanTokens(s.ContextWindow))))
	} else if pct >= 80 && !s.warnedContext["80"] {
		s.warnedContext["80"] = true
		alerts = append(alerts, s.alert(now, "context", LevelWarning,
			fmt.Sprintf("context fill %.0f%% of %s window", pct, HumanTokens(s.ContextWindow))))
	}
	return alerts
}

// Status derives a human status label plus a severity level for the session.
// now is the observation time, not the last event time.
func (s *SessionState) Status(now time.Time) (string, string) {
	if s.LastEventAt.IsZero() {
		return "no events yet", LevelInfo
	}
	switch {
	case s.InFlight:
		d := now.Sub(s.InFlightSince)
		if s.StallAfter > 0 && d > s.StallAfter {
			return fmt.Sprintf("STALLED in request %s (no completion since %s)",
				Dur(d), s.InFlightSince.Format("15:04:05")), LevelError
		}
		return fmt.Sprintf("model call in-flight %s", Dur(d)), LevelInfo
	case s.LastStopReason == "end_turn":
		return fmt.Sprintf("awaiting user (%s)", Dur(now.Sub(s.LastCoreAt))), LevelInfo
	case s.LastStopReason == "max_tokens":
		return "turn ended at max_tokens", LevelWarning
	case s.LastStopReason == "tool_use" && s.LastCoreEvent == "MessageAppended" && s.LastRole == "assistant":
		// The model requested tools and its message was appended; the
		// tools are executing until their results come back as the next
		// user-role append.
		return fmt.Sprintf("executing tools %s", Dur(now.Sub(s.LastCoreAt))), LevelInfo
	case s.LastCoreEvent == "APIRequestFailed":
		return "turn failed", LevelError
	case s.LastCoreEvent == "APIRequestCompleted":
		return "processing turn", LevelInfo
	default:
		return fmt.Sprintf("idle %s (last: %s)", Dur(now.Sub(s.LastCoreAt)), s.LastCoreEvent), LevelInfo
	}
}

func (s *SessionState) alert(now time.Time, kind, level, msg string) Alert {
	return Alert{Session: s.Name, Kind: kind, Level: level, Message: msg, At: now}
}

func (s *SessionState) appendAlerts(alerts []Alert) {
	s.Alerts = append(s.Alerts, alerts...)
	if len(s.Alerts) > maxAlerts {
		s.Alerts = s.Alerts[len(s.Alerts)-maxAlerts:]
	}
}

// ServerList returns MCP server names sorted alphabetically with status.
func (s *SessionState) ServerList() string {
	if len(s.Servers) == 0 {
		return ""
	}
	names := make([]string, 0, len(s.Servers))
	for name := range s.Servers {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = name + "=" + s.Servers[name]
	}
	return strings.Join(parts, ", ")
}

func levelFor(severity string) string {
	switch severity {
	case "error", "fatal", "critical":
		return LevelError
	case "warn", "warning":
		return LevelWarning
	default:
		return LevelInfo
	}
}

// SummarizeInput renders a short one-line summary of a tool input payload.
// Prefers the common descriptive fields (cmd, query, file_path, ...) and
// falls back to compact JSON.
func SummarizeInput(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(input, &m); err != nil {
		return truncate(string(input), 72)
	}
	for _, key := range []string{"cmd", "command", "query", "file_path", "path", "pattern", "url", "prompt", "description", "name"} {
		if v, ok := m[key]; ok && v != nil {
			switch t := v.(type) {
			case string:
				return truncate(strings.Join(strings.Fields(t), " "), 72)
			default:
				b, err := json.Marshal(t)
				if err != nil {
					continue
				}
				return truncate(string(b), 72)
			}
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return truncate(string(b), 72)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// HumanTokens renders a token count in k/M units.
func HumanTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// Dur renders a duration compactly (12s, 4m12s, 3h04m, 2d5h).
func Dur(d time.Duration) string {
	switch {
	case d < 0:
		return "0s"
	case d < time.Minute:
		return fmt.Sprintf("%.0fs", d.Seconds())
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}
