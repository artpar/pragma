package watcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildAndWritePostMortem(t *testing.T) {
	s := NewSessionState("/tmp/logs/2026-09-24T11-51-08.jsonl")
	events := []string{
		`{"kind":"UserTurnAccepted","time":"2026-09-24T11:51:10+05:30","prompt_chars":62}`,
		`{"kind":"MessageAppended","time":"2026-09-24T11:51:10.001+05:30","role":"user","content_types":["text"],"token_estimate":23}`,
		`{"kind":"APIRequestCompleted","time":"2026-09-24T11:51:20+05:30","stop_reason":"tool_use","usage":{"input_tokens":11646,"output_tokens":176},"duration_ms":1000,"model":"morph-glm53-744b","content":[{"type":"tool_call","data":{"id":"c1","name":"Bash","input":{"cmd":"ls"}}}]}`,
		`{"kind":"APIRequestCompleted","time":"2026-09-24T11:51:40+05:30","stop_reason":"end_turn","usage":{"input_tokens":20080,"cache_read_input_tokens":40000,"output_tokens":209},"duration_ms":2000,"model":"morph-glm53-744b","content":[]}`,
		`{"kind":"APIRequestFailed","time":"2026-09-24T11:52:00+05:30","error_type":"rate_limit","error_message":"429","retryable":true,"attempt":1}`,
	}
	for _, line := range events {
		s.Apply(mustEvent(t, line))
	}

	observed := time.Date(2026, 9, 24, 11, 53, 0, 0, time.Local)
	pm := BuildPostMortem(s, observed, "ended · awaiting user")
	if pm.Session != "2026-09-24T11-51-08.jsonl" {
		t.Errorf("Session = %q", pm.Session)
	}
	if pm.ContextPeak != 60080 {
		t.Errorf("ContextPeak = %d, want 60080", pm.ContextPeak)
	}
	if pm.StopReasons["tool_use"] != 1 || pm.StopReasons["end_turn"] != 1 {
		t.Errorf("StopReasons = %v", pm.StopReasons)
	}
	if pm.Retries != 0 || pm.Failures != 1 {
		t.Errorf("Retries=%d Failures=%d, want 0/1", pm.Retries, pm.Failures)
	}
	if len(pm.TopTools) != 1 || pm.TopTools[0].Name != "Bash" || pm.TopTools[0].Count != 1 {
		t.Errorf("TopTools = %+v", pm.TopTools)
	}
	if pm.FinalStatus != "ended · awaiting user" {
		t.Errorf("FinalStatus = %q", pm.FinalStatus)
	}

	dir := t.TempDir()
	path, err := WritePostMortem(dir, pm)
	if err != nil {
		t.Fatalf("WritePostMortem: %v", err)
	}
	if filepath.Base(path) != "2026-09-24T11-51-08.postmortem.json" {
		t.Errorf("postmortem file name = %q", filepath.Base(path))
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var back PostMortem
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.ContextPeak != pm.ContextPeak || back.Session != pm.Session {
		t.Errorf("round-trip mismatch: %+v vs %+v", back, pm)
	}

	// Rewrite must be idempotent (overwrite, not duplicate).
	if _, err := WritePostMortem(dir, pm); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("postmortem files = %d, want 1", len(entries))
	}
}

func TestAppendAlerts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alerts.jsonl")
	first := []Alert{{Session: "a.jsonl", Kind: "mcp_down", Level: LevelWarning, Message: "down", At: time.Now()}}
	if err := AppendAlerts(path, first); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if err := AppendAlerts(path, nil); err != nil {
		t.Fatalf("append empty: %v", err)
	}
	second := []Alert{{Session: "b.jsonl", Kind: "tool_loop", Level: LevelError, Message: "loop", At: time.Now()}}
	if err := AppendAlerts(path, second); err != nil {
		t.Fatalf("append 2: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := 0
	for _, line := range splitLines(string(b)) {
		if line == "" {
			continue
		}
		var a Alert
		if err := json.Unmarshal([]byte(line), &a); err != nil {
			t.Fatalf("bad alert line: %v", err)
		}
		lines++
	}
	if lines != 2 {
		t.Errorf("alert lines = %d, want 2", lines)
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
