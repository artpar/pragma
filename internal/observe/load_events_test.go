package observe

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEventsSkipsUnregisteredKinds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rec.jsonl")
	known := `{"kind":"SessionSaved","time":"2026-09-11T07:53:58Z","trace_id":"t","span_id":"s"}` + "\n"
	unknown := `{"kind":"AgentMDNotFound","time":"2026-09-11T07:53:58Z","trace_id":"t","span_id":"s"}` + "\n"
	if err := os.WriteFile(path, []byte(known+unknown+known), 0o644); err != nil {
		t.Fatal(err)
	}
	events, err := LoadEvents(path)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events: got %d, want 2 (unregistered kind must be skipped, not fatal)", len(events))
	}
}

func TestUnmarshalEventUnknownKindIsTyped(t *testing.T) {
	_, err := UnmarshalEvent([]byte(`{"kind":"AgentMDNotFound"}`))
	var unknown UnknownEventKindError
	if !errors.As(err, &unknown) {
		t.Fatalf("expected UnknownEventKindError, got %v", err)
	}
	if unknown.Kind != "AgentMDNotFound" {
		t.Fatalf("kind: got %q, want AgentMDNotFound", unknown.Kind)
	}
}

func TestLoadEventsDirectorySingleFileFallback(t *testing.T) {
	dir := t.TempDir()
	// Recorder-style layout: a lone timestamped jsonl file, no events.jsonl.
	path := filepath.Join(dir, "20260911T075358.350980000Z.jsonl")
	if err := os.WriteFile(path, []byte(`{"kind":"SessionSaved","time":"2026-09-11T07:53:58Z","trace_id":"t","span_id":"s"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	events, err := LoadEvents(dir)
	if err != nil {
		t.Fatalf("load events from directory: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events: got %d, want 1", len(events))
	}
}

func TestLoadEventsDirectoryPrefersCanonicalEventsFile(t *testing.T) {
	dir := t.TempDir()
	canonical := `{"kind":"SessionSaved","time":"2026-09-11T07:53:58Z","trace_id":"t1","span_id":"s"}` + "\n"
	other := `{"kind":"SessionSaved","time":"2026-09-11T08:00:00Z","trace_id":"t2","span_id":"s"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(canonical), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "20260911T075358.350980000Z.jsonl"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	events, err := LoadEvents(dir)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	if len(events) != 1 || events[0].EventTraceID() != "t1" {
		t.Fatalf("expected canonical events.jsonl to be preferred, got %d events", len(events))
	}
}

func TestLoadEventsDirectoryMultipleFilesAmbiguous(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.jsonl", "b.jsonl"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"kind":"SessionSaved"}`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, err := LoadEvents(dir)
	if err == nil || !strings.Contains(err.Error(), "multiple event files") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}
