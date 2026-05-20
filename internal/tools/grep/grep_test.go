package grep

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

func TestGrepTool_BasicMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc hello() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\nfunc world() {}\n"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(map[string]any{
		"pattern":     "func",
		"path":        dir,
		"output_mode": "files_with_matches",
	})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "Found 2 files") {
		t.Errorf("expected 2 files, got: %s", result.Content)
	}
}

func TestGrepTool_ContentMode(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("line1\nfoo bar\nline3\n"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(map[string]any{
		"pattern":     "foo",
		"path":        dir,
		"output_mode": "content",
	})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "foo bar") {
		t.Errorf("expected content with 'foo bar', got: %s", result.Content)
	}
}

func TestGrepTool_CountMode(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("foo\nbar\nfoo\n"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(map[string]any{
		"pattern":     "foo",
		"path":        dir,
		"output_mode": "count",
	})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "2 total occurrences") {
		t.Errorf("expected 2 occurrences, got: %s", result.Content)
	}
}

func TestGrepTool_NoMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello world\n"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(map[string]any{
		"pattern": "nonexistent",
		"path":    dir,
	})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "No files were found") {
		t.Errorf("expected 'No files were found', got: %s", result.Content)
	}
}

func TestGrepTool_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("Hello World\n"), 0644)

	tool := &Tool{}
	caseInsens := true
	input, _ := json.Marshal(map[string]any{
		"pattern": "hello",
		"path":    dir,
		"-i":      caseInsens,
	})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "Found 1 file") {
		t.Errorf("expected 1 file match, got: %s", result.Content)
	}
}

func TestGrepTool_GlobFilter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("func hello\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("func world\n"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(map[string]any{
		"pattern": "func",
		"path":    dir,
		"glob":    "*.go",
	})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "Found 1 file") {
		t.Errorf("expected 1 file (filtered to .go), got: %s", result.Content)
	}
}

func TestGrepTool_PatternStartingWithDash(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("--flag value\n"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(map[string]any{
		"pattern": "--flag",
		"path":    dir,
	})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "Found 1 file") {
		t.Errorf("expected match for pattern starting with dash, got: %s", result.Content)
	}
}

func TestGrepTool_MissingPattern(t *testing.T) {
	tool := &Tool{}
	input, _ := json.Marshal(map[string]any{})

	_, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	if err == nil {
		t.Fatal("expected error for missing pattern")
	}
}

func TestGrepTool_HeadLimit(t *testing.T) {
	dir := t.TempDir()
	// Create many files with matches
	for i := 0; i < 10; i++ {
		os.WriteFile(filepath.Join(dir, strings.Replace("file_NN.txt", "NN", strings.Repeat("a", i+1), 1)), []byte("match\n"), 0644)
	}

	tool := &Tool{}
	input, _ := json.Marshal(map[string]any{
		"pattern":    "match",
		"path":       dir,
		"head_limit": 3,
	})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "Found 3 files") {
		t.Errorf("expected 3 files (head_limit), got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "limit: 3") {
		t.Errorf("expected limit info, got: %s", result.Content)
	}
}
