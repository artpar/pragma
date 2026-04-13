package teamcreate

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/team"
)

type staticState struct{ dir string }

func (s staticState) WorkDir() string { return s.dir }

func newTestTool(t *testing.T) (*Tool, *app.StateStore) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	store := app.NewStateStore(app.AppState{CWD: tmp, Model: "test-model"})
	bus := observe.NewEventBus(100)
	return &Tool{Store: store, Bus: bus}, store
}

func TestInvoke_SuccessfulCreate(t *testing.T) {
	tl, store := newTestTool(t)

	input, _ := json.Marshal(teamCreateInput{TeamName: "my-team", Description: "test team"})
	result, err := tl.Invoke(context.Background(), input, staticState{"/tmp"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	// Verify output JSON
	var out teamCreateOutput
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.TeamName != "my-team" {
		t.Errorf("TeamName = %q, want 'my-team'", out.TeamName)
	}
	if out.LeadAgentID != "team-lead@my-team" {
		t.Errorf("LeadAgentID = %q, want 'team-lead@my-team'", out.LeadAgentID)
	}

	// Verify AppState updated
	snap := store.Snapshot()
	if snap.TeamContext == nil {
		t.Fatal("TeamContext is nil after create")
	}
	if snap.TeamContext.TeamName != "my-team" {
		t.Errorf("TeamContext.TeamName = %q, want 'my-team'", snap.TeamContext.TeamName)
	}

	// Verify team file was written
	tf, err := team.ReadTeamFile("my-team")
	if err != nil {
		t.Fatalf("ReadTeamFile: %v", err)
	}
	if tf == nil {
		t.Fatal("team file not created")
	}
	if len(tf.Members) != 1 {
		t.Errorf("Members = %d, want 1", len(tf.Members))
	}
	if tf.Members[0].Name != "team-lead" {
		t.Errorf("Member name = %q, want 'team-lead'", tf.Members[0].Name)
	}
}

func TestInvoke_NameSanitization(t *testing.T) {
	tl, _ := newTestTool(t)

	input, _ := json.Marshal(teamCreateInput{TeamName: "My Cool Team!"})
	result, err := tl.Invoke(context.Background(), input, staticState{"/tmp"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var out teamCreateOutput
	json.Unmarshal([]byte(result.Content), &out)
	if out.TeamName != "my-cool-team" {
		t.Errorf("TeamName = %q, want sanitized 'my-cool-team'", out.TeamName)
	}
}

func TestInvoke_AlreadyLeading(t *testing.T) {
	tl, store := newTestTool(t)

	// Pre-set team context
	store.Update(func(s *app.AppState) {
		s.TeamContext = &app.TeamContext{
			TeamName: "existing-team",
		}
	})

	input, _ := json.Marshal(teamCreateInput{TeamName: "new-team"})
	_, err := tl.Invoke(context.Background(), input, staticState{"/tmp"})
	if err == nil {
		t.Fatal("expected error when already leading")
	}
	if !strings.Contains(err.Error(), "already leading team") {
		t.Errorf("error = %q, want 'already leading team'", err.Error())
	}
}

func TestInvoke_EmptyName(t *testing.T) {
	tl, _ := newTestTool(t)

	input, _ := json.Marshal(teamCreateInput{TeamName: ""})
	_, err := tl.Invoke(context.Background(), input, staticState{"/tmp"})
	if err == nil {
		t.Fatal("expected error for empty team_name")
	}
	if !strings.Contains(err.Error(), "team_name is required") {
		t.Errorf("error = %q, want 'team_name is required'", err.Error())
	}
}

func TestInvoke_NameConflict(t *testing.T) {
	tl, _ := newTestTool(t)

	// Create first team
	input, _ := json.Marshal(teamCreateInput{TeamName: "conflict-test"})
	tl.Invoke(context.Background(), input, staticState{"/tmp"})

	// Reset team context to allow second create
	tl.Store.Update(func(s *app.AppState) {
		s.TeamContext = nil
	})

	// Create with same name — should get a generated slug
	input, _ = json.Marshal(teamCreateInput{TeamName: "conflict-test"})
	result, err := tl.Invoke(context.Background(), input, staticState{"/tmp"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var out teamCreateOutput
	json.Unmarshal([]byte(result.Content), &out)
	// The slug should be different from "conflict-test" (generated word slug)
	if out.TeamName == "conflict-test" {
		t.Error("expected generated slug for name conflict, got original name")
	}
}
