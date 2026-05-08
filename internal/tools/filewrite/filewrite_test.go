package filewrite

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolpkg "github.com/artpar/pragma/internal/tool"
)

type testState struct {
	dir   string
	cache *toolpkg.FileStateCache
}

func (s testState) WorkDir() string { return s.dir }
func (s testState) ReadFileState() *toolpkg.FileStateCache {
	return s.cache
}

func newTestState(dir string) testState {
	return testState{dir: dir, cache: toolpkg.NewFileStateCache()}
}

func markRead(t *testing.T, state testState, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	timestamp, err := toolpkg.FileTimestamp(path)
	if err != nil {
		t.Fatal(err)
	}
	state.cache.Set(path, toolpkg.FileState{
		Content:   toolpkg.NormalizeTextContent(string(data)),
		Timestamp: timestamp,
	})
}

func TestFileWriteTool_CreateNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	tool := &Tool{}
	input, _ := json.Marshal(FileWriteInput{FilePath: path, Content: "hello world"})

	result, err := tool.Invoke(context.Background(), input, newTestState(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "created") {
		t.Errorf("expected 'created', got: %s", result.Content)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "hello world" {
		t.Errorf("expected 'hello world', got: %s", string(content))
	}
}

func TestFileWriteTool_UpdateExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	os.WriteFile(path, []byte("old content"), 0644)
	state := newTestState(dir)
	markRead(t, state, path)

	tool := &Tool{}
	input, _ := json.Marshal(FileWriteInput{FilePath: path, Content: "new content"})

	result, err := tool.Invoke(context.Background(), input, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "updated") {
		t.Errorf("expected 'updated', got: %s", result.Content)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "new content" {
		t.Errorf("expected 'new content', got: %s", string(content))
	}
}

func TestFileWriteTool_CreateParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "file.txt")

	tool := &Tool{}
	input, _ := json.Marshal(FileWriteInput{FilePath: path, Content: "nested"})

	_, err := tool.Invoke(context.Background(), input, newTestState(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "nested" {
		t.Errorf("expected 'nested', got: %s", string(content))
	}
}

func TestFileWriteTool_RelativePathResolved(t *testing.T) {
	dir := t.TempDir()
	tool := &Tool{}
	input, _ := json.Marshal(FileWriteInput{FilePath: "relative.txt", Content: "resolved"})

	_, err := tool.Invoke(context.Background(), input, newTestState(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "relative.txt"))
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(content) != "resolved" {
		t.Errorf("expected 'resolved', got: %s", string(content))
	}
}

func TestFileWriteTool_EmptyContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")

	tool := &Tool{}
	input, _ := json.Marshal(FileWriteInput{FilePath: path, Content: ""})

	_, err := tool.Invoke(context.Background(), input, newTestState(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "" {
		t.Errorf("expected empty content, got: %s", string(content))
	}
}

func TestFileWriteTool_RequiresReadForExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	os.WriteFile(path, []byte("old content"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(FileWriteInput{FilePath: path, Content: "new content"})

	_, err := tool.Invoke(context.Background(), input, newTestState(dir))
	if err == nil {
		t.Fatal("expected read-first error")
	}
	if !strings.Contains(err.Error(), "not been read") {
		t.Errorf("expected read-first error, got: %v", err)
	}
}
