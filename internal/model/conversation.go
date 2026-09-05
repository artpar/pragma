package model

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"
)

// Conversation is an ordered sequence of messages with metadata.
type Conversation struct {
	ID        string       `json:"id"`
	Messages  []Message    `json:"messages"`
	System    SystemPrompt `json:"system"`
	Model     string       `json:"model"`
	Provider  string       `json:"provider"`
	WorkDir   string       `json:"work_dir"`
	ParentID  string       `json:"parent_id,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// NewConversation creates a conversation with a generated UUID and current timestamps.
func NewConversation(system SystemPrompt, model string, provider string, workDir string) Conversation {
	now := time.Now()
	return Conversation{
		ID:        NewUUID(),
		Messages:  []Message{},
		System:    system,
		Model:     model,
		Provider:  provider,
		WorkDir:   workDir,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Append adds a message and updates the timestamp.
func (c *Conversation) Append(msg Message) {
	c.Messages = append(c.Messages, msg)
	c.UpdatedAt = time.Now()
}

// Fork creates a deep copy of the conversation with a new ID and parent link.
// All slices inside ContentParts (ImagePart.Data, DocumentPart.Data, ToolCallPart.Input) are copied.
func (c Conversation) Fork(newID string) Conversation {
	msgs := make([]Message, len(c.Messages))
	for i, m := range c.Messages {
		content := make([]ContentPart, len(m.Content))
		for j, part := range m.Content {
			content[j] = deepCopyContentPart(part)
		}
		msgs[i] = Message{
			ID:        m.ID,
			Role:      m.Role,
			Content:   content,
			Timestamp: m.Timestamp,
			Flags:     m.Flags,
		}
	}
	blocks := make([]SystemBlock, len(c.System.Blocks))
	copy(blocks, c.System.Blocks)

	now := time.Now()
	return Conversation{
		ID:        newID,
		Messages:  msgs,
		System:    SystemPrompt{Blocks: blocks},
		Model:     c.Model,
		Provider:  c.Provider,
		WorkDir:   c.WorkDir,
		ParentID:  c.ID,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// DeepCopy creates a full deep copy of the conversation, preserving all fields.
// Unlike Fork, this does not change ID, timestamps, or parent link.
func (c Conversation) DeepCopy() Conversation {
	msgs := make([]Message, len(c.Messages))
	for i, m := range c.Messages {
		content := make([]ContentPart, len(m.Content))
		for j, part := range m.Content {
			content[j] = deepCopyContentPart(part)
		}
		msgs[i] = Message{
			ID:        m.ID,
			Role:      m.Role,
			Content:   content,
			Timestamp: m.Timestamp,
			Flags:     m.Flags,
		}
	}
	blocks := make([]SystemBlock, len(c.System.Blocks))
	copy(blocks, c.System.Blocks)

	return Conversation{
		ID:        c.ID,
		Messages:  msgs,
		System:    SystemPrompt{Blocks: blocks},
		Model:     c.Model,
		Provider:  c.Provider,
		WorkDir:   c.WorkDir,
		ParentID:  c.ParentID,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// APIMessages returns messages that should be sent to the LLM,
// filtering out internal messages.
func (c Conversation) APIMessages() []Message {
	out := make([]Message, 0, len(c.Messages))
	for _, m := range c.Messages {
		if !m.Flags.IsInternal {
			out = append(out, m)
		}
	}
	return out
}

func deepCopyContentPart(part ContentPart) ContentPart {
	switch p := part.(type) {
	case TextPart:
		return p
	case ImagePart:
		data := make([]byte, len(p.Data))
		copy(data, p.Data)
		return ImagePart{MimeType: p.MimeType, Data: data}
	case ToolCallPart:
		var input json.RawMessage
		if p.Input != nil {
			input = make(json.RawMessage, len(p.Input))
			copy(input, p.Input)
		}
		return ToolCallPart{ID: p.ID, Name: p.Name, Input: input, Signature: p.Signature}
	case DocumentPart:
		data := make([]byte, len(p.Data))
		copy(data, p.Data)
		return DocumentPart{MimeType: p.MimeType, Data: data}
	case ToolResultPart:
		return p
	case ThinkingPart:
		p.OpenRouterReasoningDetails = append(json.RawMessage(nil), p.OpenRouterReasoningDetails...)
		return p
	default:
		panic("deepCopyContentPart: unknown ContentPart type")
	}
}

// NewUUID generates a v4 UUID string.
func NewUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 2
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
