package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/cli"
	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/orchestration"
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
