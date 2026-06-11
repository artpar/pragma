package session

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
)

// Writer appends JSONL entries to a session file.
// Thread-safe via mutex. Syncs to disk after every write.
type Writer struct {
	file    *os.File
	encoder *json.Encoder
	mu      sync.Mutex
	written map[string]bool // message IDs already written (dedup)
	closed  bool
}

type RewriteData struct {
	Header                 HeaderData
	Messages               []model.Message
	Metadata               MetadataData
	ContentReplacements    []model.ContentReplacementRecord
	PromptHistory          []PromptHistoryData
	OrchestrationArtifacts []app.OrchestrationArtifact
	TaskResults            []TaskResultData
}

// NewWriter creates a new session JSONL file at path.
func NewWriter(path string) (*Writer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("create session file: %w", err)
	}
	return &Writer{
		file:    f,
		encoder: json.NewEncoder(f),
		written: make(map[string]bool),
	}, nil
}

// OpenWriter opens an existing session JSONL file for appending.
// Scans the file for existing message IDs to enable dedup.
func OpenWriter(path string) (*Writer, error) {
	written, err := scanMessageIDs(path)
	if err != nil {
		return nil, fmt.Errorf("scan session file: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open session file for append: %w", err)
	}
	return &Writer{
		file:    f,
		encoder: json.NewEncoder(f),
		written: written,
	}, nil
}

// scanMessageIDs reads a JSONL file and extracts all message IDs for dedup.
func scanMessageIDs(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ids := make(map[string]bool)
	scanner := newJSONLScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // skip corrupt lines
		}
		if entry.Kind != EntryMessage {
			continue
		}
		// Extract just the ID field without full message deserialization.
		var idOnly struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(entry.Data, &idOnly); err == nil && idOnly.ID != "" {
			ids[idOnly.ID] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

// WriteHeader writes a header entry to the file.
func (w *Writer) WriteHeader(h HeaderData) error {
	return w.writeEntry(EntryHeader, h)
}

// WriteMessage writes a message entry, deduplicating by message ID.
// Returns nil without writing if the message ID was already written.
func (w *Writer) WriteMessage(msg model.Message) error {
	w.mu.Lock()
	if w.written[msg.ID] {
		w.mu.Unlock()
		return nil
	}
	w.mu.Unlock()
	if err := w.writeEntry(EntryMessage, msg); err != nil {
		return err
	}
	w.mu.Lock()
	w.written[msg.ID] = true
	w.mu.Unlock()
	return nil
}

// WriteMetadata writes a metadata entry with current session counters.
func (w *Writer) WriteMetadata(m MetadataData) error {
	return w.writeEntry(EntryMetadata, m)
}

func (w *Writer) WriteContentReplacement(records []model.ContentReplacementRecord) error {
	if len(records) == 0 {
		return nil
	}
	return w.writeEntry(EntryContentReplacement, ContentReplacementData{Records: records})
}

func (w *Writer) WritePromptHistory(text string) error {
	if text == "" {
		return nil
	}
	return w.writeEntry(EntryPromptHistory, PromptHistoryData{
		Text:      text,
		Timestamp: time.Now(),
	})
}

func (w *Writer) WriteOrchestrationArtifacts(artifacts []app.OrchestrationArtifact) error {
	if len(artifacts) == 0 {
		return nil
	}
	return w.writeEntry(EntryOrchestrationArtifacts, OrchestrationArtifactsData{Artifacts: artifacts})
}

func (w *Writer) WriteTaskResult(result TaskResultData) error {
	if result.TaskID == "" {
		return nil
	}
	return w.writeEntry(EntryTaskResult, result)
}

func (w *Writer) writeEntry(kind EntryKind, data any) error {
	entry, err := MarshalEntry(kind, data)
	if err != nil {
		return fmt.Errorf("marshal %s entry: %w", kind, err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return fmt.Errorf("writer is closed")
	}
	if err := w.encoder.Encode(entry); err != nil {
		return fmt.Errorf("encode %s entry: %w", kind, err)
	}
	return w.file.Sync()
}

// Rewrite truncates the file and writes a complete durable session snapshot.
// Used after compaction to replace conversation content without dropping other
// session entry categories that Store.Load owns.
func (w *Writer) Rewrite(data RewriteData) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return fmt.Errorf("writer is closed")
	}

	// Truncate and seek to beginning
	if err := w.file.Truncate(0); err != nil {
		return fmt.Errorf("truncate session file: %w", err)
	}
	if _, err := w.file.Seek(0, 0); err != nil {
		return fmt.Errorf("seek session file: %w", err)
	}

	// Reset encoder (it may have buffered state)
	w.encoder = json.NewEncoder(w.file)

	// Reset dedup map
	w.written = make(map[string]bool, len(data.Messages))

	// Write header
	headerEntry, err := MarshalEntry(EntryHeader, data.Header)
	if err != nil {
		return fmt.Errorf("marshal header for rewrite: %w", err)
	}
	if err := w.encoder.Encode(headerEntry); err != nil {
		return fmt.Errorf("encode header for rewrite: %w", err)
	}

	// Write messages
	for _, msg := range data.Messages {
		msgEntry, err := MarshalEntry(EntryMessage, msg)
		if err != nil {
			return fmt.Errorf("marshal message for rewrite: %w", err)
		}
		if err := w.encoder.Encode(msgEntry); err != nil {
			return fmt.Errorf("encode message for rewrite: %w", err)
		}
		w.written[msg.ID] = true
	}

	if len(data.ContentReplacements) > 0 {
		if err := w.encodeRewriteEntry(EntryContentReplacement, ContentReplacementData{Records: data.ContentReplacements}); err != nil {
			return err
		}
	}
	for _, prompt := range data.PromptHistory {
		if err := w.encodeRewriteEntry(EntryPromptHistory, prompt); err != nil {
			return err
		}
	}
	if len(data.OrchestrationArtifacts) > 0 {
		if err := w.encodeRewriteEntry(EntryOrchestrationArtifacts, OrchestrationArtifactsData{Artifacts: data.OrchestrationArtifacts}); err != nil {
			return err
		}
	}
	for _, result := range data.TaskResults {
		if result.TaskID == "" {
			continue
		}
		if err := w.encodeRewriteEntry(EntryTaskResult, result); err != nil {
			return err
		}
	}

	// Write metadata
	metaEntry, err := MarshalEntry(EntryMetadata, data.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata for rewrite: %w", err)
	}
	if err := w.encoder.Encode(metaEntry); err != nil {
		return fmt.Errorf("encode metadata for rewrite: %w", err)
	}

	return w.file.Sync()
}

func (w *Writer) encodeRewriteEntry(kind EntryKind, data any) error {
	entry, err := MarshalEntry(kind, data)
	if err != nil {
		return fmt.Errorf("marshal %s for rewrite: %w", kind, err)
	}
	if err := w.encoder.Encode(entry); err != nil {
		return fmt.Errorf("encode %s for rewrite: %w", kind, err)
	}
	return nil
}

func (w *Writer) Size() (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, fmt.Errorf("writer is closed")
	}
	info, err := w.file.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// Close syncs and closes the underlying file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	syncErr := w.file.Sync()
	closeErr := w.file.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
