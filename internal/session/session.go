package session

import (
	"time"

	"github.com/artpar/pragma/internal/model"
)

// Session wraps a Conversation with persistence metadata.
type Session struct {
	Conversation        model.Conversation               `json:"conversation"`
	HandoffState        model.HandoffState               `json:"handoff_state,omitempty"`
	Summary             string                           `json:"summary,omitempty"`
	CostUSD             float64                          `json:"cost_usd,omitempty"`
	TurnCount           int                              `json:"turn_count"`
	TokenUsage          model.TokenUsage                 `json:"token_usage,omitzero"`
	SystemOverride      string                           `json:"system_override,omitempty"`
	GitRemote           string                           `json:"git_remote,omitempty"`
	ContentReplacements []model.ContentReplacementRecord `json:"content_replacements,omitempty"`
	PromptHistory       []PromptHistoryData              `json:"prompt_history,omitempty"`
}

// SessionSummary is a lightweight view for listing sessions.
type SessionSummary struct {
	ID        string    `json:"id"`
	Summary   string    `json:"summary"`
	Model     string    `json:"model"`
	WorkDir   string    `json:"work_dir"`
	TurnCount int       `json:"turn_count"`
	CostUSD   float64   `json:"cost_usd"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
