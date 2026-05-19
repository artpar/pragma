package toolresultread

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/tool"
)

type testState struct {
	sessionID string
}

func (s testState) WorkDir() string   { return "" }
func (s testState) SessionID() string { return s.sessionID }

func TestToolReadsPersistedOutputPage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sessionID := "session-1"
	path := filepath.Join(home, ".pragma", "sessions", sessionID, "tool-results", "call_one.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("abcdefghijklmnopqrstuvwxyz"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := (&Tool{}).Invoke(
		context.Background(),
		json.RawMessage(`{"tool_call_id":"call/one","offset_bytes":5,"limit_bytes":7}`),
		testState{sessionID: sessionID},
	)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var out output
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.Content != "fghijkl" {
		t.Fatalf("Content = %q", out.Content)
	}
	if out.NextOffsetBytes != 12 || !out.HasMore || out.TotalBytes != 26 {
		t.Fatalf("pagination = next %d has_more %v total %d", out.NextOffsetBytes, out.HasMore, out.TotalBytes)
	}
}

func TestToolRequiresSessionID(t *testing.T) {
	_, err := (&Tool{}).Invoke(context.Background(), json.RawMessage(`{"tool_call_id":"call-one"}`), noSessionState{})
	if err == nil || !strings.Contains(err.Error(), "session id") {
		t.Fatalf("expected session id error, got %v", err)
	}
}

type noSessionState struct{}

func (noSessionState) WorkDir() string { return "" }

var _ tool.StateSnapshot = testState{}
var _ tool.StateSnapshot = noSessionState{}
