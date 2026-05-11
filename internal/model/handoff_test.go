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
	if !strings.Contains(strings.Join(state.Invariants, "\n"), "Maintain todos") {
		t.Fatalf("default invariants missing todo guidance: %#v", state.Invariants)
	}
	if state.LatestToolResultInterpretation == "" {
		t.Fatal("expected default LatestToolResultInterpretation")
	}
	if state.NextAction == "" {
		t.Fatal("expected default NextAction")
	}
}

func TestApplyHandoffPatch(t *testing.T) {
	state := NewHandoffState("inspect repo")
	raw := json.RawMessage(`{
		"ops": [
				{"op":"replace","path":"/current_focus","value":"read query loop"},
				{"op":"add","path":"/todos/-","value":{"id":"read-query","task":"Read query loop","status":"completed"}},
				{"op":"add","path":"/recent_actions/-","value":"Read internal/query/loop.go"},
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
	if len(next.Todos) != 1 || next.Todos[0].ID != "read-query" || next.Todos[0].Status != "completed" {
		t.Fatalf("Todos = %#v", next.Todos)
	}
	if len(next.RecentActions) != 1 || next.RecentActions[0] != "Read internal/query/loop.go" {
		t.Fatalf("RecentActions = %#v", next.RecentActions)
	}
	if len(next.Files.Read) != 1 || next.Files.Read[0] != "internal/query/loop.go" {
		t.Fatalf("Files.Read = %#v", next.Files.Read)
	}
}

func TestApplyHandoffPatchAllowsArbitraryAddPaths(t *testing.T) {
	state := NewHandoffState("inspect repo")
	raw := json.RawMessage(`{
		"ops": [
			{"op":"add","path":"/files/identified/-","value":"internal/query/loop.go"},
			{"op":"add","path":"/scratch/findings/-","value":{"file":"internal/model/handoff.go","status":"reviewed"}},
			{"op":"replace","path":"/todos/-","value":{"id":"report","task":"Write report","status":"in_progress"}},
			{"op":"invented","path":"/anything/deep/0/name","value":"allowed"},
			{"op":"remove","path":"/missing/path/does/not/exist"},
			{"path":"path without leading slash","value":"allowed too"},
			{"op":"replace","path":"/freeform_known_field","value":["not","schema","checked"]}
		]
	}`)

	next, err := ApplyHandoffPatch(state, raw)
	if err != nil {
		t.Fatalf("ApplyHandoffPatch: %v", err)
	}

	identified, ok := next.Files.Extra["identified"].([]any)
	if !ok || len(identified) != 1 || identified[0] != "internal/query/loop.go" {
		t.Fatalf("Files.Extra[identified] = %#v", next.Files.Extra["identified"])
	}
	scratch, ok := next.Extra["scratch"].(map[string]any)
	if !ok {
		t.Fatalf("Extra[scratch] = %#v", next.Extra["scratch"])
	}
	findings, ok := scratch["findings"].([]any)
	if !ok || len(findings) != 1 {
		t.Fatalf("scratch.findings = %#v", scratch["findings"])
	}
	if len(next.Todos) != 1 || next.Todos[0].ID != "report" {
		t.Fatalf("Todos = %#v", next.Todos)
	}
	anything := next.Extra["anything"].(map[string]any)
	deep := anything["deep"].([]any)
	if deep[0].(map[string]any)["name"] != "allowed" {
		t.Fatalf("anything.deep = %#v", deep)
	}
	if next.Extra["path without leading slash"] != "allowed too" {
		t.Fatalf("path without leading slash = %#v", next.Extra["path without leading slash"])
	}

	data, err := json.Marshal(next)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(data)
	for _, want := range []string{`"identified"`, `"scratch"`, `"findings"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("marshaled handoff state missing %s: %s", want, got)
		}
	}
}

func TestApplyHandoffPatchPreservesArbitraryKnownFieldValues(t *testing.T) {
	state := NewHandoffState("inspect repo")
	raw := json.RawMessage(`{
		"ops": [
			{"op":"replace","path":"/todos","value":"freeform todo state"},
			{"op":"replace","path":"/files/read","value":{"notes":"not an array"}}
		]
	}`)

	next, err := ApplyHandoffPatch(state, raw)
	if err != nil {
		t.Fatalf("ApplyHandoffPatch: %v", err)
	}
	if next.Extra["todos"] != "freeform todo state" {
		t.Fatalf("Extra[todos] = %#v", next.Extra["todos"])
	}
	filesData, err := json.Marshal(next.Files)
	if err != nil {
		t.Fatalf("Marshal files: %v", err)
	}
	if !strings.Contains(string(filesData), `"notes"`) {
		t.Fatalf("files JSON did not preserve arbitrary read value: %s", filesData)
	}

	data, err := json.Marshal(next)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"todos":"freeform todo state"`) || !strings.Contains(string(data), `"notes"`) {
		t.Fatalf("state JSON did not preserve arbitrary known values: %s", data)
	}
}

func TestHandoffStateDeepCopyCopiesTodos(t *testing.T) {
	state := NewHandoffState("finish long task")
	state.Todos = []HandoffTodo{{ID: "a", Task: "first", Status: "pending"}}
	state.Extra = map[string]any{"scratch": map[string]any{"count": float64(1)}}
	state.Files.Extra = map[string]any{"identified": []any{"a.go"}}

	cp := state.DeepCopy()
	cp.Todos[0].Status = "completed"
	cp.Extra["scratch"].(map[string]any)["count"] = float64(2)
	cp.Files.Extra["identified"].([]any)[0] = "b.go"

	if state.Todos[0].Status != "pending" {
		t.Fatalf("DeepCopy shared todo backing array: %#v", state.Todos)
	}
	if state.Extra["scratch"].(map[string]any)["count"] != float64(1) {
		t.Fatalf("DeepCopy shared Extra: %#v", state.Extra)
	}
	if state.Files.Extra["identified"].([]any)[0] != "a.go" {
		t.Fatalf("DeepCopy shared Files.Extra: %#v", state.Files.Extra)
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

func TestApplyHandoffPatchOnlyRejectsMalformedPatchEnvelope(t *testing.T) {
	_, err := ApplyHandoffPatch(NewHandoffState("inspect repo"), json.RawMessage(`{"ops":`))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parse handoff patch") {
		t.Fatalf("error = %v", err)
	}
}
