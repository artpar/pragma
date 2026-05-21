package glob

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testState struct{ dir string }

func (s testState) WorkDir() string { return s.dir }

func TestGlobTool_BasicMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("hello"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "d.go"), []byte("package sub"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(GlobInput{Pattern: "**/*.go"})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(result.Content, "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	for _, line := range lines {
		if !strings.HasSuffix(line, ".go") {
			t.Errorf("unexpected non-.go file: %s", line)
		}
	}
}

func TestGlobTool_RelativePath(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0755)
	os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(GlobInput{Pattern: "*.go", Path: "src"})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content != "main.go" {
		t.Errorf("expected 'main.go', got: %q", result.Content)
	}
}

func TestGlobTool_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(GlobInput{Pattern: "*.txt", Path: dir})

	result, err := tool.Invoke(context.Background(), input, testState{"/tmp"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content != "x.txt" {
		t.Errorf("expected 'x.txt', got: %q", result.Content)
	}
}

func TestGlobTool_NoMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(GlobInput{Pattern: "*.go"})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(result.Content, "IMPORTANT: No files were found") {
		t.Errorf("expected no-match guardrail prefix, got: %q", result.Content)
	}
}

func TestGlobTool_MissingPattern(t *testing.T) {
	tool := &Tool{}
	input, _ := json.Marshal(GlobInput{})

	_, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	if err == nil {
		t.Fatal("expected error for empty pattern")
	}
}

func TestGlobTool_NonExistentDir(t *testing.T) {
	tool := &Tool{}
	input, _ := json.Marshal(GlobInput{Pattern: "*.go", Path: "/nonexistent/path/xyz"})

	_, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

func TestGlobTool_SkipsVCSDirs(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0755)
	os.WriteFile(filepath.Join(dir, ".git", "objects", "abc.go"), []byte("git internal"), 0644)
	os.WriteFile(filepath.Join(dir, "real.go"), []byte("package real"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(GlobInput{Pattern: "**/*.go"})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content != "real.go" {
		t.Errorf("expected only 'real.go', got: %q", result.Content)
	}
}

func TestGlobTool_Truncation(t *testing.T) {
	dir := t.TempDir()
	// Create 105 files — should truncate at 100
	for i := 0; i < 105; i++ {
		name := filepath.Join(dir, fmt.Sprintf("file_%03d.txt", i))
		os.WriteFile(name, []byte("x"), 0644)
	}

	tool := &Tool{}
	input, _ := json.Marshal(GlobInput{Pattern: "*.txt"})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(result.Content, "\n")
	// 100 filenames + 1 truncation message
	if len(lines) != 101 {
		t.Errorf("expected 101 lines (100 files + truncation msg), got %d", len(lines))
	}
	if !strings.Contains(result.Content, "Results are truncated") {
		t.Errorf("expected truncation message, got: %s", result.Content[len(result.Content)-100:])
	}
}

func TestGlobTool_SortByModTime(t *testing.T) {
	dir := t.TempDir()

	// Create files with different mod times
	for i, name := range []string{"old.go", "mid.go", "new.go"} {
		path := filepath.Join(dir, name)
		os.WriteFile(path, []byte("x"), 0644)
		// Set staggered mtimes: old=0s, mid=1s, new=2s
		mtime := baseTime.Add(time.Duration(i) * time.Second)
		os.Chtimes(path, mtime, mtime)
	}

	tool := &Tool{}
	input, _ := json.Marshal(GlobInput{Pattern: "*.go"})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(result.Content, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "new.go" {
		t.Errorf("expected newest first, got %s", lines[0])
	}
	if lines[2] != "old.go" {
		t.Errorf("expected oldest last, got %s", lines[2])
	}
}

var baseTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
