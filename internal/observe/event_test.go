package observe

import (
	"encoding/json"
	"testing"

	"github.com/artpar/gogent/internal/model"
)

func TestEventJSONRoundTrip(t *testing.T) {
	events := []Event{
		ConversationStarted{
			EventHeader:    NewEventHeader("ConversationStarted", "t1", "s1", ""),
			ConversationID: "conv-1",
			Model:          "claude-sonnet-4-20250514",
			Provider:       "anthropic",
			WorkDir:        "/tmp",
		},
		APIRequestCompleted{
			EventHeader: NewEventHeader("APIRequestCompleted", "t1", "s2", ""),
			StopReason:  model.StopToolUse,
			Usage:       model.TokenUsage{InputTokens: 1500, OutputTokens: 200},
			DurationMs:  342,
			Model:       "claude-sonnet-4-20250514",
		},
		ToolExecutionCompleted{
			EventHeader:     NewEventHeader("ToolExecutionCompleted", "t1", "s3", ""),
			ToolCallID:      "tc-1",
			ToolName:        "Bash",
			DurationMs:      123,
			OutputSizeBytes: 456,
			IsError:         false,
		},
		PermissionDenialEnforced{
			EventHeader: NewEventHeader("PermissionDenialEnforced", "t1", "s4", ""),
			ToolCallID:  "tc-2",
			ToolName:    "Bash",
			WasExecuted: true,
		},
		SessionEnded{
			EventHeader:  NewEventHeader("SessionEnded", "t1", "s5", ""),
			SessionID:    "sess-1",
			DurationMs:   60000,
			TurnCount:    5,
			TotalCostUSD: 0.05,
		},
	}

	for _, event := range events {
		t.Run(event.EventKind(), func(t *testing.T) {
			data, err := json.Marshal(event)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			got, err := UnmarshalEvent(data)
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if got.EventKind() != event.EventKind() {
				t.Errorf("kind: got %q, want %q", got.EventKind(), event.EventKind())
			}
			if got.EventTraceID() != event.EventTraceID() {
				t.Errorf("trace: got %q, want %q", got.EventTraceID(), event.EventTraceID())
			}
		})
	}
}

func TestUnmarshalEventUnknownKind(t *testing.T) {
	data := []byte(`{"kind":"UnknownEvent","time":"2026-04-10T00:00:00Z","trace_id":"t","span_id":"s"}`)
	_, err := UnmarshalEvent(data)
	if err == nil {
		t.Fatal("expected error for unknown event kind")
	}
}
