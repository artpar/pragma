package orchestration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewRuntimeUsesLooplabFSM(t *testing.T) {
	runtime, err := NewRuntime(Definition{
		Name:    "planner-executor",
		Initial: "planner",
		States: []State{
			{ID: "planner"},
			{ID: "implementer"},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{
			{Event: EventComplete, From: []string{"planner"}, To: "implementer"},
			{Event: EventComplete, From: []string{"implementer"}, To: "done"},
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	if got := runtime.FSM.Current(); got != "planner" {
		t.Fatalf("initial state = %q, want planner", got)
	}
	if !runtime.FSM.Can(EventComplete) {
		t.Fatalf("expected looplab FSM to allow %q", EventComplete)
	}
	if err := runtime.FSM.Event(context.Background(), EventComplete); err != nil {
		t.Fatalf("first transition: %v", err)
	}
	if got := runtime.FSM.Current(); got != "implementer" {
		t.Fatalf("state after first transition = %q, want implementer", got)
	}
	if err := runtime.FSM.Event(context.Background(), EventComplete); err != nil {
		t.Fatalf("second transition: %v", err)
	}
	if got := runtime.FSM.Current(); got != "done" {
		t.Fatalf("state after second transition = %q, want done", got)
	}
}

func TestNewRuntimeRejectsTerminalTransitionSource(t *testing.T) {
	_, err := NewRuntime(Definition{
		Name:    "bad",
		Initial: "done",
		States: []State{
			{ID: "done", Terminal: true},
			{ID: "solver"},
		},
		Transitions: []Transition{
			{Event: EventComplete, From: []string{"done"}, To: "solver"},
		},
	})
	if err == nil {
		t.Fatal("expected terminal transition source to be rejected")
	}
}

func TestLoadArchitectImplementerProsecutorYAML(t *testing.T) {
	def, err := LoadDefinitionFile(filepath.Join("..", "..", "orchestrations", "architect-implementer-prosecutor.yaml"))
	if err != nil {
		t.Fatalf("LoadDefinitionFile: %v", err)
	}
	runtime, err := NewRuntime(def)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	for _, event := range []string{
		EventComplete,
		EventComplete,
		"block",
		EventComplete,
		"approve",
	} {
		if err := runtime.FSM.Event(context.Background(), event); err != nil {
			t.Fatalf("event %q from %q: %v", event, runtime.FSM.Current(), err)
		}
	}
	if got := runtime.FSM.Current(); got != "done" {
		t.Fatalf("final state = %q, want done", got)
	}
}

func TestLoadChecklistLoopYAML(t *testing.T) {
	def, err := LoadDefinitionFile(filepath.Join("..", "..", "orchestrations", "architect-checklist-item-loop-final.yaml"))
	if err != nil {
		t.Fatalf("LoadDefinitionFile: %v", err)
	}
	runtime, err := NewRuntime(def)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	for _, event := range []string{
		EventComplete,
		EventComplete,
		EventComplete,
		"item_available",
		EventComplete,
		"item_block",
		EventComplete,
		"item_approve",
		EventComplete,
		"all_items_done",
		"final_block",
		EventComplete,
		"all_items_done",
		"final_approve",
	} {
		if err := runtime.FSM.Event(context.Background(), event); err != nil {
			t.Fatalf("event %q from %q: %v", event, runtime.FSM.Current(), err)
		}
	}
	if got := runtime.FSM.Current(); got != "done" {
		t.Fatalf("final state = %q, want done", got)
	}
}

func TestNewRuntimeRejectsPersonaControlState(t *testing.T) {
	_, err := NewRuntime(Definition{
		Name:    "bad",
		Initial: "worker",
		States: []State{
			{
				ID:      "worker",
				Persona: "implementer",
				Control: Control{
					ForEachNext: &ForEachNextControl{
						ListPath:   "/tmp/list.json",
						CursorPath: "/tmp/current.json",
						ItemEvent:  "item",
						DoneEvent:  "done",
					},
				},
			},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{
			{Event: "item", From: []string{"worker"}, To: "done"},
			{Event: "done", From: []string{"worker"}, To: "done"},
		},
	})
	if err == nil {
		t.Fatal("expected persona/control state to be rejected")
	}
}

func TestNewRuntimeRejectsInvalidTaskPromptMode(t *testing.T) {
	_, err := NewRuntime(Definition{
		Name:    "bad",
		Initial: "worker",
		States: []State{
			{ID: "worker", TaskPrompt: "summary"},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{
			{Event: EventComplete, From: []string{"worker"}, To: "done"},
		},
	})
	if err == nil {
		t.Fatal("expected invalid task_prompt mode to be rejected")
	}
}

func TestNewRuntimeRejectsTaskPromptOnControlState(t *testing.T) {
	_, err := NewRuntime(Definition{
		Name:    "bad",
		Initial: "next_item",
		States: []State{
			{
				ID:         "next_item",
				TaskPrompt: TaskPromptNone,
				Control: Control{
					ForEachNext: &ForEachNextControl{
						ListPath:   "/tmp/list.json",
						CursorPath: "/tmp/current.json",
						ItemEvent:  "item",
						DoneEvent:  "done",
					},
				},
			},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{
			{Event: "item", From: []string{"next_item"}, To: "done"},
			{Event: "done", From: []string{"next_item"}, To: "done"},
		},
	})
	if err == nil {
		t.Fatal("expected task_prompt on control state to be rejected")
	}
}

func TestExecuteForEachNextWritesFirstPendingItem(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "checklist.json")
	cursorPath := filepath.Join(dir, "current-item.json")
	writeTestChecklist(t, listPath, Checklist{
		Items: []ChecklistItem{
			{ID: "item-001", Title: "done", Status: "approved"},
			{ID: "item-002", Title: "pending", Status: "pending", Acceptance: []string{"prove it"}},
		},
	})

	event, err := ExecuteControl(State{
		ID: "next_item",
		Control: Control{
			ForEachNext: &ForEachNextControl{
				ListPath:   listPath,
				CursorPath: cursorPath,
				ItemEvent:  "item_available",
				DoneEvent:  "all_items_done",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteControl: %v", err)
	}
	if event != "item_available" {
		t.Fatalf("event = %q, want item_available", event)
	}

	var current ChecklistItem
	readTestJSON(t, cursorPath, &current)
	if current.ID != "item-002" {
		t.Fatalf("current item id = %q, want item-002", current.ID)
	}
}

func TestExecuteForEachNextPreservesUnknownItemFields(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "checklist.json")
	cursorPath := filepath.Join(dir, "current-item.json")
	raw := []byte(`{
  "items": [
    {
      "id": "item-001",
      "title": "pending",
      "status": "pending",
      "allowed_files": ["server/auth.go"],
      "forbidden_files": ["generated/**"],
      "validation_command": "go test ./server",
      "producer_evidence": {"kind": "patch-plan"}
    }
  ]
}`)
	if err := os.WriteFile(listPath, raw, 0o600); err != nil {
		t.Fatalf("write checklist: %v", err)
	}

	event, err := ExecuteControl(State{
		ID: "next_item",
		Control: Control{
			ForEachNext: &ForEachNextControl{
				ListPath:   listPath,
				CursorPath: cursorPath,
				ItemEvent:  "item_available",
				DoneEvent:  "all_items_done",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteControl: %v", err)
	}
	if event != "item_available" {
		t.Fatalf("event = %q, want item_available", event)
	}

	var current map[string]any
	readTestJSON(t, cursorPath, &current)
	if _, ok := current["allowed_files"]; !ok {
		t.Fatalf("current item lost allowed_files: %#v", current)
	}
	if _, ok := current["forbidden_files"]; !ok {
		t.Fatalf("current item lost forbidden_files: %#v", current)
	}
	if _, ok := current["validation_command"]; !ok {
		t.Fatalf("current item lost validation_command: %#v", current)
	}
	if _, ok := current["producer_evidence"]; !ok {
		t.Fatalf("current item lost producer_evidence: %#v", current)
	}
}

func TestExecuteForEachNextEmitsDoneWhenNoPendingItems(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "checklist.json")
	cursorPath := filepath.Join(dir, "current-item.json")
	writeTestChecklist(t, listPath, Checklist{
		Items: []ChecklistItem{
			{ID: "item-001", Title: "done", Status: "approved"},
		},
	})

	event, err := ExecuteControl(State{
		ID: "next_item",
		Control: Control{
			ForEachNext: &ForEachNextControl{
				ListPath:   listPath,
				CursorPath: cursorPath,
				ItemEvent:  "item_available",
				DoneEvent:  "all_items_done",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteControl: %v", err)
	}
	if event != "all_items_done" {
		t.Fatalf("event = %q, want all_items_done", event)
	}
}

func TestExecuteMarkCurrentItemUpdatesChecklist(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "checklist.json")
	cursorPath := filepath.Join(dir, "current-item.json")
	writeTestChecklist(t, listPath, Checklist{
		Items: []ChecklistItem{
			{ID: "item-001", Title: "one", Status: "pending"},
			{ID: "item-002", Title: "two", Status: "pending"},
		},
	})
	writeTestJSON(t, cursorPath, ChecklistItem{ID: "item-002", Title: "two", Status: "pending"})

	event, err := ExecuteControl(State{
		ID: "mark_item_approved",
		Control: Control{
			MarkCurrentItem: &MarkCurrentItemControl{
				ListPath:   listPath,
				CursorPath: cursorPath,
				Status:     "approved",
				Event:      EventComplete,
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteControl: %v", err)
	}
	if event != EventComplete {
		t.Fatalf("event = %q, want complete", event)
	}

	var checklist Checklist
	readTestJSON(t, listPath, &checklist)
	if checklist.Items[0].Status != "pending" || checklist.Items[1].Status != "approved" {
		t.Fatalf("statuses = %q, %q; want pending, approved", checklist.Items[0].Status, checklist.Items[1].Status)
	}
}

func TestExecuteMarkCurrentItemPreservesUnknownChecklistFields(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "checklist.json")
	cursorPath := filepath.Join(dir, "current-item.json")
	raw := []byte(`{
  "items": [
    {
      "id": "item-001",
      "status": "pending",
      "allowed_files": ["server/auth.go"],
      "validation_command": "go test ./server"
    },
    {
      "id": "item-002",
      "status": "pending",
      "allowed_files": ["client/auth.go"],
      "producer_evidence": {"kind": "generated"}
    }
  ]
}`)
	if err := os.WriteFile(listPath, raw, 0o600); err != nil {
		t.Fatalf("write checklist: %v", err)
	}
	if err := os.WriteFile(cursorPath, []byte(`{"id":"item-002","status":"pending","allowed_files":["client/auth.go"],"producer_evidence":{"kind":"generated"}}`), 0o600); err != nil {
		t.Fatalf("write current item: %v", err)
	}

	_, err := ExecuteControl(State{
		ID: "mark_item_approved",
		Control: Control{
			MarkCurrentItem: &MarkCurrentItemControl{
				ListPath:   listPath,
				CursorPath: cursorPath,
				Status:     "approved",
				Event:      EventComplete,
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteControl: %v", err)
	}

	var checklist map[string][]map[string]any
	readTestJSON(t, listPath, &checklist)
	if checklist["items"][0]["allowed_files"] == nil || checklist["items"][0]["validation_command"] == nil {
		t.Fatalf("first item metadata was stripped: %#v", checklist["items"][0])
	}
	if checklist["items"][1]["allowed_files"] == nil || checklist["items"][1]["producer_evidence"] == nil {
		t.Fatalf("second item metadata was stripped: %#v", checklist["items"][1])
	}
	if checklist["items"][1]["status"] != "approved" {
		t.Fatalf("second item status = %v, want approved", checklist["items"][1]["status"])
	}
}

func writeTestChecklist(t *testing.T, path string, checklist Checklist) {
	t.Helper()
	writeTestJSON(t, path, checklist)
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(raw, value); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
}
