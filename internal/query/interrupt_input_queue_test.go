package query

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

// INT-001 engine gates: operator input submitted mid-turn reaches the
// conversation without breaking tool_result pairing — it parks while the
// tail is an assistant tool_use awaiting results and flushes at the loop's
// next safe point; through a real loop run it is delivered on the next
// request after the tool results and companion.

func TestAppendUserInputParksBehindDanglingToolUse(t *testing.T) {
	engine := newProviderToolsTestEngine(t, &pragmaLoopTestProvider{})
	toolCallInput, err := json.Marshal(map[string]string{"cmd": "echo X"})
	if err != nil {
		t.Fatal(err)
	}
	before := len(engine.store.Snapshot().Conversation.Messages)

	// The loop's mid-tool-execution state: an assistant tool_use whose
	// results have not been appended yet.
	if err := engine.appendConversationMessage(model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleAssistant,
		Content:   []model.ContentPart{model.ToolCallPart{ID: "call-q", Name: "Bash", Input: toolCallInput}},
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	// Queued input during the dangling window must park, not interleave
	// between the tool call and its results.
	if err := engine.AppendUserInput("parked operator note"); err != nil {
		t.Fatalf("AppendUserInput during dangling tail: %v", err)
	}
	msgs := engine.store.Snapshot().Conversation.Messages
	if len(msgs) != before+1 {
		t.Fatalf("conversation gained %d messages while parked (want only the assistant): %d", len(msgs)-before-1, len(msgs))
	}
	if msgs[len(msgs)-1].Role != model.RoleAssistant {
		t.Fatalf("tail must still be the assistant tool_use message; got role %q", msgs[len(msgs)-1].Role)
	}

	// The loop completes the batch: tool results, then the companion.
	if err := engine.appendConversationMessage(model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.ToolResultPart{ToolCallID: "call-q", Content: "done"}},
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := engine.appendConversationMessage(model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: wallClockStamp(time.Now())}},
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	// Drain at the safe point: the parked note lands after the companion.
	engine.drainPendingUserInputs()
	msgs = engine.store.Snapshot().Conversation.Messages
	tail := msgs[len(msgs)-1]
	if tail.Role != model.RoleUser || !strings.Contains(messageText(tail), "parked operator note") {
		t.Fatalf("drained queued note must be the last message (user, note text); got role %q content %+v", tail.Role, tail.Content)
	}

	// The full conversation must pass request validation — the parked
	// message never broke tool_result pairing.
	if _, err := engine.messagesForRequestChecked(engine.store.Snapshot().Conversation); err != nil {
		t.Fatalf("pairing validation failed after drain: %v", err)
	}
}

func TestProviderToolsLoopDeliversQueuedInputAtNextRequest(t *testing.T) {
	prov := &pragmaLoopTestProvider{responses: []model.Response{
		{
			Content: []model.ContentPart{
				model.ToolCallPart{ID: "call-slow", Name: "Bash", Input: []byte(`{"cmd": "sleep 1; echo SLOW_DONE"}`)},
			},
			StopReason: model.StopToolUse,
		},
		{
			Content:    []model.ContentPart{model.TextPart{Text: "done"}},
			StopReason: model.StopEndTurn,
		},
	}}
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          t.TempDir(),
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	engine := NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(64), EngineConfig{
		Model:     "test-model",
		LoopMode:  LoopModeProviderTools,
		MaxTokens: 4096,
		MaxTurns:  5,
	})

	eventsDone := make(chan []LoopEvent, 1)
	go func() {
		eventsDone <- collectPragmaLoopEvents(engine.Run(t.Context(), "work"))
	}()

	// Wait for the dangling window (assistant tool_use appended, slow tool
	// still executing), then submit operator input mid-turn.
	deadline := time.Now().Add(10 * time.Second)
	sawDangling := false
	for time.Now().Before(deadline) {
		if conversationTailHasDanglingToolUse(engine.store.Snapshot().Conversation) {
			sawDangling = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !sawDangling {
		t.Fatal("loop never entered the dangling tool-execution window")
	}
	if err := engine.AppendUserInput("mid-turn operator note"); err != nil {
		t.Fatalf("AppendUserInput mid-loop: %v", err)
	}

	events := <-eventsDone
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2 (loop must complete both turns)", prov.calls)
	}
	for _, ev := range events {
		if _, isErr := ev.(ErrorEvent); isErr {
			t.Fatalf("loop errored after queued input: %v", ev)
		}
	}

	// The second request carries the queued note as a user message after
	// the tool results and the companion.
	req2 := prov.requests[1].Messages
	resultsIdx, companionIdx, noteIdx := -1, -1, -1
	for i, msg := range req2 {
		for _, part := range msg.Content {
			switch p := part.(type) {
			case model.ToolResultPart:
				if p.ToolCallID == "call-slow" {
					resultsIdx = i
				}
			case model.TextPart:
				// The companion is the stamp-only user message (the prompt
				// and the queued note carry a stamp line plus body text).
				if _, stampOnly := parseWallClockStamp(p.Text); stampOnly && msg.Role == model.RoleUser {
					companionIdx = i
				}
				if strings.Contains(p.Text, "mid-turn operator note") {
					noteIdx = i
				}
			}
		}
	}
	if resultsIdx < 0 || companionIdx < 0 {
		t.Fatalf("request 2 missing tool results (%d) or companion (%d)", resultsIdx, companionIdx)
	}
	if noteIdx < 0 {
		t.Fatalf("queued note never reached request 2; messages: %+v", req2)
	}
	if noteIdx < companionIdx || companionIdx < resultsIdx {
		t.Fatalf("order broken: results=%d companion=%d note=%d (note must follow the companion)", resultsIdx, companionIdx, noteIdx)
	}

	// The note persisted in the conversation for later requests.
	found := false
	for _, msg := range engine.store.Snapshot().Conversation.Messages {
		if strings.Contains(messageText(msg), "mid-turn operator note") {
			found = true
		}
	}
	if !found {
		t.Fatal("queued note absent from the persisted conversation")
	}
}

func messageText(msg model.Message) string {
	var sb strings.Builder
	for _, part := range msg.Content {
		if tp, ok := part.(model.TextPart); ok {
			sb.WriteString(tp.Text)
		}
	}
	return sb.String()
}

// newProviderToolsTestEngine builds a minimal provider-tools engine for
// queue/alias gates: a stub provider over a fresh conversation store.
func newProviderToolsTestEngine(t *testing.T, prov provider.Provider) *Engine {
	t.Helper()
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", t.TempDir())
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          t.TempDir(),
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	return NewEngine(prov, store, model.NewCostTracker(0), observe.NewEventBus(64), EngineConfig{
		Model:     "test-model",
		LoopMode:  LoopModeProviderTools,
		MaxTokens: 4096,
		MaxTurns:  5,
	})
}
