package background

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/artpar/pragma/internal/observe"
	"time"
)

// Status represents the current state of a background process.
type Status string

const (
	StatusStarting Status = "starting"
	StatusBusy     Status = "busy"
	StatusIdle     Status = "idle"
	StatusWaiting  Status = "waiting"
)

const (
	HeartbeatInterval = 5 * time.Second
	HeartbeatTimeout  = 20 * time.Second
)

// ProcessInfo holds metadata about a running background process.
// SessionID is attached after the child runtime starts a domain session.
// Persisted as ~/.pragma/active-sessions/{pid}.json.
type ProcessInfo struct {
	PID         int       `json:"pid"`
	PGID        int       `json:"pgid"` // Process group ID for cleanup
	SessionID   string    `json:"session_id"`
	OwnerToken  string    `json:"owner_token,omitempty"`
	CWD         string    `json:"cwd"`
	StartedAt   time.Time `json:"started_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	HeartbeatAt time.Time `json:"heartbeat_at,omitzero"`
	Status      Status    `json:"status"`
	LogPath     string    `json:"log_path"`
	Name        string    `json:"name,omitempty"`
	Model       string    `json:"model"`
	Provider    string    `json:"provider"`
	Prompt      string    `json:"prompt,omitempty"` // First 200 chars for display
}

func (p ProcessInfo) HasSession() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: p.SessionID != \"\"")
	return p.SessionID != ""
}

func (p ProcessInfo) HasFreshHeartbeat(now time.Time) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: !p.HeartbeatAt.IsZero() && now.Sub(p.HeartbeatAt) <= HeartbeatTimeout")
	return !p.HeartbeatAt.IsZero() && now.Sub(p.HeartbeatAt) <= HeartbeatTimeout
}

func NewOwnerToken() (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}
	observe.GlobalTrace("return: hex.EncodeToString(b[:]), nil")
	return hex.EncodeToString(b[:]), nil
}
