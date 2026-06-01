package orchestration

import (
	"context"
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
