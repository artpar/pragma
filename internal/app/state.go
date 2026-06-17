package app

import (
	"time"

	"github.com/artpar/pragma/internal/model"
)

// WorktreeSession tracks a temporary worktree entered during the active session.
type WorktreeSession struct {
	OriginalCWD  string `json:"original_cwd"`
	WorktreePath string `json:"worktree_path"`
	Branch       string `json:"branch"`
	HeadCommit   string `json:"head_commit"`
}

type OrchestrationArtifact struct {
	StateID    string    `json:"state_id,omitempty"`
	From       string    `json:"from,omitempty"`
	Event      string    `json:"event,omitempty"`
	To         string    `json:"to,omitempty"`
	ArtifactID string    `json:"artifact_id,omitempty"`
	Path       string    `json:"path"`
	Direction  string    `json:"direction,omitempty"`
	Root       string    `json:"root,omitempty"`
	Bytes      int64     `json:"bytes,omitempty"`
	SHA256     string    `json:"sha256,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
}

// AppState is the full application state.
type AppState struct {
	Conversation           model.Conversation      `json:"conversation"`
	CWD                    string                  `json:"cwd"`
	Model                  string                  `json:"model"`
	Provider               string                  `json:"provider"`
	MaxTokens              int                     `json:"max_tokens"`
	Temperature            *float64                `json:"temperature,omitempty"`
	Thinking               *bool                   `json:"thinking,omitempty"`
	PromptHistory          []string                `json:"prompt_history,omitempty"`
	OrchestrationArtifacts []OrchestrationArtifact `json:"orchestration_artifacts,omitempty"`
	Worktree               *WorktreeSession        `json:"worktree,omitempty"`
	ArtifactSessionID      string                  `json:"artifact_session_id,omitempty"`
}

// WorkDir returns the current working directory.
func (s AppState) WorkDir() string {
	return s.CWD
}

func (s AppState) SessionID() string {
	if s.ArtifactSessionID != "" {
		return s.ArtifactSessionID
	}
	return s.Conversation.ID
}
