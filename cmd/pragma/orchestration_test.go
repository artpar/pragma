package main

import (
	"os"
	"path/filepath"
	"testing"

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
