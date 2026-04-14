package filewrite

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testState struct{ dir string }

func (s testState) WorkDir() string { return s.dir }

func TestFileWriteTool_CreateNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	tool := &Tool{}
	input, _ := json.Marshal(FileWriteInput{FilePath: path, Content: "hello world"})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content,"created") {
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

	tool := &Tool{}
	input, _ := json.Marshal(FileWriteInput{FilePath: path, Content: "new content"})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content,"updated") {
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

	_, err := tool.Invoke(context.Background(), input, testState{dir})
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

	_, err := tool.Invoke(context.Background(), input, testState{dir})
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

	_, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "" {
		t.Errorf("expected empty content, got: %s", string(content))
	}
}
