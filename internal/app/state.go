package app

import "github.com/artpar/gogent/internal/model"

// TodoItem represents a single item in the session task checklist.
type TodoItem struct {
	Content string `json:"content"`
	Status  string `json:"status"` // "pending", "in_progress", "completed"
}

// AppState is the full application state.
// Satisfies tool.StateSnapshot via WorkDir() method.
type AppState struct {
	Conversation model.Conversation `json:"conversation"`
	CWD          string             `json:"cwd"`
	Model        string             `json:"model"`
	Provider     string             `json:"provider"`
	MaxTokens    int                `json:"max_tokens"`
	Temperature  *float64           `json:"temperature,omitempty"`
	Thinking     *bool              `json:"thinking,omitempty"`
	Todos        []TodoItem         `json:"todos,omitempty"`
	PlanMode     bool               `json:"plan_mode,omitempty"`
	PlanFile     string             `json:"plan_file,omitempty"`
}

// WorkDir returns the current working directory.
// This satisfies tool.StateSnapshot.
func (s AppState) WorkDir() string {
	return s.CWD
}
