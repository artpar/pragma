package tool

import (
	"errors"
	"github.com/artpar/pragma/internal/observe"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	defaultFileStateEntries = 100
	defaultFileStateBytes   = 25 * 1024 * 1024
)

var (
	ErrFileNotRead   = errors.New("File has not been read yet. Read it first before writing to it.")
	ErrFileModified  = errors.New("File has been unexpectedly modified. Read it again before attempting to write it.")
	ErrFileReadState = errors.New("File read state is unavailable. Read it first before writing to it.")
)

// FileState records the file contents and mtime observed by the Read tool.
type FileState struct {
	Content       string
	Timestamp     int64
	Offset        *int
	Limit         *int
	IsPartialView bool
}

// FileStateProvider is implemented by StateSnapshot wrappers that expose the
// conversation's Read/Edit/Write state.
type FileStateProvider interface {
	ReadFileState() *FileStateCache
}

// FileStateCache is a small concurrency-safe LRU cache keyed by normalized path.
type FileStateCache struct {
	mu       sync.Mutex
	entries  map[string]fileStateEntry
	order    []string
	maxItems int
	maxBytes int
	bytes    int
	effects  []FileEffect
}

type fileStateEntry struct {
	state FileState
	size  int
}

type FileEffect struct {
	Path      string
	Operation string
}

type FileStateRecord struct {
	Path          string `json:"path"`
	Content       string `json:"content"`
	Timestamp     int64  `json:"timestamp"`
	Offset        *int   `json:"offset,omitempty"`
	Limit         *int   `json:"limit,omitempty"`
	IsPartialView bool   `json:"is_partial_view,omitempty"`
}

func NewFileStateCache() *FileStateCache {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &FileStateCache{\n\tentries:\tmake(map[string]fileStateEntry),\n\torder:\t\tmake([]s...")
	return &FileStateCache{
		entries:  make(map[string]fileStateEntry),
		order:    make([]string, 0, defaultFileStateEntries),
		maxItems: defaultFileStateEntries,
		maxBytes: defaultFileStateBytes,
	}
}

func (c *FileStateCache) Snapshot() []FileStateRecord {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]FileStateRecord, 0, len(c.order))
	for _, key := range c.order {
		entry, ok := c.entries[key]
		if !ok {
			continue
		}
		state := cloneFileState(entry.state)
		out = append(out, FileStateRecord{
			Path:          key,
			Content:       state.Content,
			Timestamp:     state.Timestamp,
			Offset:        state.Offset,
			Limit:         state.Limit,
			IsPartialView: state.IsPartialView,
		})
	}
	return out
}

func (c *FileStateCache) Restore(records []FileStateRecord) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]fileStateEntry, len(records))
	c.order = make([]string, 0, len(records))
	c.bytes = 0
	for _, record := range records {
		if record.Path == "" {
			continue
		}
		key := NormalizeFilePath(record.Path)
		state := FileState{
			Content:       record.Content,
			Timestamp:     record.Timestamp,
			Offset:        cloneIntPtr(record.Offset),
			Limit:         cloneIntPtr(record.Limit),
			IsPartialView: record.IsPartialView,
		}
		size := len([]byte(state.Content))
		if old, ok := c.entries[key]; ok {
			c.bytes -= old.size
			c.removeOrderLocked(key)
		}
		c.entries[key] = fileStateEntry{state: state, size: size}
		c.order = append(c.order, key)
		c.bytes += size
		c.evictLocked()
	}
}

func FileStateCacheFrom(state StateSnapshot) (*FileStateCache, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	provider, ok := state.(FileStateProvider)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: nil, false")
		return nil, false
	}
	cache := provider.ReadFileState()
	observe.GlobalTrace("return: cache, cache != nil")
	return cache, cache != nil
}

func (c *FileStateCache) Get(path string) (FileState, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if c == nil {
		observe.GlobalTrace("if: c == nil")
		observe.GlobalTrace("return: FileState{}, false")
		return FileState{}, false
	}
	key := NormalizeFilePath(path)

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: FileState{}, false")
		return FileState{}, false
	}
	c.touchLocked(key)
	observe.GlobalTrace("return: cloneFileState(entry.state), true")
	return cloneFileState(entry.state), true
}

func (c *FileStateCache) Set(path string, state FileState) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if c == nil {
		observe.GlobalTrace("if: c == nil")
		return
	}
	key := NormalizeFilePath(path)
	state = cloneFileState(state)
	size := len([]byte(state.Content))

	c.mu.Lock()
	defer c.mu.Unlock()

	if old, ok := c.entries[key]; ok {
		observe.GlobalTrace("if: ok")
		c.bytes -= old.size
		c.removeOrderLocked(key)
	}
	c.entries[key] = fileStateEntry{state: state, size: size}
	c.order = append(c.order, key)
	c.bytes += size
	c.evictLocked()
}

func (c *FileStateCache) Delete(path string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if c == nil {
		observe.GlobalTrace("if: c == nil")
		return
	}
	key := NormalizeFilePath(path)

	c.mu.Lock()
	defer c.mu.Unlock()

	if old, ok := c.entries[key]; ok {
		observe.GlobalTrace("if: ok")
		delete(c.entries, key)
		c.bytes -= old.size
		c.removeOrderLocked(key)
	}
}

func (c *FileStateCache) EffectCursor() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.effects)
}

func (c *FileStateCache) EffectsSince(cursor int) []FileEffect {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(c.effects) {
		return nil
	}
	out := make([]FileEffect, len(c.effects[cursor:]))
	copy(out, c.effects[cursor:])
	return out
}

func NormalizeFilePath(path string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if abs, err := filepath.Abs(path); err == nil {
		observe.GlobalTrace("if: err == nil")
		observe.GlobalTrace("return: filepath.Clean(abs)")
		return filepath.Clean(abs)
	}
	observe.GlobalTrace("return: filepath.Clean(path)")
	return filepath.Clean(path)
}

func FileTimestamp(path string) (int64, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	info, err := os.Stat(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: 0, err")
		return 0, err
	}
	observe.GlobalTrace("return: info.ModTime().UnixMilli(), nil")
	return info.ModTime().UnixMilli(), nil
}

func NormalizeTextContent(content string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: strings.ReplaceAll(content, \"\\r\\n\", \"\\n\")")
	return strings.ReplaceAll(content, "\r\n", "\n")
}

func EnsureFileFreshForWrite(state StateSnapshot, path, currentContent string, timestamp int64) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cache, ok := FileStateCacheFrom(state)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: ErrFileReadState")
		return ErrFileReadState
	}
	readState, ok := cache.Get(path)
	if !ok || readState.IsPartialView {
		observe.GlobalTrace("if: !ok || readState.IsPartialView")
		observe.GlobalTrace("return: ErrFileNotRead")
		return ErrFileNotRead
	}
	if timestamp > readState.Timestamp {
		observe.GlobalTrace("if: timestamp > readState.Timestamp")
		if readState.Offset == nil && readState.Limit == nil && currentContent == readState.Content {
			observe.GlobalTrace("if: readState.Offset == nil && readState.Limit == nil && currentContent == readSt...")
			observe.GlobalTrace("return: nil")
			return nil
		}
		observe.GlobalTrace("return: ErrFileModified")
		return ErrFileModified
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func RecordFileState(state StateSnapshot, path, content string, timestamp int64, offset, limit *int, isPartialView bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cache, ok := FileStateCacheFrom(state)
	if !ok {
		observe.GlobalTrace("if: !ok")
		return
	}
	cache.Set(path, FileState{
		Content:       content,
		Timestamp:     timestamp,
		Offset:        offset,
		Limit:         limit,
		IsPartialView: isPartialView,
	})
}

func RecordFileWriteState(state StateSnapshot, path, content string, timestamp int64, offset, limit *int, isPartialView bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cache, ok := FileStateCacheFrom(state)
	if !ok {
		observe.GlobalTrace("if: !ok")
		return
	}
	cache.Set(path, FileState{
		Content:       content,
		Timestamp:     timestamp,
		Offset:        offset,
		Limit:         limit,
		IsPartialView: isPartialView,
	})
	cache.recordEffect(path, "write")
}

func RecordFileDelete(state StateSnapshot, path string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cache, ok := FileStateCacheFrom(state)
	if !ok {
		observe.GlobalTrace("if: !ok")
		return
	}
	cache.Delete(path)
	cache.recordEffect(path, "delete")
}

func cloneFileState(state FileState) FileState {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	state.Offset = cloneIntPtr(state.Offset)
	state.Limit = cloneIntPtr(state.Limit)
	observe.GlobalTrace("return: state")
	return state
}

func cloneIntPtr(v *int) *int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if v == nil {
		observe.GlobalTrace("if: v == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	out := *v
	observe.GlobalTrace("return: &out")
	return &out
}

func (c *FileStateCache) touchLocked(key string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.removeOrderLocked(key)
	c.order = append(c.order, key)
}

func (c *FileStateCache) removeOrderLocked(key string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for i, existing := range c.order {
		observe.GlobalTrace("range c.order")
		if existing == key {
			observe.GlobalTrace("if: existing == key")
			c.order = append(c.order[:i], c.order[i+1:]...)
			return
		}
	}
}

func (c *FileStateCache) evictLocked() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for len(c.entries) > 1 && (len(c.entries) > c.maxItems || c.bytes > c.maxBytes) {
		observe.GlobalTrace("for: len(c.entries) > 1 && (len(c.entries) > c.maxItems || c.bytes > c.maxBytes)")
		key := c.order[0]
		c.order = c.order[1:]
		entry := c.entries[key]
		delete(c.entries, key)
		c.bytes -= entry.size
	}
}

func (c *FileStateCache) recordEffect(path, operation string) {
	if c == nil {
		return
	}
	effect := FileEffect{
		Path:      NormalizeFilePath(path),
		Operation: operation,
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.effects = append(c.effects, effect)
}
