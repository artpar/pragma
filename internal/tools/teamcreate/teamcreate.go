package teamcreate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/team"
	"github.com/artpar/pragma/internal/tool"
)

type teamCreateInput struct {
	TeamName    string `json:"team_name" desc:"Name for the new team to create"`
	Description string `json:"description,omitempty" desc:"Team description/purpose"`
	AgentType   string `json:"agent_type,omitempty" desc:"Type/role of the team lead (e.g., researcher, test-runner)"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
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

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"TeamCreate\"")
	return "TeamCreate"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: teamCreateDescription")
	return teamCreateDescription
}

const teamCreateDescription = `Create a multi-agent swarm team with a lead agent and associated task list.

Use this when the user asks for team/swarm/group collaboration on complex tasks
that benefit from parallel work.

# Workflow
1. Create team (this tool)
2. Create tasks for the team's work items
3. Spawn teammates to work on tasks
4. Monitor progress with available task-status tools
5. When done, use the available cleanup tool

# Notes
- Team maps to one task list
- Each team has a lead agent (you) and zero or more teammates
- Teammates go idle between turns — this is normal, not an error
- Task list coordination: check periodically, claim tasks via TaskUpdate
- Team config stored at ~/.pragma/teams/{name}/config.json
- Only one team per leader at a time`

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

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "teamcreate", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "teamcreate", "Tool.CheckPerm", "exit")
	var in struct {
		TeamName string `json:"team_name"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "teamcreate", "Tool.CheckPerm", "if: err != nil")
		observe.TraceCtx(ctx, "teamcreate", "Tool.CheckPerm", "return: checker.Check(ctx, \"TeamCreate\", \"\")")
		return checker.Check(ctx, "TeamCreate", "")
	}
	observe.TraceCtx(ctx, "teamcreate", "Tool.CheckPerm", "return: checker.Check(ctx, \"TeamCreate\", in.TeamName)")
	return checker.Check(ctx, "TeamCreate", in.TeamName)
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke", "exit")

	var in teamCreateInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.TeamName == "" {
		observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke", "if: in.TeamName == \"\"")
		observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"team_name is required\")")
		return tool.InvokeResult{}, fmt.Errorf("team_name is required")
	}

	snap := t.Store.Snapshot()

	if snap.TeamContext != nil {
		observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke", "if: snap.TeamContext != nil")
		observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\n\t\"already leading team %q — delete the cur...")
		return tool.InvokeResult{}, fmt.Errorf(
			"already leading team %q — delete the current team before creating a new one",
			snap.TeamContext.TeamName)
	}

	sanitized := team.SanitizeName(in.TeamName)
	finalName := sanitized
	if team.TeamExists(finalName) {
		observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke", "if: team.TeamExists(finalName)")
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

	if err := team.WriteTeamFile(finalName, &tf); err != nil {
		observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"write team file: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("write team file: %w", err)
	}

	tasksDir := team.TasksDir(finalName)
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"create tasks dir: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("create tasks dir: %w", err)
	}

	teamFilePath := team.TeamFilePath(finalName)
	t.Store.Update(func(s *app.AppState) {
		s.TeamContext = &app.TeamContext{
			TeamName:     finalName,
			TeamFilePath: teamFilePath,
			LeadAgentID:  leadAgentID,
		}
	})

	if t.Bus != nil {
		observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke", "if: t.Bus != nil")
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
	observe.TraceCtx(ctx, "teamcreate", "Tool.Invoke", "return: tool.InvokeResult{Content: string(data)}, nil")
	return tool.InvokeResult{Content: string(data)}, nil
}
