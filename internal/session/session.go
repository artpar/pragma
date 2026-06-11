package session

import (
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
)

// Session wraps a Conversation with persistence metadata.
type Session struct {
	Conversation           model.Conversation               `json:"conversation"`
	Summary                string                           `json:"summary,omitempty"`
	CostUSD                float64                          `json:"cost_usd,omitempty"`
	TurnCount              int                              `json:"turn_count"`
	TokenUsage             model.TokenUsage                 `json:"token_usage,omitzero"`
	GitRemote              string                           `json:"git_remote,omitempty"`
	ContentReplacements    []model.ContentReplacementRecord `json:"content_replacements,omitempty"`
	PromptHistory          []PromptHistoryData              `json:"prompt_history,omitempty"`
	OrchestrationArtifacts []app.OrchestrationArtifact      `json:"orchestration_artifacts,omitempty"`
	Worktree               *app.WorktreeSession             `json:"worktree,omitempty"`
}

// SessionSummary is a lightweight view for listing sessions.
type SessionSummary struct {
	ID        string    `json:"id"`
	Summary   string    `json:"summary"`
	Model     string    `json:"model"`
	Provider  string    `json:"provider"`
	WorkDir   string    `json:"work_dir"`
	TurnCount int       `json:"turn_count"`
	CostUSD   float64   `json:"cost_usd"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
