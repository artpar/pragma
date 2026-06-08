package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/orchestration"
	"github.com/artpar/pragma/internal/persona"
)

func TestSelectStateEventDefaultsToComplete(t *testing.T) {
	event, err := selectStateEvent(orchestration.State{ID: "worker"})
	if err != nil {
		t.Fatalf("selectStateEvent: %v", err)
	}
	if event != orchestration.EventComplete {
		t.Fatalf("event = %q, want %q", event, orchestration.EventComplete)
	}
}

func TestBuildOrchestrationPromptCanOmitTask(t *testing.T) {
	def := orchestration.Definition{
		Name:    "test",
		Initial: "surface_mapper",
		States: []orchestration.State{
			{ID: "surface_mapper"},
			{ID: "item_worker", TaskPrompt: orchestration.TaskPromptNone},
		},
	}
	system, prompt := buildOrchestrationPrompt(def, orchestration.State{
		ID:         "item_worker",
		TaskPrompt: orchestration.TaskPromptNone,
	}, persona.Definition{
		ID:     "item_worker",
		Prompt: "Read current-item.json.",
	}, "full benchmark task", "Use checklist item 1.")

	if strings.Contains(prompt, "full benchmark task") {
		t.Fatalf("prompt included task despite task_prompt none: %q", prompt)
	}
	if !strings.Contains(prompt, "Use checklist item 1.") {
		t.Fatalf("prompt lost handoff: %q", prompt)
	}
	if strings.Contains(prompt, "Read current-item.json.") {
		t.Fatalf("prompt included persona text in user message: %q", prompt)
	}
	if len(system.Blocks) != 1 || !strings.Contains(system.Blocks[0].Text, "Read current-item.json.") {
		t.Fatalf("system prompt lost persona text: %#v", system.Blocks)
	}
}

func TestBuildOrchestrationPromptSplitsPersonaSystemAndTaskUser(t *testing.T) {
	def := orchestration.Definition{
		Name:    "test",
		Initial: "surface_mapper",
		States: []orchestration.State{
			{ID: "surface_mapper"},
			{ID: "evidence_mapper"},
		},
		Transitions: []orchestration.Transition{
			{Event: orchestration.EventComplete, From: []string{"surface_mapper"}, To: "evidence_mapper"},
		},
	}
	system, prompt := buildOrchestrationPrompt(def, orchestration.State{
		ID: "surface_mapper",
	}, persona.Definition{
		ID:     "surface_mapper",
		Prompt: "You are the surface mapper.",
	}, "Feature request body", "")

	if len(system.Blocks) != 1 || !strings.HasPrefix(system.Blocks[0].Text, "You are the surface mapper.\n\nPragma loop mode is a shell-action transport.") {
		t.Fatalf("system prompt = %#v", system.Blocks)
	}
	if !strings.Contains(system.Blocks[0].Text, "```bash\nyour_command_here\n```") {
		t.Fatalf("system prompt lost shell-action format: %#v", system.Blocks)
	}
	if strings.Contains(prompt, "You are the surface mapper.") {
		t.Fatalf("user prompt included persona text: %q", prompt)
	}
	if !strings.Contains(prompt, "## Task\n\nFeature request body\n") {
		t.Fatalf("user prompt lost task body: %q", prompt)
	}
}

func TestBuildOrchestrationPromptInjectsArtifactContract(t *testing.T) {
	def := orchestration.Definition{
		Name:    "test",
		Initial: "item_worker",
		States: []orchestration.State{
			{ID: "item_worker"},
		},
	}
	_, prompt := buildOrchestrationPrompt(def, orchestration.State{
		ID: "item_worker",
		Artifacts: orchestration.Artifacts{
			Inputs: []orchestration.Artifact{{
				ID:          "current_item",
				Path:        "/tmp/pragma/current-item.json",
				Required:    true,
				Description: "Current checklist item.",
			}},
			Outputs: []orchestration.Artifact{{
				ID:            "item_verdict",
				Path:          "/tmp/pragma/item-verdict.md",
				Required:      true,
				Kind:          "verdict",
				AllowedValues: []string{"APPROVE", "BLOCK"},
				Description:   "Reviewer decision.",
			}},
		},
	}, persona.Definition{
		ID:     "item_worker",
		Prompt: "Use runtime artifacts.",
	}, "task", "")

	for _, want := range []string{
		"## Runtime Artifact Contract",
		"`current_item` (required): `/tmp/pragma/current-item.json` - Current checklist item.",
		"`item_verdict` (required): `/tmp/pragma/item-verdict.md` - Reviewer decision. Kind: verdict. Allowed values: APPROVE, BLOCK.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %q", want, prompt)
		}
	}
}

func TestLoadPersonaForStateUsesExplicitPersona(t *testing.T) {
	def, err := loadPersonaForState(filepath.Join("..", "..", "personas"), orchestration.State{
		ID:      "first-pass",
		Persona: "implementer",
	})
	if err != nil {
		t.Fatalf("loadPersonaForState: %v", err)
	}
	if def.ID != "implementer" {
		t.Fatalf("persona id = %q, want implementer", def.ID)
	}
}

func TestControlName(t *testing.T) {
	if got := controlName(orchestration.State{
		Control: orchestration.Control{
			ForEachNext: &orchestration.ForEachNextControl{},
		},
	}); got != "foreach_next" {
		t.Fatalf("controlName foreach = %q, want foreach_next", got)
	}
	if got := controlName(orchestration.State{
		Control: orchestration.Control{
			MarkCurrentItem: &orchestration.MarkCurrentItemControl{},
		},
	}); got != "mark_current_item" {
		t.Fatalf("controlName mark = %q, want mark_current_item", got)
	}
	if got := controlName(orchestration.State{
		Control: orchestration.Control{
			ArtifactVerdict: &orchestration.ArtifactVerdictControl{},
		},
	}); got != "artifact_verdict" {
		t.Fatalf("controlName artifact verdict = %q, want artifact_verdict", got)
	}
}
