package session

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/model"
)

func testConversation() model.Conversation {
	now := time.Now()
	return model.Conversation{
		ID: "test-session-001",
		Messages: []model.Message{
			{
				ID:        "msg-1",
				Role:      model.RoleUser,
				Content:   []model.ContentPart{model.TextPart{Text: "Hello, world"}},
				Timestamp: now,
			},
			{
				ID:   "msg-2",
				Role: model.RoleAssistant,
				Content: []model.ContentPart{
					model.TextPart{Text: "Hi! How can I help?"},
					model.ToolCallPart{ID: "tc-1", Name: "Bash", Input: json.RawMessage(`{"command":"ls"}`)},
				},
				Timestamp: now.Add(time.Second),
			},
			{
				ID:   "msg-3",
				Role: model.RoleUser,
				Content: []model.ContentPart{
					model.ToolResultPart{ToolCallID: "tc-1", Content: "file1.go\nfile2.go"},
				},
				Timestamp: now.Add(2 * time.Second),
			},
		},
		System: model.SystemPrompt{Blocks: []model.SystemBlock{
			{Text: "You are helpful.", Cacheable: true},
		}},
		Model:     "test-model",
		Provider:  "anthropic",
		WorkDir:   "/tmp/test",
		CreatedAt: now,
		UpdatedAt: now.Add(2 * time.Second),
	}
}

// createTestSession creates a JSONL session via Create + WriteMessage + WriteMetadata.
func createTestSession(t *testing.T, store *Store, conv model.Conversation, summary string, cost float64, turnCount int) {
	t.Helper()
	w, err := store.Create(HeaderData{
		SessionID: conv.ID,
		Model:     conv.Model,
		Provider:  conv.Provider,
		WorkDir:   conv.WorkDir,
		CreatedAt: conv.CreatedAt,
		System:    conv.System,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, msg := range conv.Messages {
		if err := w.WriteMessage(msg); err != nil {
			t.Fatalf("WriteMessage: %v", err)
		}
	}
	if err := w.WriteMetadata(MetadataData{
		Summary:   summary,
		CostUSD:   cost,
		TurnCount: turnCount,
		UpdatedAt: conv.UpdatedAt,
	}); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}
	w.Close()
}

func TestCreateLoad_ContentReplacements(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	conv := testConversation()
	w, err := store.Create(HeaderData{
		SessionID: conv.ID,
		Model:     conv.Model,
		Provider:  conv.Provider,
		WorkDir:   conv.WorkDir,
		CreatedAt: conv.CreatedAt,
		System:    conv.System,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	records := []model.ContentReplacementRecord{{
		Kind:        model.ContentReplacementKindToolResult,
		ToolUseID:   "toolu-1",
		Replacement: "<persisted-output>\npreview\n</persisted-output>",
	}}
	if err := w.WriteContentReplacement(records); err != nil {
		t.Fatalf("WriteContentReplacement: %v", err)
	}
	w.Close()

	loaded, err := store.Load(conv.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.ContentReplacements) != 1 {
		t.Fatalf("ContentReplacements len = %d, want 1", len(loaded.ContentReplacements))
	}
	if loaded.ContentReplacements[0] != records[0] {
		t.Fatalf("ContentReplacements[0] = %#v, want %#v", loaded.ContentReplacements[0], records[0])
	}
}

func TestCreateLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	conv := testConversation()
	createTestSession(t, store, conv, "Hello, world", 0.0123, 2)

	loaded, err := store.Load("test-session-001")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Summary != "Hello, world" {
		t.Errorf("Summary = %q, want %q", loaded.Summary, "Hello, world")
	}
	if loaded.CostUSD != 0.0123 {
		t.Errorf("CostUSD = %f, want %f", loaded.CostUSD, 0.0123)
	}
	if loaded.TurnCount != 2 {
		t.Errorf("TurnCount = %d, want %d", loaded.TurnCount, 2)
	}

	// Verify conversation
	if loaded.Conversation.ID != conv.ID {
		t.Errorf("Conversation.ID = %q, want %q", loaded.Conversation.ID, conv.ID)
	}
	if len(loaded.Conversation.Messages) != len(conv.Messages) {
		t.Fatalf("Messages count = %d, want %d", len(loaded.Conversation.Messages), len(conv.Messages))
	}

	// Verify ContentPart discriminators survived round-trip
	msg2 := loaded.Conversation.Messages[1]
	if len(msg2.Content) != 2 {
		t.Fatalf("msg2 content count = %d, want 2", len(msg2.Content))
	}
	if _, ok := msg2.Content[0].(model.TextPart); !ok {
		t.Errorf("msg2.Content[0] type = %T, want TextPart", msg2.Content[0])
	}
	if tc, ok := msg2.Content[1].(model.ToolCallPart); !ok {
		t.Errorf("msg2.Content[1] type = %T, want ToolCallPart", msg2.Content[1])
	} else if tc.Name != "Bash" {
		t.Errorf("ToolCallPart.Name = %q, want Bash", tc.Name)
	}

	// Verify ToolResultPart survived
	msg3 := loaded.Conversation.Messages[2]
	if tr, ok := msg3.Content[0].(model.ToolResultPart); !ok {
		t.Errorf("msg3.Content[0] type = %T, want ToolResultPart", msg3.Content[0])
	} else if tr.ToolCallID != "tc-1" {
		t.Errorf("ToolResultPart.ToolCallID = %q, want tc-1", tr.ToolCallID)
	}

	// Verify system prompt survived
	if len(loaded.Conversation.System.Blocks) != 1 {
		t.Fatalf("System.Blocks count = %d, want 1", len(loaded.Conversation.System.Blocks))
	}
	if loaded.Conversation.System.Blocks[0].Text != "You are helpful." {
		t.Errorf("System block text = %q", loaded.Conversation.System.Blocks[0].Text)
	}
}

func TestLoad_EmptyTextBlockStripping(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	conv := testConversation()
	// Inject empty text block (the bug from GitHub #41992)
	conv.Messages[1].Content = append(conv.Messages[1].Content, model.TextPart{Text: ""})

	createTestSession(t, store, conv, "", 0, 1)

	loaded, err := store.Load("test-session-001")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// The empty TextPart should have been stripped
	msg2 := loaded.Conversation.Messages[1]
	for i, part := range msg2.Content {
		if tp, ok := part.(model.TextPart); ok && tp.Text == "" {
			t.Errorf("msg2.Content[%d] is empty TextPart — should have been stripped", i)
		}
	}
}

func TestList_SortOrder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	for i, id := range []string{"old", "mid", "new"} {
		conv := testConversation()
		conv.ID = id
		conv.UpdatedAt = now.Add(time.Duration(i) * time.Hour)
		createTestSession(t, store, conv, id, 0, 1)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(list) != 3 {
		t.Fatalf("List count = %d, want 3", len(list))
	}

	// Newest first (by mtime; all created in sequence so last created is newest)
	if list[0].ID != "new" {
		t.Errorf("list[0].ID = %q, want new", list[0].ID)
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	conv := testConversation()
	createTestSession(t, store, conv, "", 0, 1)

	if err := store.Delete("test-session-001"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = store.Load("test-session-001")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestLoad_NotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Load("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}
