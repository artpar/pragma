package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/model"
)

func testHeader() HeaderData {
	return HeaderData{
		SessionID: "test-jsonl-001",
		Model:     "test-model",
		Provider:  "anthropic",
		WorkDir:   "/tmp/test",
		CreatedAt: time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC),
		System: model.SystemPrompt{Blocks: []model.SystemBlock{
			{Text: "You are helpful.", Cacheable: true},
		}},
	}
}

func testMessages() []model.Message {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	return []model.Message{
		{
			ID:        "msg-1",
			Role:      model.RoleUser,
			Content:   []model.ContentPart{model.TextPart{Text: "Hello"}},
			Timestamp: now,
		},
		{
			ID:   "msg-2",
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				model.TextPart{Text: "Hi there!"},
				model.ToolCallPart{ID: "tc-1", Name: "Bash", Input: json.RawMessage(`{"command":"ls"}`)},
			},
			Timestamp: now.Add(time.Second),
		},
		{
			ID:   "msg-3",
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.ToolResultPart{ToolCallID: "tc-1", Content: "file1.go"},
			},
			Timestamp: now.Add(2 * time.Second),
		},
	}
}

func TestWriter_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	w, err := NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}

	header := testHeader()
	if err := w.WriteHeader(header); err != nil {
		t.Fatal(err)
	}

	msgs := testMessages()
	for _, msg := range msgs {
		if err := w.WriteMessage(msg); err != nil {
			t.Fatal(err)
		}
	}
	meta := MetadataData{
		CostUSD:   0.005,
		TurnCount: 2,
		UpdatedAt: time.Date(2026, 4, 20, 12, 1, 0, 0, time.UTC),
		Summary:   "Hello",
	}
	if err := w.WriteMetadata(meta); err != nil {
		t.Fatal(err)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// Load via store's JSONL loader
	store := &Store{dir: dir}
	// Rename to match expected pattern
	os.Rename(path, filepath.Join(dir, "test-jsonl-001.jsonl"))

	sess, err := store.Load("test-jsonl-001")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if sess.Conversation.ID != "test-jsonl-001" {
		t.Errorf("ID = %q, want test-jsonl-001", sess.Conversation.ID)
	}
	if len(sess.Conversation.Messages) != 3 {
		t.Fatalf("Messages = %d, want 3", len(sess.Conversation.Messages))
	}
	if sess.Summary != "Hello" {
		t.Errorf("Summary = %q, want Hello", sess.Summary)
	}
	if sess.CostUSD != 0.005 {
		t.Errorf("CostUSD = %f, want 0.005", sess.CostUSD)
	}
	if sess.TurnCount != 2 {
		t.Errorf("TurnCount = %d, want 2", sess.TurnCount)
	}
	// Verify ContentPart discriminators survived
	msg2 := sess.Conversation.Messages[1]
	if len(msg2.Content) != 2 {
		t.Fatalf("msg2 content = %d, want 2", len(msg2.Content))
	}
	if _, ok := msg2.Content[0].(model.TextPart); !ok {
		t.Errorf("msg2.Content[0] = %T, want TextPart", msg2.Content[0])
	}
	if tc, ok := msg2.Content[1].(model.ToolCallPart); !ok {
		t.Errorf("msg2.Content[1] = %T, want ToolCallPart", msg2.Content[1])
	} else if tc.Name != "Bash" {
		t.Errorf("ToolCallPart.Name = %q, want Bash", tc.Name)
	}

	// Verify ToolResultPart
	msg3 := sess.Conversation.Messages[2]
	if tr, ok := msg3.Content[0].(model.ToolResultPart); !ok {
		t.Errorf("msg3.Content[0] = %T, want ToolResultPart", msg3.Content[0])
	} else if tr.ToolCallID != "tc-1" {
		t.Errorf("ToolCallID = %q, want tc-1", tr.ToolCallID)
	}

	// Verify system prompt
	if len(sess.Conversation.System.Blocks) != 1 {
		t.Fatalf("System blocks = %d, want 1", len(sess.Conversation.System.Blocks))
	}
	if sess.Conversation.System.Blocks[0].Text != "You are helpful." {
		t.Errorf("System text = %q", sess.Conversation.System.Blocks[0].Text)
	}
}

func TestWriter_MessageDedup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dedup.jsonl")

	w, err := NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}

	header := testHeader()
	w.WriteHeader(header)

	msg := testMessages()[0]
	w.WriteMessage(msg)
	w.WriteMessage(msg) // duplicate — should be skipped
	w.WriteMessage(msg) // duplicate — should be skipped
	w.Close()

	// Count message lines
	data, _ := os.ReadFile(path)
	msgCount := 0
	for _, line := range splitLines(data) {
		var entry Entry
		if json.Unmarshal(line, &entry) == nil && entry.Kind == EntryMessage {
			msgCount++
		}
	}

	if msgCount != 1 {
		t.Errorf("message count = %d, want 1 (dedup failed)", msgCount)
	}
}

func TestWriter_OpenDedup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opendedup.jsonl")

	// Write initial session
	w1, _ := NewWriter(path)
	w1.WriteHeader(testHeader())
	msgs := testMessages()
	w1.WriteMessage(msgs[0])
	w1.WriteMessage(msgs[1])
	w1.Close()

	// Reopen and try to write same messages
	w2, err := OpenWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	w2.WriteMessage(msgs[0]) // already written — should skip
	w2.WriteMessage(msgs[1]) // already written — should skip
	w2.WriteMessage(msgs[2]) // new — should write
	w2.Close()

	// Count message lines
	data, _ := os.ReadFile(path)
	msgCount := 0
	for _, line := range splitLines(data) {
		var entry Entry
		if json.Unmarshal(line, &entry) == nil && entry.Kind == EntryMessage {
			msgCount++
		}
	}

	if msgCount != 3 {
		t.Errorf("message count = %d, want 3", msgCount)
	}
}

func TestWriter_CorruptLastLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.jsonl")

	w, _ := NewWriter(path)
	w.WriteHeader(testHeader())
	w.WriteMessage(testMessages()[0])
	w.WriteMetadata(MetadataData{TurnCount: 1, Summary: "test", UpdatedAt: time.Now()})
	w.Close()

	// Append a corrupt partial line (simulating crash mid-write)
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(`{"kind":"message","data":{"id":"msg-99","role":"user","content":[{"type`)
	f.Close()

	// Load should succeed, skipping the corrupt line
	store := &Store{dir: dir}
	os.Rename(path, filepath.Join(dir, "test-jsonl-001.jsonl"))

	sess, err := store.Load("test-jsonl-001")
	if err != nil {
		t.Fatalf("Load with corrupt line: %v", err)
	}

	if len(sess.Conversation.Messages) != 1 {
		t.Errorf("Messages = %d, want 1 (corrupt line should be skipped)", len(sess.Conversation.Messages))
	}
	if sess.Summary != "test" {
		t.Errorf("Summary = %q, want test", sess.Summary)
	}
}

func TestWriter_Rewrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rewrite.jsonl")

	w, _ := NewWriter(path)
	header := testHeader()
	w.WriteHeader(header)
	msgs := testMessages()
	for _, msg := range msgs {
		w.WriteMessage(msg)
	}
	w.WriteMetadata(MetadataData{TurnCount: 2, Summary: "original"})

	// Rewrite with only the first message (simulating compaction)
	newMeta := MetadataData{TurnCount: 1, Summary: "compacted", UpdatedAt: time.Now()}
	if err := w.Rewrite(RewriteData{
		Header:   header,
		Messages: msgs[:1],
		Metadata: newMeta,
	}); err != nil {
		t.Fatal(err)
	}
	w.Close()

	store := &Store{dir: dir}
	os.Rename(path, filepath.Join(dir, "test-jsonl-001.jsonl"))

	sess, err := store.Load("test-jsonl-001")
	if err != nil {
		t.Fatal(err)
	}

	if len(sess.Conversation.Messages) != 1 {
		t.Errorf("Messages = %d, want 1 after rewrite", len(sess.Conversation.Messages))
	}
	if sess.Summary != "compacted" {
		t.Errorf("Summary = %q, want compacted", sess.Summary)
	}
}

func TestWriter_LastMetadataWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi-meta.jsonl")

	w, _ := NewWriter(path)
	w.WriteHeader(testHeader())
	w.WriteMessage(testMessages()[0])

	// Write multiple metadata entries — last should win
	w.WriteMetadata(MetadataData{TurnCount: 1, Summary: "first", CostUSD: 0.001})
	w.WriteMetadata(MetadataData{TurnCount: 2, Summary: "second", CostUSD: 0.002})
	w.WriteMetadata(MetadataData{TurnCount: 3, Summary: "third", CostUSD: 0.003})
	w.Close()

	store := &Store{dir: dir}
	os.Rename(path, filepath.Join(dir, "test-jsonl-001.jsonl"))

	sess, _ := store.Load("test-jsonl-001")
	if sess.TurnCount != 3 {
		t.Errorf("TurnCount = %d, want 3", sess.TurnCount)
	}
	if sess.Summary != "third" {
		t.Errorf("Summary = %q, want third", sess.Summary)
	}
	if sess.CostUSD != 0.003 {
		t.Errorf("CostUSD = %f, want 0.003", sess.CostUSD)
	}
}

func TestStore_DeleteJSONL(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	store, _ := NewStore()

	header := HeaderData{
		SessionID: "delete-me",
		Model:     "test-model",
		Provider:  "anthropic",
		WorkDir:   "/tmp",
		CreatedAt: time.Now(),
		System:    model.SystemPrompt{},
	}
	w, _ := store.Create(header)
	w.WriteMessage(model.Message{
		ID: "dm-1", Role: model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: "bye"}},
		Timestamp: time.Now(),
	})
	w.Close()

	if err := store.Delete("delete-me"); err != nil {
		t.Fatal(err)
	}

	_, err := store.Load("delete-me")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

// splitLines splits raw bytes into non-empty lines.
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := range data {
		if data[i] == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
