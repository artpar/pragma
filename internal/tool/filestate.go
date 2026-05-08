package tool

import (
	"errors"
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
}

type fileStateEntry struct {
	state FileState
	size  int
}

func NewFileStateCache() *FileStateCache {
	return &FileStateCache{
		entries:  make(map[string]fileStateEntry),
		order:    make([]string, 0, defaultFileStateEntries),
		maxItems: defaultFileStateEntries,
		maxBytes: defaultFileStateBytes,
	}
}

func FileStateCacheFrom(state StateSnapshot) (*FileStateCache, bool) {
	provider, ok := state.(FileStateProvider)
	if !ok {
		return nil, false
	}
	cache := provider.ReadFileState()
	return cache, cache != nil
}

func (c *FileStateCache) Get(path string) (FileState, bool) {
	if c == nil {
		return FileState{}, false
	}
	key := NormalizeFilePath(path)

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return FileState{}, false
	}
	c.touchLocked(key)
	return cloneFileState(entry.state), true
}

func (c *FileStateCache) Set(path string, state FileState) {
	if c == nil {
		return
	}
	key := NormalizeFilePath(path)
	state = cloneFileState(state)
	size := len([]byte(state.Content))

	c.mu.Lock()
	defer c.mu.Unlock()

	if old, ok := c.entries[key]; ok {
		c.bytes -= old.size
		c.removeOrderLocked(key)
	}
	c.entries[key] = fileStateEntry{state: state, size: size}
	c.order = append(c.order, key)
	c.bytes += size
	c.evictLocked()
}

func (c *FileStateCache) Delete(path string) {
	if c == nil {
		return
	}
	key := NormalizeFilePath(path)

	c.mu.Lock()
	defer c.mu.Unlock()

	if old, ok := c.entries[key]; ok {
		delete(c.entries, key)
		c.bytes -= old.size
		c.removeOrderLocked(key)
	}
}

func NormalizeFilePath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}

func FileTimestamp(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.ModTime().UnixMilli(), nil
}

func NormalizeTextContent(content string) string {
	return strings.ReplaceAll(content, "\r\n", "\n")
}

func EnsureFileFreshForWrite(state StateSnapshot, path, currentContent string, timestamp int64) error {
	cache, ok := FileStateCacheFrom(state)
	if !ok {
		return ErrFileReadState
	}
	readState, ok := cache.Get(path)
	if !ok || readState.IsPartialView {
		return ErrFileNotRead
	}
	if timestamp > readState.Timestamp {
		if readState.Offset == nil && readState.Limit == nil && currentContent == readState.Content {
			return nil
		}
		return ErrFileModified
	}
	return nil
}

func RecordFileState(state StateSnapshot, path, content string, timestamp int64, offset, limit *int, isPartialView bool) {
	cache, ok := FileStateCacheFrom(state)
	if !ok {
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

func cloneFileState(state FileState) FileState {
	state.Offset = cloneIntPtr(state.Offset)
	state.Limit = cloneIntPtr(state.Limit)
	return state
}

func cloneIntPtr(v *int) *int {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func (c *FileStateCache) touchLocked(key string) {
	c.removeOrderLocked(key)
	c.order = append(c.order, key)
}

func (c *FileStateCache) removeOrderLocked(key string) {
	for i, existing := range c.order {
		if existing == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			return
		}
	}
}

func (c *FileStateCache) evictLocked() {
	for len(c.entries) > 1 && (len(c.entries) > c.maxItems || c.bytes > c.maxBytes) {
		key := c.order[0]
		c.order = c.order[1:]
		entry := c.entries[key]
		delete(c.entries, key)
		c.bytes -= entry.size
	}
}
