package app

import (
	"time"

	"github.com/artpar/pragma/internal/model"
)

// TodoItem represents a single item in the session task checklist.
type TodoItem struct {
	Content string `json:"content"`
	Status  string `json:"status"` // "pending", "in_progress", "completed"
}

// TeamContext tracks the current team leadership state.
// Defined in app/ (not team/) to avoid import cycle.
type TeamContext struct {
	TeamName     string `json:"team_name"`
	TeamFilePath string `json:"team_file_path"`
	LeadAgentID  string `json:"lead_agent_id"`
}

// WorktreeSession tracks a temporary worktree entered during the active session.
type WorktreeSession struct {
	OriginalCWD  string `json:"original_cwd"`
	WorktreePath string `json:"worktree_path"`
	Branch       string `json:"branch"`
	HeadCommit   string `json:"head_commit"`
}

type OrchestrationArtifact struct {
	StateID   string    `json:"state_id,omitempty"`
	Event     string    `json:"event,omitempty"`
	Path      string    `json:"path"`
	Direction string    `json:"direction,omitempty"`
	Root      string    `json:"root,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// AppState is the full application state.
// Satisfies tool.StateSnapshot via WorkDir() method.
type AppState struct {
	Conversation           model.Conversation      `json:"conversation"`
	HandoffState           model.HandoffState      `json:"handoff_state,omitempty"`
	CWD                    string                  `json:"cwd"`
	Model                  string                  `json:"model"`
	Provider               string                  `json:"provider"`
	MaxTokens              int                     `json:"max_tokens"`
	Temperature            *float64                `json:"temperature,omitempty"`
	Thinking               *bool                   `json:"thinking,omitempty"`
	Todos                  []TodoItem              `json:"todos,omitempty"`
	PromptHistory          []string                `json:"prompt_history,omitempty"`
	OrchestrationArtifacts []OrchestrationArtifact `json:"orchestration_artifacts,omitempty"`
	AdvisorModel           string                  `json:"advisor_model,omitempty"`
	TeamContext            *TeamContext            `json:"team_context,omitempty"`
	Worktree               *WorktreeSession        `json:"worktree,omitempty"`
	ArtifactSessionID      string                  `json:"artifact_session_id,omitempty"`
}

// WorkDir returns the current working directory.
// This satisfies tool.StateSnapshot.
func (s AppState) WorkDir() string {
	return s.CWD
}

func (s AppState) SessionID() string {
	if s.ArtifactSessionID != "" {
		return s.ArtifactSessionID
	}
	return s.Conversation.ID
}
