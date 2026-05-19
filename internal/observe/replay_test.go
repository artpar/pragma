package observe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/artpar/pragma/internal/model"
)

func TestLoadReplayIndexesAPIRequestPayloads(t *testing.T) {
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, "events.jsonl")
	events := []Event{
		APIRequestStarted{
			EventHeader:  NewEventHeader("APIRequestStarted", "trace", "span-1", ""),
			Model:        "model-1",
			MessageCount: 1,
			Messages: []model.Message{{
				ID:      "m1",
				Role:    model.RoleUser,
				Content: []model.ContentPart{model.TextPart{Text: "first"}},
			}},
		},
		MessageAppended{
			EventHeader: NewEventHeader("MessageAppended", "trace", "span-mid", ""),
			MessageID:   "m2",
			Role:        "assistant",
		},
		APIRequestStarted{
			EventHeader:  NewEventHeader("APIRequestStarted", "trace", "span-2", ""),
			Model:        "model-2",
			MessageCount: 1,
			Messages: []model.Message{{
				ID:      "m3",
				Role:    model.RoleUser,
				Content: []model.ContentPart{model.TextPart{Text: "second"}},
			}},
		},
	}
	f, err := os.Create(eventsPath)
	if err != nil {
		t.Fatalf("create events: %v", err)
	}
	enc := json.NewEncoder(f)
	for _, ev := range events {
		if err := enc.Encode(ev); err != nil {
			t.Fatalf("encode event: %v", err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close events: %v", err)
	}

	engine, err := LoadReplay(dir)
	if err != nil {
		t.Fatalf("load replay: %v", err)
	}
	req, ok := engine.APIRequest(2)
	if !ok {
		t.Fatal("missing request for turn 2")
	}
	if req.Model != "model-2" || len(req.Messages) != 1 {
		t.Fatalf("wrong request payload: %#v", req)
	}
	if _, ok := engine.APIRequest(3); ok {
		t.Fatal("unexpected request for turn 3")
	}
}
