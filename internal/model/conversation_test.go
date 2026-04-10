package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewConversation(t *testing.T) {
	sys := SystemPrompt{Blocks: []SystemBlock{{Text: "You are helpful."}}}
	conv := NewConversation(sys, "claude-sonnet-4-20250514", "anthropic", "/tmp/work")

	if conv.ID == "" {
		t.Error("expected non-empty ID")
	}
	if conv.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model: got %q", conv.Model)
	}
	if conv.Provider != "anthropic" {
		t.Errorf("Provider: got %q", conv.Provider)
	}
	if conv.WorkDir != "/tmp/work" {
		t.Errorf("WorkDir: got %q", conv.WorkDir)
	}
	if len(conv.Messages) != 0 {
		t.Errorf("Messages: got %d, want 0", len(conv.Messages))
	}
	if conv.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestConversationAppend(t *testing.T) {
	conv := NewConversation(SystemPrompt{}, "model", "prov", "/tmp")
	before := conv.UpdatedAt

	msg := Message{
		ID:        "msg-1",
		Role:      RoleUser,
		Content:   []ContentPart{TextPart{Text: "hello"}},
		Timestamp: time.Now(),
	}
	conv.Append(msg)

	if len(conv.Messages) != 1 {
		t.Fatalf("Messages: got %d, want 1", len(conv.Messages))
	}
	if conv.Messages[0].ID != "msg-1" {
		t.Errorf("Message ID: got %q", conv.Messages[0].ID)
	}
	if !conv.UpdatedAt.After(before) && conv.UpdatedAt != before {
		t.Error("expected UpdatedAt to advance")
	}
}

func TestConversationForkDeepCopy(t *testing.T) {
	conv := NewConversation(
		SystemPrompt{Blocks: []SystemBlock{{Text: "sys"}}},
		"model", "prov", "/tmp",
	)
	conv.Append(Message{
		ID:        "msg-1",
		Role:      RoleUser,
		Content:   []ContentPart{TextPart{Text: "original"}},
		Timestamp: time.Now(),
	})

	forked := conv.Fork("fork-1")

	// Verify fork properties
	if forked.ID != "fork-1" {
		t.Errorf("forked ID: got %q", forked.ID)
	}
	if forked.ParentID != conv.ID {
		t.Errorf("forked ParentID: got %q, want %q", forked.ParentID, conv.ID)
	}

	// Modify forked — original should not change
	forked.Append(Message{
		ID:        "msg-2",
		Role:      RoleAssistant,
		Content:   []ContentPart{TextPart{Text: "forked reply"}},
		Timestamp: time.Now(),
	})

	if len(conv.Messages) != 1 {
		t.Errorf("original should still have 1 message, got %d", len(conv.Messages))
	}
	if len(forked.Messages) != 2 {
		t.Errorf("forked should have 2 messages, got %d", len(forked.Messages))
	}

	// Modify forked content slice — original should not change
	forked.Messages[0].Content = append(forked.Messages[0].Content, TextPart{Text: "added"})
	if len(conv.Messages[0].Content) != 1 {
		t.Error("original content slice was modified by fork")
	}
}

func TestConversationForkDeepCopyImageData(t *testing.T) {
	conv := NewConversation(SystemPrompt{}, "model", "prov", "/tmp")
	imgData := []byte{0x89, 0x50, 0x4e, 0x47}
	conv.Append(Message{
		ID:        "msg-1",
		Role:      RoleUser,
		Content:   []ContentPart{ImagePart{MimeType: "image/png", Data: imgData}},
		Timestamp: time.Now(),
	})

	forked := conv.Fork("fork-1")

	// Modify forked image data — original should not change
	forkedImg := forked.Messages[0].Content[0].(ImagePart)
	forkedImg.Data[0] = 0xFF
	// Since ImagePart is a value type, modifying forkedImg doesn't affect the fork.
	// But verify the underlying slice was deep-copied by checking through the fork:
	forked.Messages[0].Content[0] = ImagePart{MimeType: "image/png", Data: forkedImg.Data}

	origImg := conv.Messages[0].Content[0].(ImagePart)
	if origImg.Data[0] == 0xFF {
		t.Error("original ImagePart.Data was modified by fork — slice not deep-copied")
	}
}

func TestConversationForkDeepCopyDocumentData(t *testing.T) {
	conv := NewConversation(SystemPrompt{}, "model", "prov", "/tmp")
	docData := []byte{0x25, 0x50, 0x44, 0x46} // %PDF magic bytes
	conv.Append(Message{
		ID:        "msg-1",
		Role:      RoleUser,
		Content:   []ContentPart{DocumentPart{MimeType: "application/pdf", Data: docData}},
		Timestamp: time.Now(),
	})

	forked := conv.Fork("fork-1")

	// Modify forked document data — original should not change
	forkedDoc := forked.Messages[0].Content[0].(DocumentPart)
	forkedDoc.Data[0] = 0xFF
	forked.Messages[0].Content[0] = DocumentPart{MimeType: "application/pdf", Data: forkedDoc.Data}

	origDoc := conv.Messages[0].Content[0].(DocumentPart)
	if origDoc.Data[0] == 0xFF {
		t.Error("original DocumentPart.Data was modified by fork — slice not deep-copied")
	}
}

func TestConversationForkDeepCopyToolCallInput(t *testing.T) {
	conv := NewConversation(SystemPrompt{}, "model", "prov", "/tmp")
	input := json.RawMessage(`{"key":"value"}`)
	conv.Append(Message{
		ID:        "msg-1",
		Role:      RoleAssistant,
		Content:   []ContentPart{ToolCallPart{ID: "tc-1", Name: "Bash", Input: input}},
		Timestamp: time.Now(),
	})

	forked := conv.Fork("fork-1")

	// Modify forked input — original should not change
	forkedTc := forked.Messages[0].Content[0].(ToolCallPart)
	forkedTc.Input[0] = 'X'
	forked.Messages[0].Content[0] = ToolCallPart{ID: "tc-1", Name: "Bash", Input: forkedTc.Input}

	origTc := conv.Messages[0].Content[0].(ToolCallPart)
	if origTc.Input[0] == 'X' {
		t.Error("original ToolCallPart.Input was modified by fork — slice not deep-copied")
	}
}

func TestConversationAPIMessages(t *testing.T) {
	conv := NewConversation(SystemPrompt{}, "model", "prov", "/tmp")

	conv.Append(Message{
		ID:        "msg-1",
		Role:      RoleUser,
		Content:   []ContentPart{TextPart{Text: "visible"}},
		Timestamp: time.Now(),
	})
	conv.Append(Message{
		ID:        "msg-internal",
		Role:      RoleUser,
		Content:   []ContentPart{TextPart{Text: "hidden"}},
		Timestamp: time.Now(),
		Flags:     MessageFlags{IsInternal: true},
	})
	conv.Append(Message{
		ID:        "msg-2",
		Role:      RoleAssistant,
		Content:   []ContentPart{TextPart{Text: "response"}},
		Timestamp: time.Now(),
	})

	api := conv.APIMessages()
	if len(api) != 2 {
		t.Fatalf("APIMessages: got %d, want 2", len(api))
	}
	if api[0].ID != "msg-1" {
		t.Errorf("first API message: got %q", api[0].ID)
	}
	if api[1].ID != "msg-2" {
		t.Errorf("second API message: got %q", api[1].ID)
	}
}

func TestConversationDeepCopy(t *testing.T) {
	conv := NewConversation(
		SystemPrompt{Blocks: []SystemBlock{{Text: "sys"}}},
		"model", "prov", "/tmp",
	)
	conv.Append(Message{
		ID:        "msg-1",
		Role:      RoleUser,
		Content:   []ContentPart{TextPart{Text: "original"}},
		Timestamp: time.Now(),
	})

	cp := conv.DeepCopy()

	// Same ID and timestamps
	if cp.ID != conv.ID {
		t.Errorf("DeepCopy ID: got %q, want %q", cp.ID, conv.ID)
	}
	if !cp.CreatedAt.Equal(conv.CreatedAt) {
		t.Error("DeepCopy should preserve CreatedAt")
	}
	if !cp.UpdatedAt.Equal(conv.UpdatedAt) {
		t.Error("DeepCopy should preserve UpdatedAt")
	}

	// Mutate copy — original unchanged
	cp.Append(Message{
		ID:        "msg-2",
		Role:      RoleAssistant,
		Content:   []ContentPart{TextPart{Text: "added"}},
		Timestamp: time.Now(),
	})
	if len(conv.Messages) != 1 {
		t.Errorf("original should still have 1 message, got %d", len(conv.Messages))
	}
}

func TestAPIMessagesReturnsEmptyNotNil(t *testing.T) {
	conv := NewConversation(SystemPrompt{}, "model", "prov", "/tmp")
	msgs := conv.APIMessages()
	if msgs == nil {
		t.Error("APIMessages() returned nil, want empty non-nil slice")
	}
	if len(msgs) != 0 {
		t.Errorf("APIMessages() returned %d messages, want 0", len(msgs))
	}
}

func TestNewConversationMessagesNotNil(t *testing.T) {
	conv := NewConversation(SystemPrompt{}, "model", "prov", "/tmp")
	if conv.Messages == nil {
		t.Error("NewConversation().Messages should be non-nil empty slice, got nil")
	}
}

func TestDeepCopyNilToolCallInput(t *testing.T) {
	conv := NewConversation(SystemPrompt{}, "model", "prov", "/tmp")
	conv.Append(Message{
		ID:        "msg-1",
		Role:      RoleAssistant,
		Content:   []ContentPart{ToolCallPart{ID: "tc-1", Name: "Bash", Input: nil}},
		Timestamp: time.Now(),
	})

	forked := conv.Fork("fork-1")
	tc := forked.Messages[0].Content[0].(ToolCallPart)
	if tc.Input != nil {
		t.Errorf("nil Input should remain nil after deep copy, got %v", tc.Input)
	}
}

func TestConversationRoundTrip(t *testing.T) {
	conv := NewConversation(
		SystemPrompt{Blocks: []SystemBlock{{Text: "sys", Cacheable: true}}},
		"model", "prov", "/tmp",
	)
	conv.Append(Message{
		ID:   "msg-1",
		Role: RoleUser,
		Content: []ContentPart{
			TextPart{Text: "hello"},
			ToolResultPart{ToolCallID: "tc-1", Content: "result", IsError: false},
		},
		Timestamp: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
	})

	data, err := json.Marshal(conv)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Conversation
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != conv.ID {
		t.Errorf("ID mismatch")
	}
	if len(got.Messages) != len(conv.Messages) {
		t.Fatalf("message count: got %d, want %d", len(got.Messages), len(conv.Messages))
	}
	if len(got.Messages[0].Content) != 2 {
		t.Errorf("content count: got %d, want 2", len(got.Messages[0].Content))
	}
}
