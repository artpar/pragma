package teamdelete

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/team"
)

func newTestTool(t *testing.T) (*Tool, *app.StateStore) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	store := app.NewStateStore(app.AppState{CWD: tmp})
	bus := observe.NewEventBus(100)
	return &Tool{Store: store, Bus: bus}, store
}

func TestInvoke_NoActiveTeam(t *testing.T) {
	tl, _ := newTestTool(t)

	result, err := tl.Invoke(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var out teamDeleteOutput
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.Success {
		t.Error("expected Success = true for no active team")
	}
	if !strings.Contains(out.Message, "No active team") {
		t.Errorf("Message = %q, want 'No active team'", out.Message)
	}
}

func TestInvoke_SuccessfulDelete(t *testing.T) {
	tl, store := newTestTool(t)

	// Create a team first
	tf := &team.TeamFile{
		Name:        "delete-me",
		CreatedAt:   1,
		LeadAgentID: "team-lead@delete-me",
		Members: []team.TeamMember{
			{
				AgentID:       "team-lead@delete-me",
				Name:          "team-lead",
				JoinedAt:      1,
				CWD:           "/tmp",
				Subscriptions: []string{},
			},
		},
	}
	if err := team.WriteTeamFile("delete-me", tf); err != nil {
		t.Fatalf("WriteTeamFile: %v", err)
	}

	store.Update(func(s *app.AppState) {
		s.TeamContext = &app.TeamContext{
			TeamName:     "delete-me",
			TeamFilePath: team.TeamFilePath("delete-me"),
			LeadAgentID:  "team-lead@delete-me",
		}
	})

	result, err := tl.Invoke(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var out teamDeleteOutput
	json.Unmarshal([]byte(result.Content), &out)
	if !out.Success {
		t.Errorf("Success = false, want true. Message: %s", out.Message)
	}

	// Verify AppState cleared
	snap := store.Snapshot()
	if snap.TeamContext != nil {
		t.Error("TeamContext should be nil after delete")
	}

	// Verify team file removed
	if team.TeamExists("delete-me") {
		t.Error("team file still exists after delete")
	}
}

func TestInvoke_ActiveMembersRejection(t *testing.T) {
	tl, store := newTestTool(t)

	// Create team with an active non-lead member
	tf := &team.TeamFile{
		Name:        "active-test",
		CreatedAt:   1,
		LeadAgentID: "team-lead@active-test",
		Members: []team.TeamMember{
			{
				AgentID:       "team-lead@active-test",
				Name:          "team-lead",
				JoinedAt:      1,
				CWD:           "/tmp",
				Subscriptions: []string{},
			},
			{
				AgentID:       "worker@active-test",
				Name:          "worker-1",
				JoinedAt:      1,
				CWD:           "/tmp",
				IsActive:      nil, // nil = active
				Subscriptions: []string{},
			},
		},
	}
	if err := team.WriteTeamFile("active-test", tf); err != nil {
		t.Fatalf("WriteTeamFile: %v", err)
	}

	store.Update(func(s *app.AppState) {
		s.TeamContext = &app.TeamContext{
			TeamName:     "active-test",
			TeamFilePath: team.TeamFilePath("active-test"),
			LeadAgentID:  "team-lead@active-test",
		}
	})

	result, err := tl.Invoke(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var out teamDeleteOutput
	json.Unmarshal([]byte(result.Content), &out)
	if out.Success {
		t.Error("expected Success = false for active members")
	}
	if !strings.Contains(out.Message, "active member") {
		t.Errorf("Message = %q, want mention of active members", out.Message)
	}
	if !strings.Contains(out.Message, "worker-1") {
		t.Errorf("Message = %q, want worker-1 name listed", out.Message)
	}
}
