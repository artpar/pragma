// Package metaobserve adds a model-based judging layer on top of the
// mechanical watcher: it periodically samples the tail of each live
// pragma session (event log joined to the session file by session UUID),
// runs one cheap critic model call over a bounded digest, and appends
// findings with session+time provenance to the self-improvement queue.
//
// It never writes pragma state: its only writes are its own findings and
// log records. The critic call is the only external side effect.
package metaobserve

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/observe"
)

// SessionSample is one bounded observation of a live session: the event
// log lines inside the sample window plus the tail of the durable session
// file (which carries the operator/assistant text the event log omits).
type SessionSample struct {
	LogName     string    // event log base name
	LogPath     string    // event log path
	SessionID   string    // last SessionSaved session UUID
	WindowFrom  time.Time // window start (last sample time)
	WindowTo    time.Time // window end (this sample time)
	LogLines    []string  // raw event lines, oldest first
	SessionMsgs []string  // raw session-file lines (messages + last metadata), oldest first
}

// readTail caps how much of the tail of a log file is read per sample.
const (
	logTailBytes     = 512 * 1024
	sessionTailBytes = 256 * 1024
)

// readTail returns the last bytes of path (bounded by limit), split into
// non-empty lines. The first line may be partial (cut mid-line) and is
// dropped.
func readTail(path string, limit int64) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	off := fi.Size() - limit
	if off < 0 {
		off = 0
	}
	if _, err := f.Seek(off, 0); err != nil {
		return nil, err
	}
	buf := make([]byte, fi.Size()-off)
	if _, err := f.Read(buf); err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	if off > 0 && len(lines) > 0 {
		lines = lines[1:] // drop partial first line
	}
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out, nil
}

// eventEnvelope is the minimal shape every event line carries.
type eventEnvelope struct {
	Kind string    `json:"kind"`
	Time time.Time `json:"time"`
}

// CollectSample gathers the bounded tail of one session for the window
// [from, to]. The session file is found through the last SessionSaved
// event's session UUID; if the window contains no SessionSaved, the
// caller should fall back to the previously observed UUID.
func CollectSample(logPath, sessionsDir string, from, to time.Time, maxLogLines, maxSessionMsgs int) (*SessionSample, error) {
	all, err := readTail(logPath, logTailBytes)
	if err != nil {
		return nil, err
	}
	s := &SessionSample{
		LogPath:    logPath,
		LogName:    filepath.Base(logPath),
		WindowFrom: from,
		WindowTo:   to,
	}
	var sessionID string
	for _, line := range all {
		var env eventEnvelope
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			continue
		}
		if env.Time.Before(from) || env.Time.After(to) {
			continue
		}
		s.LogLines = append(s.LogLines, line)
		if env.Kind == "SessionSaved" {
			var e observe.SessionSaved
			if err := json.Unmarshal([]byte(line), &e); err == nil && e.SessionID != "" {
				sessionID = e.SessionID
			}
		}
	}
	if n := len(s.LogLines); n > maxLogLines {
		s.LogLines = s.LogLines[n-maxLogLines:]
	}
	s.SessionID = sessionID
	if sessionID == "" || sessionsDir == "" {
		return s, nil
	}
	sessPath := filepath.Join(sessionsDir, sessionID+".jsonl")
	raw, err := readTail(sessPath, sessionTailBytes)
	if err != nil {
		return s, nil // event tail alone is still usable
	}
	msgs := make([]string, 0, len(raw))
	lastMeta := ""
	for _, line := range raw {
		if strings.Contains(line, `"kind": "message"`) || strings.Contains(line, `"kind":"message"`) {
			msgs = append(msgs, line)
		} else if strings.Contains(line, `"kind": "metadata"`) || strings.Contains(line, `"kind":"metadata"`) {
			lastMeta = line
		}
	}
	if n := len(msgs); n > maxSessionMsgs {
		msgs = msgs[n-maxSessionMsgs:]
	}
	if lastMeta != "" {
		msgs = append(msgs, lastMeta)
	}
	s.SessionMsgs = msgs
	return s, nil
}

// truncateRunes cuts s to at most n runes with an ellipsis marker.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// renderEventLine renders one raw event line for the digest.
func renderEventLine(b *strings.Builder, line string) {
	var env eventEnvelope
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		return
	}
	ts := env.Time.Format("15:04:05")
	switch env.Kind {
	case "MessageAppended":
		var e observe.MessageAppended
		if json.Unmarshal([]byte(line), &e) != nil {
			return
		}
		fmt.Fprintf(b, "%s MessageAppended role=%s types=%v est=%dtok\n", ts, e.Role, e.ContentTypes, e.TokenEstimate)
	case "UserTurnAccepted":
		var e observe.UserTurnAccepted
		if json.Unmarshal([]byte(line), &e) != nil {
			return
		}
		fmt.Fprintf(b, "%s UserTurnAccepted operator input chars=%d\n", ts, e.PromptChars)
	case "APIRequestCompleted":
		var e observe.APIRequestCompleted
		if json.Unmarshal([]byte(line), &e) != nil {
			return
		}
		fmt.Fprintf(b, "%s APIRequestCompleted stop=%s in=%d out=%d dur=%dms\n",
			ts, e.StopReason, e.Usage.InputTokens, e.Usage.OutputTokens, e.DurationMs)
		renderContentItems(b, e.Content)
	case "APIRequestFailed":
		var e observe.APIRequestFailed
		if json.Unmarshal([]byte(line), &e) != nil {
			return
		}
		fmt.Fprintf(b, "%s APIRequestFailed %s: %s\n", ts, e.ErrorType, truncateRunes(e.ErrorMessage, 120))
	case "APIRetryScheduled":
		var e observe.APIRetryScheduled
		if json.Unmarshal([]byte(line), &e) != nil {
			return
		}
		fmt.Fprintf(b, "%s APIRetryScheduled attempt=%d delay=%dms %s\n", ts, e.Attempt, e.DelayMs, truncateRunes(e.Reason, 80))
	case "CompactionStarted":
		var e observe.CompactionStarted
		if json.Unmarshal([]byte(line), &e) != nil {
			return
		}
		fmt.Fprintf(b, "%s CompactionStarted pre=%d msgs=%d\n", ts, e.PreTokenCount, e.MessageCount)
	case "CompactionCompleted":
		var e observe.CompactionCompleted
		if json.Unmarshal([]byte(line), &e) != nil {
			return
		}
		fmt.Fprintf(b, "%s CompactionCompleted post=%d summarized=%d\n", ts, e.PostTokenCount, e.SummarizedCount)
	case "CompactionFailed":
		var e observe.CompactionFailed
		if json.Unmarshal([]byte(line), &e) != nil {
			return
		}
		fmt.Fprintf(b, "%s CompactionFailed %s: %s\n", ts, e.ErrorType, truncateRunes(e.ErrorMessage, 120))
	case "MCPServerConnected":
		var e observe.MCPServerConnected
		if json.Unmarshal([]byte(line), &e) != nil {
			return
		}
		fmt.Fprintf(b, "%s MCPServerConnected %s tools=%d\n", ts, e.ServerName, e.ToolCount)
	case "MCPServerDisconnected":
		var e observe.MCPServerDisconnected
		if json.Unmarshal([]byte(line), &e) != nil {
			return
		}
		fmt.Fprintf(b, "%s MCPServerDisconnected %s: %s\n", ts, e.ServerName, truncateRunes(e.Reason, 80))
	case "MCPServerFailed":
		var e observe.MCPServerFailed
		if json.Unmarshal([]byte(line), &e) != nil {
			return
		}
		fmt.Fprintf(b, "%s MCPServerFailed %s: %s\n", ts, e.ServerName, truncateRunes(e.ErrorMessage, 80))
	case "ErrorOccurred":
		var e observe.ErrorOccurred
		if json.Unmarshal([]byte(line), &e) != nil {
			return
		}
		fmt.Fprintf(b, "%s ErrorOccurred %s %s: %s\n", ts, e.Severity, e.Component, truncateRunes(e.ErrorMessage, 120))
	case "MCPHealthCheck", "SessionSaved":
		// periodic keepalive / pure persistence bookkeeping: no digest value
	default:
		fmt.Fprintf(b, "%s %s\n", ts, env.Kind)
	}
}

// renderContentItems renders thinking text and tool calls from an
// APIRequestCompleted content array.
func renderContentItems(b *strings.Builder, raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	var items []struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &items) != nil {
		return
	}
	for _, it := range items {
		switch it.Type {
		case "thinking":
			var d struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(it.Data, &d) == nil && strings.TrimSpace(d.Text) != "" {
				fmt.Fprintf(b, "   thinking: %s\n", truncateRunes(strings.ReplaceAll(d.Text, "\n", " "), 240))
			}
		case "tool_call":
			var d struct {
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			}
			if json.Unmarshal(it.Data, &d) == nil {
				fmt.Fprintf(b, "   tool_call %s %s\n", d.Name, truncateRunes(string(d.Input), 160))
			}
		case "text":
			var d struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(it.Data, &d) == nil && strings.TrimSpace(d.Text) != "" {
				fmt.Fprintf(b, "   text: %s\n", truncateRunes(strings.ReplaceAll(d.Text, "\n", " "), 240))
			}
		}
	}
}

// renderSessionMsg renders one session-file line (message or metadata).
func renderSessionMsg(b *strings.Builder, line string) {
	if strings.Contains(line, `"kind": "metadata"`) || strings.Contains(line, `"kind":"metadata"`) {
		var d struct {
			Data struct {
				CostUSD float64 `json:"cost_usd"`
				Tokens  struct {
					Input  int `json:"input_tokens"`
					Output int `json:"output_tokens"`
				} `json:"token_usage"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(line), &d) != nil {
			return
		}
		fmt.Fprintf(b, "[session totals] cost $%.2f input=%d tok output=%d tok\n",
			d.Data.CostUSD, d.Data.Tokens.Input, d.Data.Tokens.Output)
		return
	}
	var m struct {
		Data struct {
			Role    string    `json:"role"`
			Time    time.Time `json:"timestamp"`
			Content []struct {
				Type string          `json:"type"`
				Data json.RawMessage `json:"data"`
			} `json:"content"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(line), &m) != nil {
		return
	}
	if m.Data.Role == "" {
		return
	}
	for _, part := range m.Data.Content {
		switch part.Type {
		case "text":
			var d struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(part.Data, &d) == nil {
				label := "assistant"
				if m.Data.Role == "user" {
					label = "OPERATOR"
				}
				fmt.Fprintf(b, "[%s %s] %s\n", m.Data.Time.Format("15:04:05"), label,
					truncateRunes(strings.ReplaceAll(strings.TrimSpace(d.Text), "\n", " ↩ "), 500))
			}
		case "thinking":
			var d struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(part.Data, &d) == nil {
				fmt.Fprintf(b, "[%s thinking] %s\n", m.Data.Time.Format("15:04:05"),
					truncateRunes(strings.ReplaceAll(d.Text, "\n", " "), 240))
			}
		case "tool_call":
			var d struct {
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			}
			if json.Unmarshal(part.Data, &d) == nil {
				fmt.Fprintf(b, "[%s tool_call] %s %s\n", m.Data.Time.Format("15:04:05"), d.Name,
					truncateRunes(string(d.Input), 120))
			}
		case "tool_result":
			var d struct {
				Content string `json:"content"`
				IsError bool   `json:"is_error"`
			}
			if json.Unmarshal(part.Data, &d) == nil {
				tag := ""
				if d.IsError {
					tag = " ERROR"
				}
				fmt.Fprintf(b, "[%s tool_result%s] %s\n", m.Data.Time.Format("15:04:05"), tag,
					truncateRunes(strings.ReplaceAll(strings.TrimSpace(d.Content), "\n", " ↩ "), 160))
			}
		}
	}
}

// Digest renders the bounded critic-facing text for this sample.
func (s *SessionSample) Digest(maxChars int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "== pragma meta-observe sample ==\n")
	fmt.Fprintf(&b, "session_log: %s\n", s.LogName)
	fmt.Fprintf(&b, "session_id: %s\n", s.SessionID)
	fmt.Fprintf(&b, "window: %s .. %s\n", s.WindowFrom.Format(time.RFC3339), s.WindowTo.Format(time.RFC3339))
	fmt.Fprintf(&b, "\n-- event log tail (%d events) --\n", len(s.LogLines))
	for _, line := range s.LogLines {
		renderEventLine(&b, line)
	}
	if len(s.SessionMsgs) > 0 {
		fmt.Fprintf(&b, "\n-- session conversation tail (%d lines) --\n", len(s.SessionMsgs))
		for _, line := range s.SessionMsgs {
			renderSessionMsg(&b, line)
		}
	} else {
		fmt.Fprintf(&b, "\n-- session conversation tail unavailable (no session link yet) --\n")
	}
	out := b.String()
	if len(out) > maxChars {
		out = out[:maxChars] + "\n...(digest truncated)"
	}
	return out
}
