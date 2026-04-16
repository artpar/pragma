package session

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	sess := Session{
		Conversation:   testConversation(),
		Summary:        "Hello, world",
		CostUSD:        0.0123,
		TurnCount:      2,
		SystemOverride: "",
		GitRemote:      "git@github.com:user/repo.git",
	}

	if err := store.Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load("test-session-001")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Verify top-level fields
	if loaded.Summary != sess.Summary {
		t.Errorf("Summary = %q, want %q", loaded.Summary, sess.Summary)
	}
	if loaded.CostUSD != sess.CostUSD {
		t.Errorf("CostUSD = %f, want %f", loaded.CostUSD, sess.CostUSD)
	}
	if loaded.TurnCount != sess.TurnCount {
		t.Errorf("TurnCount = %d, want %d", loaded.TurnCount, sess.TurnCount)
	}
	if loaded.GitRemote != sess.GitRemote {
		t.Errorf("GitRemote = %q, want %q", loaded.GitRemote, sess.GitRemote)
	}

	// Verify conversation
	if loaded.Conversation.ID != sess.Conversation.ID {
		t.Errorf("Conversation.ID = %q, want %q", loaded.Conversation.ID, sess.Conversation.ID)
	}
	if len(loaded.Conversation.Messages) != len(sess.Conversation.Messages) {
		t.Fatalf("Messages count = %d, want %d", len(loaded.Conversation.Messages), len(sess.Conversation.Messages))
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

func TestSave_EmptyTextBlockStripping(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	conv := testConversation()
	// Inject empty text block (the bug from GitHub #41992)
	conv.Messages[1].Content = append(conv.Messages[1].Content, model.TextPart{Text: ""})

	sess := Session{Conversation: conv, TurnCount: 1}
	if err := store.Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

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

func TestSave_NoMessages(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	sess := Session{
		Conversation: model.Conversation{ID: "empty", Messages: []model.Message{}},
	}
	err = store.Save(sess)
	if err == nil {
		t.Fatal("expected error for session with no messages")
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
		sess := Session{Conversation: conv, Summary: id, TurnCount: 1}
		if err := store.Save(sess); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(list) != 3 {
		t.Fatalf("List count = %d, want 3", len(list))
	}

	// Newest first
	if list[0].ID != "new" {
		t.Errorf("list[0].ID = %q, want new", list[0].ID)
	}
	if list[1].ID != "mid" {
		t.Errorf("list[1].ID = %q, want mid", list[1].ID)
	}
	if list[2].ID != "old" {
		t.Errorf("list[2].ID = %q, want old", list[2].ID)
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	sess := Session{Conversation: testConversation(), TurnCount: 1}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

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

func TestSave_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	sess := Session{Conversation: testConversation(), TurnCount: 1}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	// No .tmp file should remain
	entries, _ := os.ReadDir(store.dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestByteStableRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	sess := Session{Conversation: testConversation(), TurnCount: 1}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	// Read raw bytes
	path := filepath.Join(store.dir, "test-session-001.json")
	data1, _ := os.ReadFile(path)

	// Load and save again
	loaded, _ := store.Load("test-session-001")
	if err := store.Save(loaded); err != nil {
		t.Fatal(err)
	}

	data2, _ := os.ReadFile(path)

	// Bytes should be identical (no whitespace normalization)
	if string(data1) != string(data2) {
		t.Error("save/load/save produced different bytes — not byte-stable")
	}
}
