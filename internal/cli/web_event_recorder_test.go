package cli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/session"
)

func TestSessionWebEventRecorderBuffersUntilSessionWriterExists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := session.NewStore()
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	rt := &InteractiveRuntime{Deps: &Deps{}}
	record := newSessionWebEventRecorder(rt)
	received := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	if err := record(session.WebEventData{
		Sequence:   1,
		ReceivedAt: received,
		Type:       "prompt_accepted",
		DataType:   "interactive.AcceptedPromptEvent",
		Data:       json.RawMessage(`{"prompt":"hello"}`),
	}); err != nil {
		t.Fatal(err)
	}

	w, err := store.Create(session.HeaderData{
		SessionID: "buffered-web-events",
		Model:     "glm",
		Provider:  "lilac",
		WorkDir:   "/repo",
		CreatedAt: received,
		System:    model.SystemPrompt{},
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	rt.Deps.SessionWriter = w
	if err := record(session.WebEventData{
		Sequence:   2,
		ReceivedAt: received.Add(time.Second),
		Type:       "text",
		DataType:   "query.TextEvent",
		Data:       json.RawMessage(`{"text":"world"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	sess, err := store.Load("buffered-web-events")
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.WebEvents) != 2 {
		t.Fatalf("web event count: got %d, want 2", len(sess.WebEvents))
	}
	if sess.WebEvents[0].Type != "prompt_accepted" || string(sess.WebEvents[0].Data) != `{"prompt":"hello"}` {
		t.Fatalf("first event lost or changed: %#v", sess.WebEvents[0])
	}
	if sess.WebEvents[1].Type != "text" || string(sess.WebEvents[1].Data) != `{"text":"world"}` {
		t.Fatalf("second event lost or changed: %#v", sess.WebEvents[1])
	}
}

func TestSessionWebEventRecorderCapsPendingEvents(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := session.NewStore()
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	rt := &InteractiveRuntime{Deps: &Deps{}}
	record := newSessionWebEventRecorder(rt)
	received := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	for sequence := 1; sequence <= maxPendingWebEvents+5; sequence++ {
		if err := record(session.WebEventData{
			Sequence:   sequence,
			ReceivedAt: received,
			Type:       "slash_result",
			DataType:   "slash.Result",
			Data:       json.RawMessage(`{"DisplayText":"help"}`),
		}); err != nil {
			t.Fatal(err)
		}
	}

	w, err := store.Create(session.HeaderData{
		SessionID: "capped-web-events",
		Model:     "glm",
		Provider:  "lilac",
		WorkDir:   "/repo",
		CreatedAt: received,
		System:    model.SystemPrompt{},
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	rt.Deps.SessionWriter = w
	if err := record(session.WebEventData{
		Sequence:   maxPendingWebEvents + 6,
		ReceivedAt: received,
		Type:       "text",
		DataType:   "query.TextEvent",
		Data:       json.RawMessage(`{"text":"started"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	sess, err := store.Load("capped-web-events")
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.WebEvents) != maxPendingWebEvents+1 {
		t.Fatalf("web event count: got %d, want %d", len(sess.WebEvents), maxPendingWebEvents+1)
	}
	if sess.WebEvents[0].Sequence != 6 {
		t.Fatalf("first retained sequence: got %d, want 6", sess.WebEvents[0].Sequence)
	}
}
