package observe

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/model"
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

func TestAPIRequestStartedPreservesReplayPayload(t *testing.T) {
	temp := 0.2
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}}}`)
	event := APIRequestStarted{
		EventHeader:   NewEventHeader("APIRequestStarted", "trace", "span", ""),
		Model:         "claude-sonnet-4-20250514",
		MaxTokens:     4096,
		MessageCount:  1,
		ToolCount:     1,
		TokenEstimate: 42,
		Messages: []model.Message{{
			ID:        "m1",
			Role:      model.RoleUser,
			Content:   []model.ContentPart{model.TextPart{Text: "inspect this"}},
			Timestamp: time.Unix(1, 0),
		}},
		SystemPrompt: model.SystemPrompt{Blocks: []model.SystemBlock{{
			Text:      "system",
			Cacheable: true,
		}}},
		Tools: []model.ToolDef{{
			Name:        "Read",
			Description: "read file",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		Temperature:    &temp,
		Thinking:       &APIThinkingConfig{Enabled: true, BudgetTokens: 1024},
		ResponseSchema: schema,
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	gotEvent, err := UnmarshalEvent(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, ok := gotEvent.(APIRequestStarted)
	if !ok {
		t.Fatalf("type: got %T, want APIRequestStarted", gotEvent)
	}
	if got.MaxTokens != event.MaxTokens || got.Model != event.Model || len(got.Messages) != 1 || len(got.Tools) != 1 {
		t.Fatalf("payload counts/model not preserved: %#v", got)
	}
	if got.SystemPrompt.Blocks[0].Text != "system" || !got.SystemPrompt.Blocks[0].Cacheable {
		t.Fatalf("system prompt not preserved: %#v", got.SystemPrompt)
	}
	if got.Temperature == nil || *got.Temperature != temp {
		t.Fatalf("temperature not preserved: %#v", got.Temperature)
	}
	if got.Thinking == nil || !got.Thinking.Enabled || got.Thinking.BudgetTokens != 1024 {
		t.Fatalf("thinking not preserved: %#v", got.Thinking)
	}
	if !bytes.Equal(got.ResponseSchema, schema) {
		t.Fatalf("response schema: got %s want %s", got.ResponseSchema, schema)
	}
}
