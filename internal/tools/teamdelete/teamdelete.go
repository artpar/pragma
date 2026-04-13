package teamdelete

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/team"
	"github.com/artpar/gogent/internal/tool"
)

var inputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {}
}`)

type teamDeleteOutput struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	TeamName string `json:"team_name,omitempty"`
}

// Tool implements the TeamDelete tool for cleaning up swarm teams.
type Tool struct {
	Store *app.StateStore
	Bus   *observe.EventBus
}

func (t *Tool) Name() string        { return "TeamDelete" }
func (t *Tool) Description() string { return teamDeleteDescription }

const teamDeleteDescription = `Clean up team and task directories when swarm work is complete.

Removes team configuration, task directories, and any git worktrees created for teammates.
Will fail if active team members remain — use requestShutdown to gracefully terminate
teammates before calling this tool.`

func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "teamdelete", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "teamdelete", "Tool.CheckPerm", "exit")
	return checker.Check(ctx, "TeamDelete", "")
}

func (t *Tool) Invoke(ctx context.Context, _ json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "teamdelete", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "teamdelete", "Tool.Invoke", "exit")

	snap := t.Store.Snapshot()

	if snap.TeamContext == nil {
		out := teamDeleteOutput{Success: true, Message: "No active team to clean up"}
		data, _ := json.Marshal(out)
		return tool.InvokeResult{Content: string(data)}, nil
	}

	teamName := snap.TeamContext.TeamName

	// Check for active members
	tf, err := team.ReadTeamFile(teamName)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("read team file: %w", err)
	}

	if tf != nil {
		var activeNames []string
		for _, m := range tf.Members {
			if m.Name == team.TeamLeadName {
				continue // skip lead
			}
			// IsActive == nil or *IsActive == true means active
			if m.IsActive == nil || *m.IsActive {
				activeNames = append(activeNames, m.Name)
			}
		}
		if len(activeNames) > 0 {
			out := teamDeleteOutput{
				Success:  false,
				TeamName: teamName,
				Message: fmt.Sprintf(
					"Cannot cleanup team with %d active member(s): %s. Use requestShutdown to gracefully terminate teammates first.",
					len(activeNames), strings.Join(activeNames, ", ")),
			}
			data, _ := json.Marshal(out)
			return tool.InvokeResult{Content: string(data)}, nil
		}
	}

	// Clean up directories and worktrees
	if err := team.CleanupTeamDirectories(teamName, t.Bus); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("cleanup team: %w", err)
	}

	// Clear AppState
	t.Store.Update(func(s *app.AppState) {
		s.TeamContext = nil
	})

	// Emit event
	if t.Bus != nil {
		t.Bus.Emit(observe.TeamDeleted{
			EventHeader: observe.NewEventHeader("TeamDeleted", "", "", ""),
			TeamName:    teamName,
		})
	}

	out := teamDeleteOutput{
		Success:  true,
		Message:  "Team cleaned up successfully",
		TeamName: teamName,
	}
	data, _ := json.Marshal(out)
	return tool.InvokeResult{Content: string(data)}, nil
}
