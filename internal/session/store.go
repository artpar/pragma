package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/artpar/gogent/internal/config"
	"github.com/artpar/gogent/internal/model"
)

// Store persists sessions to ~/.gogent/sessions/.
type Store struct {
	dir string
}

// NewStore creates a Store, creating the sessions directory if needed.
func NewStore() (*Store, error) {
	dir, err := config.SessionsDir()
	if err != nil {
		return nil, fmt.Errorf("resolve sessions directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create sessions directory: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Save writes a session to <dir>/<id>.json.
// Validates the session before writing (strips empty text blocks).
// Uses atomic write (temp file + rename).
func (s *Store) Save(sess Session) error {
	if len(sess.Conversation.Messages) == 0 {
		return errors.New("session has no messages")
	}

	// Validate: strip empty text blocks from all messages
	sanitizeConversation(&sess.Conversation)

	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	path := s.sessionPath(sess.Conversation.ID)
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write temp session file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) // clean up on rename failure
		return fmt.Errorf("rename session file: %w", err)
	}

	return nil
}

// Load reads a session by conversation ID.
func (s *Store) Load(id string) (Session, error) {
	path := s.sessionPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Session{}, fmt.Errorf("session %q not found: %w", id, err)
		}
		return Session{}, fmt.Errorf("read session %q: %w", id, err)
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return Session{}, fmt.Errorf("parse session %q: %w", id, err)
	}

	// Validate on read: strip any empty text blocks that slipped through
	sanitizeConversation(&sess.Conversation)

	return sess, nil
}

// List returns all sessions, sorted by UpdatedAt descending (newest first).
func (s *Store) List() ([]SessionSummary, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions directory: %w", err)
	}

	var summaries []SessionSummary
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(s.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue // skip unreadable files
		}

		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			continue // skip corrupt files
		}

		summaries = append(summaries, summaryFromSession(sess))
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})

	return summaries, nil
}

// Delete removes a session file.
func (s *Store) Delete(id string) error {
	path := s.sessionPath(id)
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete session %q: %w", id, err)
	}
	return nil
}

func (s *Store) sessionPath(id string) string {
	return filepath.Join(s.dir, id+".json")
}

// sanitizeConversation strips empty text blocks from all messages.
// This prevents session corruption from streaming artifacts (GitHub issue #41992).
func sanitizeConversation(conv *model.Conversation) {
	for i := range conv.Messages {
		msg := &conv.Messages[i]
		filtered := msg.Content[:0] // reuse underlying array
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok && tp.Text == "" {
				continue // skip empty text blocks
			}
			filtered = append(filtered, part)
		}
		msg.Content = filtered
	}
}
