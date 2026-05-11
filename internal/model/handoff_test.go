package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewHandoffState(t *testing.T) {
	state := NewHandoffState("finish a long task")
	if state.SchemaVersion != HandoffSchemaV1 {
		t.Fatalf("SchemaVersion = %q, want %q", state.SchemaVersion, HandoffSchemaV1)
	}
	if state.Goal != "finish a long task" {
		t.Fatalf("Goal = %q", state.Goal)
	}
	if len(state.Invariants) == 0 {
		t.Fatal("expected default invariants")
	}
}

func TestApplyHandoffPatch(t *testing.T) {
	state := NewHandoffState("inspect repo")
	raw := json.RawMessage(`{
		"ops": [
			{"op":"replace","path":"/current_focus","value":"read query loop"},
			{"op":"add","path":"/completed/-","value":"found provider request boundary"},
			{"op":"add","path":"/files/read/-","value":"internal/query/loop.go"}
		]
	}`)

	next, err := ApplyHandoffPatch(state, raw)
	if err != nil {
		t.Fatalf("ApplyHandoffPatch: %v", err)
	}
	if next.CurrentFocus != "read query loop" {
		t.Fatalf("CurrentFocus = %q", next.CurrentFocus)
	}
	if len(next.Completed) != 1 || next.Completed[0] != "found provider request boundary" {
		t.Fatalf("Completed = %#v", next.Completed)
	}
	if len(next.Files.Read) != 1 || next.Files.Read[0] != "internal/query/loop.go" {
		t.Fatalf("Files.Read = %#v", next.Files.Read)
	}
}

func TestApplyHandoffPatchRemove(t *testing.T) {
	state := NewHandoffState("inspect repo")
	state.OpenQuestions = []string{"old question", "new question"}

	next, err := ApplyHandoffPatch(state, json.RawMessage(`{"ops":[{"op":"remove","path":"/open_questions/0"}]}`))
	if err != nil {
		t.Fatalf("ApplyHandoffPatch: %v", err)
	}
	if len(next.OpenQuestions) != 1 || next.OpenQuestions[0] != "new question" {
		t.Fatalf("OpenQuestions = %#v", next.OpenQuestions)
	}
}

func TestApplyHandoffPatchInvalid(t *testing.T) {
	_, err := ApplyHandoffPatch(NewHandoffState("inspect repo"), json.RawMessage(`{"ops":[{"op":"replace","path":"/missing","value":"x"}]}`))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "cannot replace missing") {
		t.Fatalf("error = %v", err)
	}
}
