package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/tool"
)

// Store persists sessions to ~/.pragma/sessions/.
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

// Create opens a new session JSONL file and writes the header entry.
// Returns a Writer for appending messages.
func (s *Store) Create(header HeaderData) (*Writer, error) {
	if !IsValidSessionID(header.SessionID) {
		return nil, fmt.Errorf("invalid session ID %q", header.SessionID)
	}
	path := filepath.Join(s.dir, header.SessionID+".jsonl")
	w, err := NewWriter(path)
	if err != nil {
		return nil, err
	}
	if err := w.WriteHeader(header); err != nil {
		w.Close()
		return nil, fmt.Errorf("write header: %w", err)
	}
	return w, nil
}

// Open opens an existing session JSONL file for appending.
// Reads existing message IDs into the dedup set.
func (s *Store) Open(id string) (*Writer, error) {
	if !IsValidSessionID(id) {
		return nil, fmt.Errorf("invalid session ID %q", id)
	}
	path := filepath.Join(s.dir, id+".jsonl")
	return OpenWriter(path)
}

// Load reads a session by conversation ID from its JSONL file.
func (s *Store) Load(id string) (Session, error) {
	if !IsValidSessionID(id) {
		return Session{}, fmt.Errorf("invalid session ID %q: must contain only alphanumeric characters and hyphens", id)
	}

	jsonlPath := filepath.Join(s.dir, id+".jsonl")
	return s.loadJSONL(jsonlPath)
}

func (s *Store) loadJSONL(path string) (Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, fmt.Errorf("open session: %w", err)
	}
	defer f.Close()

	var header HeaderData
	var messages []model.Message
	var meta MetadataData
	var handoffState model.HandoffState
	var replacements []model.ContentReplacementRecord
	var promptHistory []PromptHistoryData
	var fileStateRecords []tool.FileStateRecord
	var todos []app.TodoItem
	hasHeader := false

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 64<<20) // file_state entries can include cached file contents
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // skip corrupt/partial lines
		}

		switch entry.Kind {
		case EntryHeader:
			if err := json.Unmarshal(entry.Data, &header); err == nil {
				hasHeader = true
			}
		case EntryMessage:
			var msg model.Message
			if err := json.Unmarshal(entry.Data, &msg); err == nil {
				messages = append(messages, msg)
			}
		case EntryMetadata:
			json.Unmarshal(entry.Data, &meta) // last one wins
		case EntryHandoffState:
			var data HandoffStateData
			if err := json.Unmarshal(entry.Data, &data); err == nil {
				handoffState = data.State
			}
		case EntryContentReplacement:
			var data ContentReplacementData
			if err := json.Unmarshal(entry.Data, &data); err == nil {
				replacements = append(replacements, data.Records...)
			}
		case EntryPromptHistory:
			var data PromptHistoryData
			if err := json.Unmarshal(entry.Data, &data); err == nil {
				promptHistory = append(promptHistory, data)
			}
		case EntryFileState:
			var data FileStateData
			if err := json.Unmarshal(entry.Data, &data); err == nil {
				fileStateRecords = data.Records
			}
		case EntryTodos:
			var data TodosData
			if err := json.Unmarshal(entry.Data, &data); err == nil {
				todos = data.Items
			}
		}
	}

	if !hasHeader {
		return Session{}, fmt.Errorf("session file has no header: %s", path)
	}

	conv := model.Conversation{
		ID:        header.SessionID,
		Messages:  messages,
		System:    header.System,
		Model:     firstNonEmpty(meta.Model, header.Model),
		Provider:  firstNonEmpty(meta.Provider, header.Provider),
		WorkDir:   header.WorkDir,
		CreatedAt: header.CreatedAt,
		UpdatedAt: meta.UpdatedAt,
	}
	sanitizeConversation(&conv)

	return Session{
		Conversation:        conv,
		HandoffState:        handoffState,
		Summary:             meta.Summary,
		CostUSD:             meta.CostUSD,
		TurnCount:           meta.TurnCount,
		TokenUsage:          meta.TokenUsage,
		SystemOverride:      header.SystemOverride,
		GitRemote:           header.GitRemote,
		ContentReplacements: replacements,
		PromptHistory:       promptHistory,
		FileStateRecords:    fileStateRecords,
		Todos:               todos,
	}, nil
}

// List returns all sessions, sorted by UpdatedAt descending (newest first).
// Reads only the first line (header) of each JSONL file for fast listing.
func (s *Store) List() ([]SessionSummary, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions directory: %w", err)
	}

	type fileInfo struct {
		path string
		info os.FileInfo
	}
	var files []fileInfo

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".jsonl" {
			continue
		}
		if strings.HasSuffix(name, ".tmp") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{path: filepath.Join(s.dir, name), info: info})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].info.ModTime().After(files[j].info.ModTime())
	})

	var summaries []SessionSummary
	for _, f := range files {
		summary, err := s.readJSONLSummary(f.path, f.info)
		if err != nil {
			continue
		}
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// readJSONLSummary reads only the first line (header) of a JSONL session file.
func (s *Store) readJSONLSummary(path string, info os.FileInfo) (SessionSummary, error) {
	f, err := os.Open(path)
	if err != nil {
		return SessionSummary{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	if !scanner.Scan() {
		return SessionSummary{}, errors.New("empty file")
	}

	var entry Entry
	if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
		return SessionSummary{}, err
	}
	if entry.Kind != EntryHeader {
		return SessionSummary{}, errors.New("first line is not header")
	}
	var header HeaderData
	if err := json.Unmarshal(entry.Data, &header); err != nil {
		return SessionSummary{}, err
	}

	// Scan for the last metadata entry to get summary, cost, turns
	var meta MetadataData
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		if e.Kind == EntryMetadata {
			json.Unmarshal(e.Data, &meta) // last one wins
		}
	}

	summary := meta.Summary
	updatedAt := meta.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = info.ModTime()
	}

	return SessionSummary{
		ID:        header.SessionID,
		Summary:   summary,
		Model:     firstNonEmpty(meta.Model, header.Model),
		Provider:  firstNonEmpty(meta.Provider, header.Provider),
		WorkDir:   header.WorkDir,
		TurnCount: meta.TurnCount,
		CostUSD:   meta.CostUSD,
		CreatedAt: header.CreatedAt,
		UpdatedAt: updatedAt,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// Delete removes a session JSONL file.
func (s *Store) Delete(id string) error {
	if !IsValidSessionID(id) {
		return fmt.Errorf("invalid session ID %q", id)
	}
	path := filepath.Join(s.dir, id+".jsonl")
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete session %q: %w", id, err)
	}
	return nil
}

// IsValidSessionID checks that the ID contains only safe characters
// to prevent path traversal attacks via crafted --resume values.
func IsValidSessionID(id string) bool {
	if id == "" {
		return false
	}
	for _, ch := range id {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-') {
			return false
		}
	}
	return true
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
