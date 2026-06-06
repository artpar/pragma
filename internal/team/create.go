package team

import (
	"fmt"
	"os"
	"time"
)

type CreateWorkspaceInput struct {
	TeamName    string
	Description string
	AgentType   string
	Model       string
	CWD         string
}

type CreateWorkspaceResult struct {
	TeamName     string
	TeamFilePath string
	LeadAgentID  string
}

// CreateWorkspace commits the durable team config and task workspace together.
func CreateWorkspace(input CreateWorkspaceInput) (CreateWorkspaceResult, error) {
	finalName := SanitizeName(input.TeamName)
	if TeamExists(finalName) {
		finalName = GenerateWordSlug()
	}

	leadAgentID := FormatAgentID(TeamLeadName, finalName)
	now := time.Now().UnixMilli()
	tf := TeamFile{
		Name:        finalName,
		Description: input.Description,
		CreatedAt:   now,
		LeadAgentID: leadAgentID,
		Members: []TeamMember{{
			AgentID:       leadAgentID,
			Name:          TeamLeadName,
			AgentType:     input.AgentType,
			Model:         input.Model,
			JoinedAt:      now,
			CWD:           input.CWD,
			Subscriptions: []string{},
		}},
	}

	if err := WriteTeamFile(finalName, &tf); err != nil {
		return CreateWorkspaceResult{}, fmt.Errorf("write team file: %w", err)
	}
	if err := os.MkdirAll(TasksDir(finalName), 0o755); err != nil {
		rollbackCreateWorkspace(finalName)
		return CreateWorkspaceResult{}, fmt.Errorf("create tasks dir: %w", err)
	}

	return CreateWorkspaceResult{
		TeamName:     finalName,
		TeamFilePath: TeamFilePath(finalName),
		LeadAgentID:  leadAgentID,
	}, nil
}

func rollbackCreateWorkspace(teamName string) {
	_ = os.RemoveAll(TeamDir(teamName))
	_ = os.RemoveAll(TasksDir(teamName))
}
