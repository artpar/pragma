package background

import "time"

// Status represents the current state of a background process.
type Status string

const (
	StatusStarting Status = "starting"
	StatusBusy     Status = "busy"
	StatusIdle     Status = "idle"
	StatusWaiting  Status = "waiting"
)

// ProcessInfo holds metadata about a running background process.
// SessionID is attached after the child runtime starts a domain session.
// Persisted as ~/.pragma/active-sessions/{pid}.json.
type ProcessInfo struct {
	PID       int       `json:"pid"`
	PGID      int       `json:"pgid"` // Process group ID for cleanup
	SessionID string    `json:"session_id"`
	CWD       string    `json:"cwd"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Status    Status    `json:"status"`
	LogPath   string    `json:"log_path"`
	Name      string    `json:"name,omitempty"`
	Model     string    `json:"model"`
	Provider  string    `json:"provider"`
	Prompt    string    `json:"prompt,omitempty"` // First 200 chars for display
}

func (p ProcessInfo) HasSession() bool {
	return p.SessionID != ""
}
