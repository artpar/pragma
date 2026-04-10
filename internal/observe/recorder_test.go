package observe

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
)

func TestRecorderWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	rec, err := NewRecorder(path)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	rec.HandleEvent(ConversationStarted{
		EventHeader:    NewEventHeader("ConversationStarted", "t1", "s1", ""),
		ConversationID: "conv-1",
		Model:          "test",
		Provider:       "test",
		WorkDir:        "/tmp",
	})
	rec.HandleEvent(ToolExecutionCompleted{
		EventHeader: NewEventHeader("ToolExecutionCompleted", "t1", "s2", ""),
		ToolCallID:  "tc-1",
		ToolName:    "Bash",
		DurationMs:  100,
	})

	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Read back and verify
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var events []Event
	for scanner.Scan() {
		ev, err := UnmarshalEvent(scanner.Bytes())
		if err != nil {
			t.Fatalf("UnmarshalEvent: %v", err)
		}
		events = append(events, ev)
	}

	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].EventKind() != "ConversationStarted" {
		t.Errorf("event[0]: got %q", events[0].EventKind())
	}
	if events[1].EventKind() != "ToolExecutionCompleted" {
		t.Errorf("event[1]: got %q", events[1].EventKind())
	}
}
