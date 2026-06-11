package session

import (
	"encoding/json"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
)

// EntryKind discriminates JSONL line types in session files.
type EntryKind string

const (
	EntryHeader                 EntryKind = "header"
	EntryMessage                EntryKind = "message"
	EntryMetadata               EntryKind = "metadata"
	EntryContentReplacement     EntryKind = "content_replacement"
	EntryPromptHistory          EntryKind = "prompt_history"
	EntryOrchestrationArtifacts EntryKind = "orchestration_artifacts"
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

type OrchestrationArtifactsData struct {
	Artifacts []app.OrchestrationArtifact `json:"artifacts"`
}

// MarshalEntry creates a JSONL-ready Entry from typed data.
func MarshalEntry(kind EntryKind, data any) (Entry, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return Entry{}, err
	}
	return Entry{Kind: kind, Data: raw}, nil
}
