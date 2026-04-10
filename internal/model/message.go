package model

import (
	"encoding/json"
	"fmt"
	"time"
)

// Role identifies the speaker of a message.
// Only 2 roles — tool results live as ToolResultPart inside a RoleUser message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// MessageFlags controls message visibility and semantics.
type MessageFlags struct {
	IsInternal       bool `json:"is_internal,omitempty"`
	IsCompactSummary bool `json:"is_compact_summary,omitempty"`
	IsMeta           bool `json:"is_meta,omitempty"`
}

// Message is a single turn in a conversation.
type Message struct {
	ID        string        `json:"id"`
	Role      Role          `json:"role"`
	Content   []ContentPart `json:"content"`
	Timestamp time.Time     `json:"timestamp"`
	Flags     MessageFlags  `json:"flags,omitzero"`
}

// messageJSON is the raw JSON shape for Message, with content as raw bytes
// so we can dispatch through the ContentPart discriminator.
type messageJSON struct {
	ID        string          `json:"id"`
	Role      Role            `json:"role"`
	Content   json.RawMessage `json:"content"`
	Timestamp time.Time       `json:"timestamp"`
	Flags     MessageFlags    `json:"flags,omitzero"`
}

func (m Message) MarshalJSON() ([]byte, error) {
	content, err := MarshalContentParts(m.Content)
	if err != nil {
		return nil, fmt.Errorf("marshal message content: %w", err)
	}
	return json.Marshal(messageJSON{
		ID:        m.ID,
		Role:      m.Role,
		Content:   content,
		Timestamp: m.Timestamp,
		Flags:     m.Flags,
	})
}

func (m *Message) UnmarshalJSON(data []byte) error {
	var raw messageJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal message: %w", err)
	}
	parts, err := UnmarshalContentParts(raw.Content)
	if err != nil {
		return fmt.Errorf("unmarshal message content: %w", err)
	}
	m.ID = raw.ID
	m.Role = raw.Role
	m.Content = parts
	m.Timestamp = raw.Timestamp
	m.Flags = raw.Flags
	return nil
}

// SystemBlock is one block of the system prompt.
// Cacheable is a hint — the provider decides how to implement caching.
type SystemBlock struct {
	Text      string `json:"text"`
	Cacheable bool   `json:"cacheable,omitempty"`
}

// SystemPrompt is the complete system prompt, composed of ordered blocks.
type SystemPrompt struct {
	Blocks []SystemBlock `json:"blocks"`
}
