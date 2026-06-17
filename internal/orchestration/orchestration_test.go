package orchestration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/persona"
	"github.com/artpar/pragma/internal/query"

	"gopkg.in/yaml.v3"
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
		EventComplete,
		"block",
		EventComplete,
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
	def, err := LoadDefinitionFile(filepath.Join("..", "..", "examples", "orchestrations", "architect-checklist-item-loop-final.yaml"))
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
		EventComplete,
		"item_block",
		EventComplete,
		EventComplete,
		"item_approve",
		EventComplete,
		"all_items_done",
		EventComplete,
		"final_block",
		EventComplete,
		"all_items_done",
		EventComplete,
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

func TestLoadTaskEvidenceItemLoopYAML(t *testing.T) {
	def, err := LoadDefinitionFile(filepath.Join("..", "..", "orchestrations", "task-evidence-item-loop.yaml"))
	if err != nil {
		t.Fatalf("LoadDefinitionFile: %v", err)
	}
	if _, err := NewRuntime(def); err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
}

func TestLoadEveryActiveOrchestrationYAML(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "orchestrations", "*.yaml"))
	if err != nil {
		t.Fatalf("glob orchestration definitions: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("expected active orchestration definitions")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			def, err := LoadDefinitionFile(path)
			if err != nil {
				t.Fatalf("LoadDefinitionFile: %v", err)
			}
			if _, err := NewRuntime(def); err != nil {
				t.Fatalf("NewRuntime: %v", err)
			}
		})
	}
}

func TestTransitionHandoffUnmarshalAndValidation(t *testing.T) {
	var def Definition
	raw := []byte(`
name: handoff-test
initial: first
states:
  - id: first
  - id: second
  - id: done
    terminal: true
transitions:
  - event: complete
    from: [first]
    to: second
    handoff:
      - id: first_report
        path: report.md
        required: true
        allowed_values: [APPROVE]
  - event: complete
    from: [second]
    to: done
`)
	if err := yaml.Unmarshal(raw, &def); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	runtime, err := NewRuntime(def)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	tr, ok := runtime.TransitionFor("first", EventComplete)
	if !ok {
		t.Fatal("expected transition lookup")
	}
	if len(tr.Handoff) != 1 || tr.Handoff[0].ID != "first_report" || !tr.Handoff[0].Required {
		t.Fatalf("handoff = %#v", tr.Handoff)
	}

	def.Transitions[0].Handoff[0].AllowedValues = []string{" "}
	if _, err := NewRuntime(def); err == nil {
		t.Fatal("expected empty allowed transition handoff value to be rejected")
	}
}

func TestNewRuntimeRejectsDuplicateTransitionKey(t *testing.T) {
	_, err := NewRuntime(Definition{
		Name:    "bad",
		Initial: "source",
		States: []State{
			{ID: "source"},
			{ID: "one"},
			{ID: "two"},
		},
		Transitions: []Transition{
			{Event: EventComplete, From: []string{"source"}, To: "one"},
			{Event: EventComplete, From: []string{"source"}, To: "two"},
		},
	})
	if err == nil {
		t.Fatal("expected duplicate transition key to be rejected")
	}
}

func TestTransitionLookupUsesSourceStateAndEvent(t *testing.T) {
	runtime, err := NewRuntime(Definition{
		Name:    "lookup",
		Initial: "a",
		States: []State{
			{ID: "a", Event: Event{Default: "go"}},
			{ID: "b", Event: Event{Default: "go"}},
			{ID: "shared"},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{
			{Event: "go", From: []string{"a"}, To: "shared", Handoff: []Artifact{{ID: "from_a", Path: "a.md"}}},
			{Event: "go", From: []string{"b"}, To: "shared", Handoff: []Artifact{{ID: "from_b", Path: "b.md"}}},
			{Event: EventComplete, From: []string{"shared"}, To: "done"},
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	tr, ok := runtime.TransitionFor("a", "go")
	if !ok || len(tr.Handoff) != 1 || tr.Handoff[0].ID != "from_a" {
		t.Fatalf("lookup a/go = %#v, %v", tr, ok)
	}
	tr, ok = runtime.TransitionFor("b", "go")
	if !ok || len(tr.Handoff) != 1 || tr.Handoff[0].ID != "from_b" {
		t.Fatalf("lookup b/go = %#v, %v", tr, ok)
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

func TestNewRuntimeRejectsInvalidArtifacts(t *testing.T) {
	_, err := NewRuntime(Definition{
		Name:    "bad",
		Initial: "worker",
		States: []State{
			{
				ID: "worker",
				Artifacts: Artifacts{
					Inputs: []Artifact{{ID: "current_item"}},
				},
			},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{
			{Event: EventComplete, From: []string{"worker"}, To: "done"},
		},
	})
	if err == nil {
		t.Fatal("expected artifact without path to be rejected")
	}
}

func TestRequiredTransitionHandoffFailsBeforeModelCall(t *testing.T) {
	dir := t.TempDir()
	personaDir := filepath.Join(dir, "personas")
	if err := os.MkdirAll(personaDir, 0o755); err != nil {
		t.Fatalf("mkdir persona dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(personaDir, "worker.yaml"), []byte("id: worker\nprompt: Worker prompt\n"), 0o600); err != nil {
		t.Fatalf("write persona: %v", err)
	}
	ch := make(chan query.LoopEvent, 8)
	state := State{ID: "worker", Persona: "worker"}
	_, _, err := runNodeEvents(context.Background(), ch, nil, NewProjection(), personaDir, Definition{Name: "test"}, state, "", "", "", []Artifact{
		{ID: "missing", Path: "missing.md", Required: true},
	}, "source", EventComplete, dir)
	if err == nil {
		t.Fatal("expected missing required handoff to fail")
	}
	if !strings.Contains(err.Error(), "read transition handoff artifact") {
		t.Fatalf("error = %v", err)
	}
}

func TestOptionalTransitionHandoffMissingIsOmitted(t *testing.T) {
	handoff, reads, err := RenderTransitionHandoff(State{ID: "worker"}, []Artifact{
		{ID: "optional_context", Path: "does-not-exist.md"},
	}, t.TempDir(), true)
	if err != nil {
		t.Fatalf("RenderTransitionHandoff: %v", err)
	}
	if handoff != "" {
		t.Fatalf("handoff = %q, want empty", handoff)
	}
	if len(reads) != 0 {
		t.Fatalf("reads = %#v, want none", reads)
	}
}

func TestPromptDoesNotRenderDestinationStateInputs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stale.md"), []byte("STALE_DESTINATION_INPUT"), 0o600); err != nil {
		t.Fatalf("write stale artifact: %v", err)
	}
	_, prompt, err := BuildPromptWithArtifactRootChecked(
		Definition{Name: "test"},
		State{
			ID: "worker",
			Artifacts: Artifacts{Inputs: []Artifact{{
				ID:       "stale",
				Path:     "stale.md",
				Required: true,
			}}},
		},
		testPersona("worker"),
		"",
		"",
		dir,
	)
	if err != nil {
		t.Fatalf("BuildPromptWithArtifactRootChecked: %v", err)
	}
	if strings.Contains(prompt, "STALE_DESTINATION_INPUT") || strings.Contains(prompt, "## Handoff From Previous Phase") {
		t.Fatalf("prompt rendered destination input handoff:\n%s", prompt)
	}
}

func TestStateMaxTurnsValidationAndPromptContract(t *testing.T) {
	def := Definition{
		Name:    "budgeted",
		Initial: "worker",
		States: []State{
			{ID: "worker", Persona: "worker", MaxTurns: 7},
			{ID: "done", Terminal: true},
		},
		Transitions: []Transition{{Event: EventComplete, From: []string{"worker"}, To: "done"}},
	}
	if _, err := NewRuntime(def); err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	system, _, err := BuildPromptWithArtifactRootChecked(def, def.States[0], testPersona("worker"), "", "", t.TempDir())
	if err != nil {
		t.Fatalf("BuildPromptWithArtifactRootChecked: %v", err)
	}
	if len(system.Blocks) == 0 || !strings.Contains(system.Blocks[0].Text, "runtime budget of 7 shell actions") {
		t.Fatalf("system prompt missing max_turns contract:\n%+v", system.Blocks)
	}

	bad := def
	bad.States = []State{{ID: "worker", MaxTurns: -1}, {ID: "done", Terminal: true}}
	if _, err := NewRuntime(bad); err == nil || !strings.Contains(err.Error(), "invalid max_turns -1") {
		t.Fatalf("NewRuntime negative max_turns err = %v", err)
	}
}

func TestRequiredOutputCompletionCheckStillEnforcesOutputs(t *testing.T) {
	dir := t.TempDir()
	state := State{
		ID: "worker",
		Artifacts: Artifacts{Outputs: []Artifact{{
			ID:       "report",
			Path:     "report.md",
			Required: true,
		}}},
	}
	check := requiredOutputArtifactCompletionCheck(state, dir)
	if check == nil {
		t.Fatal("expected completion check")
	}
	ok, guidance, err := check()
	if err != nil {
		t.Fatalf("completion check: %v", err)
	}
	if ok || !strings.Contains(guidance, "report.md") {
		t.Fatalf("ok=%v guidance=%q, want missing report guidance", ok, guidance)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte("done"), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}
	ok, guidance, err = check()
	if err != nil {
		t.Fatalf("completion check after write: %v", err)
	}
	if !ok || guidance != "" {
		t.Fatalf("ok=%v guidance=%q, want success", ok, guidance)
	}
}

func TestRequiredOutputCompletionCheckRejectsWeakAcceptanceMap(t *testing.T) {
	dir := t.TempDir()
	state := State{
		ID: "acceptance_mapper",
		Artifacts: Artifacts{Outputs: []Artifact{{
			ID:       "acceptance_map",
			Path:     "acceptance-map.json",
			Required: true,
			Checks:   acceptanceMapIntegrityChecks(),
		}}},
	}
	content := `{
  "version": 1,
  "source": "task_prompt",
  "acceptance_items": [
    {
      "id": "ACCEPT-THING",
      "task_text": "The feature works.",
      "source_quote": "The feature works",
      "behavior_surface": "config",
      "repo_surfaces_to_verify": ["/workspace/src/feature-config"],
      "required_validation": ["unknown", "command: behavior-check /workspace/src/feature-config"],
      "status": "pending",
      "validation_evidence": [],
      "blocking_if_missing": true,
      "notes": "bad validation"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "acceptance-map.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("write acceptance map: %v", err)
	}

	check := requiredOutputArtifactCompletionCheck(state, dir, "The feature works.")
	ok, guidance, err := check()
	if err != nil {
		t.Fatalf("completion check: %v", err)
	}
	if ok {
		t.Fatal("weak acceptance map passed")
	}
	for _, want := range []string{"non-repo-relative path", "placeholder validation", "absolute repository path"} {
		if !strings.Contains(guidance, want) {
			t.Fatalf("guidance missing %q:\n%s", want, guidance)
		}
	}
}

func TestRequiredOutputCompletionCheckAcceptsBehaviorAcceptanceMap(t *testing.T) {
	dir := t.TempDir()
	state := State{
		ID: "acceptance_mapper",
		Artifacts: Artifacts{Outputs: []Artifact{{
			ID:       "acceptance_map",
			Path:     "acceptance-map.json",
			Required: true,
			Checks:   acceptanceMapIntegrityChecks(),
		}}},
	}
	content := `{
  "version": 1,
  "source": "task_prompt",
  "acceptance_items": [
    {
      "id": "ACCEPT-THING",
      "task_text": "The feature works.",
      "source_quote": "The feature works",
      "behavior_surface": "config",
      "repo_surfaces_to_verify": ["src/feature-config"],
      "required_validation": ["run behavior-check feature-config"],
      "status": "pending",
      "validation_evidence": [],
      "blocking_if_missing": true,
      "notes": "behavior validation"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "acceptance-map.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("write acceptance map: %v", err)
	}

	check := requiredOutputArtifactCompletionCheck(state, dir, "The feature works.")
	ok, guidance, err := check()
	if err != nil {
		t.Fatalf("completion check: %v", err)
	}
	if !ok || guidance != "" {
		t.Fatalf("ok=%v guidance=%q, want success", ok, guidance)
	}
}

func TestRequiredOutputCompletionCheckRejectsCorruptedSourceQuote(t *testing.T) {
	dir := t.TempDir()
	state := State{
		ID: "acceptance_auditor",
		Artifacts: Artifacts{Outputs: []Artifact{{
			ID:       "acceptance_map",
			Path:     "acceptance-map.json",
			Required: true,
			Checks:   acceptanceMapIntegrityChecks(),
		}}},
	}
	content := `{
  "version": 1,
  "source": "task_prompt",
  "acceptance_items": [
    {
      "id": "ACCEPT-FRAMEWORK",
      "task_text": "The requested feature integrates with the existing framework.",
      "source_quote": "The requested feature integrates with the wrong framework",
      "behavior_surface": "runtime",
      "repo_surfaces_to_verify": ["src/feature-entrypoint"],
      "required_validation": ["unknown"],
      "status": "pending",
      "validation_evidence": [],
      "blocking_if_missing": true,
      "notes": "typo in source quote"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "acceptance-map.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("write acceptance map: %v", err)
	}

	check := requiredOutputArtifactCompletionCheck(state, dir, "The requested feature integrates with the existing framework")
	ok, guidance, err := check()
	if err != nil {
		t.Fatalf("completion check: %v", err)
	}
	if ok || !strings.Contains(guidance, "source_quote is not an exact substring") {
		t.Fatalf("ok=%v guidance=%q, want corrupted source quote rejection", ok, guidance)
	}
}

func TestTaskEvidenceItemLoopTransitionHandoffBranchPrompts(t *testing.T) {
	def, err := LoadDefinitionFile(filepath.Join("..", "..", "orchestrations", "task-evidence-item-loop.yaml"))
	if err != nil {
		t.Fatalf("LoadDefinitionFile: %v", err)
	}
	def = relativizeDefinitionArtifactPaths(def)
	runtime, err := NewRuntime(def)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	dir := t.TempDir()
	writeText(t, dir, "current-item.json", `{"id":"item-001","status":"pending"}`)
	writeText(t, dir, "handoff-prompts/next_item/current-item.md", "CURRENT_ITEM_HANDOFF")
	writeText(t, dir, "implementer-report.md", "IMPLEMENTER_REPORT")
	writeText(t, dir, "item-verdict.md", "ITEM_VERDICT")
	writeText(t, dir, "item-block-classification.json", `{"decision":"redo_item_worker","marker":"ITEM_BLOCK_CLASSIFICATION"}`)
	writeText(t, dir, "checklist.json", `{"items":[{"id":"item-001","status":"completed"}]}`)
	writeText(t, dir, "patch-plan.md", "PATCH_PLAN")

	firstWorker := branchPrompt(t, runtime, "next_item", "item_available", "item_worker", dir)
	requireContains(t, firstWorker, "current_item")
	requireContains(t, firstWorker, "CURRENT_ITEM_HANDOFF")
	requireNotContains(t, firstWorker, "ITEM_BLOCK_CLASSIFICATION")

	retryWorker := branchPrompt(t, runtime, "route_item_block_classification", "redo_item_worker", "item_worker", dir)
	requireContains(t, retryWorker, "ITEM_BLOCK_CLASSIFICATION")
	requireContains(t, retryWorker, "ITEM_VERDICT")
	requireContains(t, retryWorker, "IMPLEMENTER_REPORT")

	normalChecklist := branchPrompt(t, runtime, "mark_item_completed", EventComplete, "checklist_writer", dir)
	requireContains(t, normalChecklist, "PATCH_PLAN")
	requireNotContains(t, normalChecklist, "ITEM_BLOCK_CLASSIFICATION")

	repairChecklist := branchPrompt(t, runtime, "route_item_block_classification", "repair_checklist_scope", "checklist_writer", dir)
	requireContains(t, repairChecklist, "ITEM_BLOCK_CLASSIFICATION")
	requireContains(t, repairChecklist, "ITEM_VERDICT")
	requireContains(t, repairChecklist, "PATCH_PLAN")
}

func TestExecuteForEachNextWritesFirstPendingItem(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "checklist.json")
	cursorPath := filepath.Join(dir, "current-item.json")
	writeTestChecklist(t, listPath, Checklist{
		Items: []ChecklistItem{
			{ID: "item-001", Status: "approved"},
			{ID: "item-002", Status: "pending"},
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
			{ID: "item-001", Status: "approved"},
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
			{ID: "item-001", Status: "pending"},
			{ID: "item-002", Status: "pending"},
		},
	})
	writeTestJSON(t, cursorPath, ChecklistItem{ID: "item-002", Status: "pending"})

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

func TestExecuteArtifactVerdictEmitsDecisionEvent(t *testing.T) {
	dir := t.TempDir()
	verdictPath := filepath.Join(dir, "verdict.md")
	if err := os.WriteFile(verdictPath, []byte("Findings:\n- ok\n\nDecision:\nAPPROVE\n"), 0o600); err != nil {
		t.Fatalf("write verdict: %v", err)
	}

	event, err := ExecuteControl(State{
		ID: "route_verdict",
		Control: Control{
			ArtifactVerdict: &ArtifactVerdictControl{
				Path:         verdictPath,
				ApproveEvent: "approve",
				BlockEvent:   "block",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteControl: %v", err)
	}
	if event != "approve" {
		t.Fatalf("event = %q, want approve", event)
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

func testPersona(id string) persona.Definition {
	return persona.Definition{ID: id, Prompt: "Persona prompt"}
}

func acceptanceMapIntegrityChecks() []ArtifactIntegrityCheck {
	return []ArtifactIntegrityCheck{
		{Type: "json_field_equals", Field: "source", Value: "task_prompt"},
		{Type: "json_array_non_empty", Path: "acceptance_items"},
		{Type: "json_each_required_fields", Path: "acceptance_items", Fields: []string{"id", "source_quote"}},
		{Type: "json_each_string_substring_of_task_prompt", Path: "acceptance_items", Field: "source_quote"},
		{Type: "json_each_repo_relative_paths", Path: "acceptance_items", Field: "repo_surfaces_to_verify"},
		{Type: "json_each_behavior_validation_commands", Path: "acceptance_items", Field: "required_validation"},
	}
}

func writeText(t *testing.T, root string, rel string, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func relativizeDefinitionArtifactPaths(def Definition) Definition {
	rel := func(path string) string {
		return strings.TrimPrefix(path, "/tmp/pragma/")
	}
	for stateIdx := range def.States {
		for inputIdx := range def.States[stateIdx].Artifacts.Inputs {
			def.States[stateIdx].Artifacts.Inputs[inputIdx].Path = rel(def.States[stateIdx].Artifacts.Inputs[inputIdx].Path)
		}
		for outputIdx := range def.States[stateIdx].Artifacts.Outputs {
			def.States[stateIdx].Artifacts.Outputs[outputIdx].Path = rel(def.States[stateIdx].Artifacts.Outputs[outputIdx].Path)
		}
		if control := def.States[stateIdx].Control.ForEachNext; control != nil {
			control.ListPath = rel(control.ListPath)
			control.CursorPath = rel(control.CursorPath)
			control.HandoffPath = rel(control.HandoffPath)
		}
		if control := def.States[stateIdx].Control.MarkCurrentItem; control != nil {
			control.ListPath = rel(control.ListPath)
			control.CursorPath = rel(control.CursorPath)
		}
		if control := def.States[stateIdx].Control.ArtifactVerdict; control != nil {
			control.Path = rel(control.Path)
		}
		if control := def.States[stateIdx].Control.ArtifactDecision; control != nil {
			control.Path = rel(control.Path)
		}
	}
	for transitionIdx := range def.Transitions {
		for handoffIdx := range def.Transitions[transitionIdx].Handoff {
			def.Transitions[transitionIdx].Handoff[handoffIdx].Path = rel(def.Transitions[transitionIdx].Handoff[handoffIdx].Path)
		}
	}
	return def
}

func branchPrompt(t *testing.T, runtime *Runtime, from string, event string, to string, artifactRoot string) string {
	t.Helper()
	tr, ok := runtime.TransitionFor(from, event)
	if !ok {
		t.Fatalf("missing transition %s --%s", from, event)
	}
	if tr.To != to {
		t.Fatalf("transition %s --%s--> %s, want %s", from, event, tr.To, to)
	}
	state, ok := runtime.States[to]
	if !ok {
		t.Fatalf("missing state %s", to)
	}
	handoff, _, err := RenderTransitionHandoff(state, tr.Handoff, artifactRoot, true)
	if err != nil {
		t.Fatalf("RenderTransitionHandoff %s --%s--> %s: %v", from, event, to, err)
	}
	_, prompt, err := BuildPromptWithArtifactRootChecked(runtime.Definition, state, testPersona(firstNonEmpty(state.Persona, state.ID)), "", handoff, artifactRoot)
	if err != nil {
		t.Fatalf("BuildPromptWithArtifactRootChecked %s: %v", to, err)
	}
	return prompt
}

func requireContains(t *testing.T, text string, needle string) {
	t.Helper()
	if !strings.Contains(text, needle) {
		t.Fatalf("expected prompt to contain %q:\n%s", needle, text)
	}
}

func requireNotContains(t *testing.T, text string, needle string) {
	t.Helper()
	if strings.Contains(text, needle) {
		t.Fatalf("expected prompt not to contain %q:\n%s", needle, text)
	}
}
