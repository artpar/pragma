package session

import (
	"time"

	"github.com/artpar/gogent/internal/model"
)

// Session wraps a Conversation with persistence metadata.
type Session struct {
	Conversation   model.Conversation `json:"conversation"`
	Summary        string             `json:"summary,omitempty"`
	CostUSD        float64            `json:"cost_usd,omitempty"`
	TurnCount      int                `json:"turn_count"`
	SystemOverride string             `json:"system_override,omitempty"`
	GitRemote      string             `json:"git_remote,omitempty"`
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

// summaryFromSession extracts a SessionSummary from a full Session.
func summaryFromSession(sess Session) SessionSummary {
	return SessionSummary{
		ID:        sess.Conversation.ID,
		Summary:   sess.Summary,
		Model:     sess.Conversation.Model,
		WorkDir:   sess.Conversation.WorkDir,
		TurnCount: sess.TurnCount,
		CostUSD:   sess.CostUSD,
		CreatedAt: sess.Conversation.CreatedAt,
		UpdatedAt: sess.Conversation.UpdatedAt,
	}
}
