package observe

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestLoggerLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LevelInfo, FormatJSON, nil)

	// Debug event should be filtered by Info level
	logger.HandleEvent(ToolExecutionStarted{
		EventHeader: NewEventHeader("ToolExecutionStarted", "t1", "s1", ""),
		ToolCallID:  "tc-1",
		ToolName:    "Bash",
	})

	if buf.Len() != 0 {
		t.Errorf("expected no output for debug event at info level, got: %s", buf.String())
	}

	// Info event should pass
	logger.HandleEvent(ToolExecutionCompleted{
		EventHeader: NewEventHeader("ToolExecutionCompleted", "t1", "s2", ""),
		ToolCallID:  "tc-1",
		ToolName:    "Bash",
		DurationMs:  100,
	})

	if buf.Len() == 0 {
		t.Error("expected output for info event at info level")
	}
}

func TestLoggerTopicFiltering(t *testing.T) {
	var buf bytes.Buffer
	topics := map[string]bool{"api": true}
	logger := NewLogger(&buf, LevelTrace, FormatJSON, topics)

	// Tool event should be filtered
	logger.HandleEvent(ToolExecutionCompleted{
		EventHeader: NewEventHeader("ToolExecutionCompleted", "t1", "s1", ""),
		ToolCallID:  "tc-1",
		ToolName:    "Bash",
	})

	if buf.Len() != 0 {
		t.Errorf("expected no output for tool event with api-only filter")
	}

	// API event should pass
	logger.HandleEvent(APIRequestStarted{
		EventHeader: NewEventHeader("APIRequestStarted", "t1", "s2", ""),
		Model:       "test",
	})

	if buf.Len() == 0 {
		t.Error("expected output for api event with api filter")
	}
}

func TestLoggerJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LevelTrace, FormatJSON, nil)

	logger.HandleEvent(ConversationStarted{
		EventHeader:    NewEventHeader("ConversationStarted", "t1", "s1", ""),
		ConversationID: "conv-1",
		Model:          "test",
		Provider:       "test",
		WorkDir:        "/tmp",
	})

	var raw map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if raw["kind"] != "ConversationStarted" {
		t.Errorf("kind: got %v", raw["kind"])
	}
}

func TestLoggerTextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LevelTrace, FormatText, nil)

	logger.HandleEvent(ConversationStarted{
		EventHeader:    NewEventHeader("ConversationStarted", "t1", "s1", ""),
		ConversationID: "conv-1",
		Model:          "test",
		Provider:       "test",
		WorkDir:        "/tmp",
	})

	out := buf.String()
	if len(out) == 0 {
		t.Error("expected non-empty text output")
	}
}
