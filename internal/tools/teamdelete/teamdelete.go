package teamdelete

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/task"
	"github.com/artpar/pragma/internal/team"
	"github.com/artpar/pragma/internal/tool"
)

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
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
	Tasks *task.Registry
	Bus   *observe.EventBus
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"TeamDelete\"")
	return "TeamDelete"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: teamDeleteDescription")
	return teamDeleteDescription
}

const teamDeleteDescription = `Clean up team and task directories when swarm work is complete.

Removes team configuration, task directories, and any git worktrees created for teammates.
Will fail if active teammates remain — use requestShutdown to gracefully terminate
teammates before calling this tool.`

func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: false}")
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "teamdelete", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "teamdelete", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "teamdelete", "Tool.CheckPerm", "return: checker.Check(ctx, \"TeamDelete\", \"\")")
	return checker.Check(ctx, "TeamDelete", "")
}

func (t *Tool) Invoke(ctx context.Context, _ json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "teamdelete", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "teamdelete", "Tool.Invoke", "exit")

	snap := t.Store.Snapshot()

	if snap.TeamContext == nil {
		observe.TraceCtx(ctx, "teamdelete", "Tool.Invoke", "if: snap.TeamContext == nil")
		out := teamDeleteOutput{Success: true, Message: "No active team to clean up"}
		data, _ := json.Marshal(out)
		observe.TraceCtx(ctx, "teamdelete", "Tool.Invoke", "return: tool.InvokeResult{Content: string(data)}, nil")
		return tool.InvokeResult{Content: string(data)}, nil
	}

	teamName := snap.TeamContext.TeamName

	activeNames := t.activeTeammateNames()
	if len(activeNames) > 0 {
		observe.TraceCtx(ctx, "teamdelete", "Tool.Invoke", "if: len(activeNames) > 0")
		out := teamDeleteOutput{
			Success:  false,
			TeamName: teamName,
			Message: fmt.Sprintf(
				"Cannot cleanup team with %d active teammate(s): %s. Use requestShutdown to gracefully terminate teammates first.",
				len(activeNames), strings.Join(activeNames, ", ")),
		}
		data, _ := json.Marshal(out)
		observe.TraceCtx(ctx, "teamdelete", "Tool.Invoke", "return: tool.InvokeResult{Content: string(data)}, nil")
		return tool.InvokeResult{Content: string(data)}, nil
	}

	if err := team.CleanupTeamDirectories(teamName, t.Bus); err != nil {
		observe.TraceCtx(ctx, "teamdelete", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "teamdelete", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"cleanup team: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("cleanup team: %w", err)
	}

	t.Store.Update(func(s *app.AppState) {
		s.TeamContext = nil
	})

	if t.Bus != nil {
		observe.TraceCtx(ctx, "teamdelete", "Tool.Invoke", "if: t.Bus != nil")
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
	observe.TraceCtx(ctx, "teamdelete", "Tool.Invoke", "return: tool.InvokeResult{Content: string(data)}, nil")
	return tool.InvokeResult{Content: string(data)}, nil
}

func (t *Tool) activeTeammateNames() []string {
	if t.Tasks == nil {
		return nil
	}
	teammates := t.Tasks.ListTeammates(false)
	names := make([]string, 0, len(teammates))
	for _, tk := range teammates {
		names = append(names, tk.AgentName)
	}
	return names
}
