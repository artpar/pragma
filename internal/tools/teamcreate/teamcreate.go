package teamcreate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/team"
	"github.com/artpar/gogent/internal/tool"
)

type teamCreateInput struct {
	TeamName    string `json:"team_name" desc:"Name for the new team to create"`
	Description string `json:"description,omitempty" desc:"Team description/purpose"`
	AgentType   string `json:"agent_type,omitempty" desc:"Type/role of the team lead (e.g., researcher, test-runner)"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["team_name"],
	"properties": {
		"team_name": {
			"type": "string",
			"description": "Name for the new team to create"
		},
		"description": {
			"type": "string",
			"description": "Team description/purpose"
		},
		"agent_type": {
			"type": "string",
			"description": "Type/role of the team lead (e.g., researcher, test-runner)"
		}
	}
}`)

type teamCreateOutput struct {
	TeamName     string `json:"team_name"`
	TeamFilePath string `json:"team_file_path"`
	LeadAgentID  string `json:"lead_agent_id"`
}

// Tool implements the TeamCreate tool for creating multi-agent swarm teams.
type Tool struct {
	Store *app.StateStore
	Bus   *observe.EventBus
}

func (t *Tool) Name() string        { return "TeamCreate" }
func (t *Tool) Description() string { return teamCreateDescription }

const teamCreateDescription = `Create a multi-agent swarm team with a lead agent and associated task list.

Use this when the user asks for team/swarm/group collaboration on complex tasks
that benefit from parallel work.

# Workflow
1. Create team (this tool)
2. Create tasks via TaskCreate for the team's work items
3. Spawn teammates via Agent tool to work on tasks
4. Monitor progress via TaskList/TaskGet
5. When done, use TeamDelete to clean up

# Notes
- Team = TaskList (1:1 correspondence)
- Each team has a lead agent (you) and zero or more teammates
- Teammates go idle between turns — this is normal, not an error
- Task list coordination: check periodically, claim tasks via TaskUpdate
- Team config stored at ~/.gogent/teams/{name}/config.json
- Only one team per leader at a time`

func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "teamcreate", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "teamcreate", "Tool.CheckPerm", "exit")
	var in struct {
		TeamName string `json:"team_name"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return checker.Check(ctx, "TeamCreate", "")
	}
	return checker.Check(ctx, "TeamCreate", in.TeamName)
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke", "exit")

	var in teamCreateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.TeamName == "" {
		return tool.InvokeResult{}, fmt.Errorf("team_name is required")
	}

	snap := t.Store.Snapshot()

	// Only one team per leader
	if snap.TeamContext != nil {
		return tool.InvokeResult{}, fmt.Errorf(
			"already leading team %q — delete the current team before creating a new one",
			snap.TeamContext.TeamName)
	}

	// Resolve unique name
	sanitized := team.SanitizeName(in.TeamName)
	finalName := sanitized
	if team.TeamExists(finalName) {
		finalName = team.GenerateWordSlug()
		observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke",
			fmt.Sprintf("name conflict, generated slug: %s", finalName))
	}

	leadAgentID := team.FormatAgentID(team.TeamLeadName, finalName)
	now := time.Now().UnixMilli()

	tf := team.TeamFile{
		Name:        finalName,
		Description: in.Description,
		CreatedAt:   now,
		LeadAgentID: leadAgentID,
		Members: []team.TeamMember{{
			AgentID:       leadAgentID,
			Name:          team.TeamLeadName,
			AgentType:     in.AgentType,
			Model:         snap.Model,
			JoinedAt:      now,
			CWD:           snap.CWD,
			Subscriptions: []string{},
		}},
	}

	// Write team file (atomic: temp + rename)
	if err := team.WriteTeamFile(finalName, &tf); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("write team file: %w", err)
	}

	// Create tasks directory
	tasksDir := team.TasksDir(finalName)
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("create tasks dir: %w", err)
	}

	// Update AppState
	teamFilePath := team.TeamFilePath(finalName)
	t.Store.Update(func(s *app.AppState) {
		s.TeamContext = &app.TeamContext{
			TeamName:     finalName,
			TeamFilePath: teamFilePath,
			LeadAgentID:  leadAgentID,
		}
	})

	// Emit event
	if t.Bus != nil {
		t.Bus.Emit(observe.TeamCreated{
			EventHeader: observe.NewEventHeader("TeamCreated", "", "", ""),
			TeamName:    finalName,
			LeadAgentID: leadAgentID,
			MemberCount: 1,
		})
	}

	out := teamCreateOutput{
		TeamName:     finalName,
		TeamFilePath: teamFilePath,
		LeadAgentID:  leadAgentID,
	}
	data, _ := json.Marshal(out)
	return tool.InvokeResult{Content: string(data)}, nil
}
