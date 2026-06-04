package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/cli"
	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/model"
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

func TestSelectStateEventFromFileRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verdict.md")
	if err := os.WriteFile(path, []byte("Findings:\nNone\n\nDecision:\nAPPROVE\n"), 0o600); err != nil {
		t.Fatalf("write verdict: %v", err)
	}

	event, err := selectStateEvent(orchestration.State{
		ID: "review",
		Event: orchestration.Event{
			FromFile: &orchestration.FileEventRule{
				Path: path,
				Rules: []orchestration.TextEvent{
					{Contains: "Decision:\nAPPROVE", Event: "approve"},
					{Contains: "Decision:\nBLOCK", Event: "block"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("selectStateEvent: %v", err)
	}
	if event != "approve" {
		t.Fatalf("event = %q, want approve", event)
	}
}

func TestSelectStateEventUsesLastDecisionVerdict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verdict.md")
	content := "Findings:\nPrevious text said Decision:\nAPPROVE\n\nDecision:\nBLOCK\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write verdict: %v", err)
	}

	event, err := selectStateEvent(orchestration.State{
		ID: "review",
		Event: orchestration.Event{
			FromFile: &orchestration.FileEventRule{
				Path: path,
				Rules: []orchestration.TextEvent{
					{Contains: "Decision:\nAPPROVE", Event: "approve"},
					{Contains: "Decision:\nBLOCK", Event: "block"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("selectStateEvent: %v", err)
	}
	if event != "block" {
		t.Fatalf("event = %q, want block", event)
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
	if !strings.Contains(prompt, `/tmp/pragma/handoff-prompts/surface_mapper/complete.md`) {
		t.Fatalf("user prompt lost next handoff path: %q", prompt)
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
}

func TestNewIsolatedOrchestrationStoreDropsConversationHistory(t *testing.T) {
	system := model.SystemPrompt{
		Blocks: []model.SystemBlock{{Text: "system prompt", Cacheable: true}},
	}
	conv := model.NewConversation(system, "model-a", "provider-a", "/work")
	conv.Append(model.Message{
		ID:      "previous-message",
		Role:    model.RoleUser,
		Content: []model.ContentPart{model.TextPart{Text: "previous persona state"}},
	})

	d := &cli.Deps{
		Cfg: config.Config{
			Model:     "model-a",
			Provider:  "provider-a",
			MaxTokens: 1234,
		},
		Cwd: "/work",
		Store: app.NewStateStore(app.AppState{
			Conversation: conv,
			CWD:          "/work",
			Model:        "model-a",
			Provider:     "provider-a",
			MaxTokens:    1234,
		}),
	}

	store := newIsolatedOrchestrationStore(d)
	snap := store.Snapshot()

	if len(snap.Conversation.Messages) != 0 {
		t.Fatalf("isolated messages = %d, want 0", len(snap.Conversation.Messages))
	}
	if len(snap.Conversation.System.Blocks) != 1 || snap.Conversation.System.Blocks[0].Text != "system prompt" {
		t.Fatalf("isolated system prompt = %#v, want original system prompt", snap.Conversation.System.Blocks)
	}
	if snap.Conversation.ID == conv.ID {
		t.Fatal("isolated conversation reused root conversation ID")
	}
	if snap.CWD != "/work" || snap.Model != "model-a" || snap.Provider != "provider-a" || snap.MaxTokens != 1234 {
		t.Fatalf("isolated app state metadata = cwd=%q model=%q provider=%q max=%d", snap.CWD, snap.Model, snap.Provider, snap.MaxTokens)
	}
}
