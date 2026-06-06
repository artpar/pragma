package session

import (
	"encoding/json"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/tool"
)

// EntryKind discriminates JSONL line types in session files.
type EntryKind string

const (
	EntryHeader                 EntryKind = "header"
	EntryMessage                EntryKind = "message"
	EntryMetadata               EntryKind = "metadata"
	EntryHandoffState           EntryKind = "handoff_state"
	EntryContentReplacement     EntryKind = "content_replacement"
	EntryPromptHistory          EntryKind = "prompt_history"
	EntryFileState              EntryKind = "file_state"
	EntryTodos                  EntryKind = "todos"
	EntryTeamContext            EntryKind = "team_context"
	EntryTaskResult             EntryKind = "task_result"
	EntryOrchestrationArtifacts EntryKind = "orchestration_artifacts"
	EntryWebEvent               EntryKind = "web_event"
)

// Entry is a single JSONL line in a session file. Discriminated by Kind.
type Entry struct {
	Kind EntryKind       `json:"kind"`
	Data json.RawMessage `json:"data"`
}

// HeaderData is the session-level metadata written as the first JSONL line.
type HeaderData struct {
	SessionID      string             `json:"session_id"`
	Model          string             `json:"model"`
	Provider       string             `json:"provider"`
	WorkDir        string             `json:"work_dir"`
	GitRemote      string             `json:"git_remote,omitempty"`
	SystemOverride string             `json:"system_override,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	System         model.SystemPrompt `json:"system"`
}

// MetadataData carries mutable session counters, appended after each turn.
// On load, the last MetadataData entry wins.
type MetadataData struct {
	CostUSD    float64              `json:"cost_usd"`
	TurnCount  int                  `json:"turn_count"`
	TokenUsage model.TokenUsage     `json:"token_usage"`
	UpdatedAt  time.Time            `json:"updated_at"`
	Summary    string               `json:"summary,omitempty"`
	Model      string               `json:"model,omitempty"`
	Provider   string               `json:"provider,omitempty"`
	WorkDir    string               `json:"work_dir,omitempty"`
	Worktree   *app.WorktreeSession `json:"worktree,omitempty"`
}

type ContentReplacementData struct {
	Records []model.ContentReplacementRecord `json:"records"`
}

type PromptHistoryData struct {
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

type FileStateData struct {
	Records []tool.FileStateRecord `json:"records"`
}

type TodosData struct {
	Items []app.TodoItem `json:"items"`
}

type TeamContextData struct {
	Context *app.TeamContext `json:"context,omitempty"`
}

type OrchestrationArtifactsData struct {
	Artifacts []app.OrchestrationArtifact `json:"artifacts"`
}

type TaskResultData struct {
	TaskID     string    `json:"task_id"`
	Subject    string    `json:"subject,omitempty"`
	AgentName  string    `json:"agent_name,omitempty"`
	Status     string    `json:"status"`
	Result     string    `json:"result,omitempty"`
	Error      string    `json:"error,omitempty"`
	TokensUsed int       `json:"tokens_used,omitempty"`
	DurationMs int64     `json:"duration_ms,omitempty"`
	TurnCount  int       `json:"turn_count,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}

type WebEventData struct {
	Sequence   int             `json:"sequence"`
	ReceivedAt time.Time       `json:"received_at"`
	Type       string          `json:"type"`
	DataType   string          `json:"data_type"`
	Data       json.RawMessage `json:"data"`
}

type HandoffStateData struct {
	State model.HandoffState `json:"state"`
}

// MarshalEntry creates a JSONL-ready Entry from typed data.
func MarshalEntry(kind EntryKind, data any) (Entry, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return Entry{}, err
	}
	return Entry{Kind: kind, Data: raw}, nil
}
