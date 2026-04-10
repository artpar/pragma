package fileedit

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

func TestFileEditTool_BasicReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(FileEditInput{
		FilePath:  path,
		OldString: "hello",
		NewString: "goodbye",
	})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content,"updated successfully") {
		t.Errorf("expected success message, got: %s", result.Content)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "goodbye world" {
		t.Errorf("expected 'goodbye world', got: %s", string(content))
	}
}

func TestFileEditTool_ReplaceAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("foo bar foo baz foo"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(FileEditInput{
		FilePath:   path,
		OldString:  "foo",
		NewString:  "qux",
		ReplaceAll: true,
	})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content,"All 3 occurrences") {
		t.Errorf("expected 'All 3 occurrences', got: %s", result.Content)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "qux bar qux baz qux" {
		t.Errorf("expected all replaced, got: %s", string(content))
	}
}

func TestFileEditTool_MultipleMatchesNoReplaceAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("foo foo foo"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(FileEditInput{
		FilePath:  path,
		OldString: "foo",
		NewString: "bar",
	})

	_, err := tool.Invoke(context.Background(), input, testState{dir})
	if err == nil {
		t.Fatal("expected error for multiple matches without replace_all")
	}
	if !strings.Contains(err.Error(), "3 matches") {
		t.Errorf("expected match count in error, got: %v", err)
	}
}

func TestFileEditTool_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(FileEditInput{
		FilePath:  path,
		OldString: "nonexistent",
		NewString: "replacement",
	})

	_, err := tool.Invoke(context.Background(), input, testState{dir})
	if err == nil {
		t.Fatal("expected error for string not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestFileEditTool_NoOpReject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(FileEditInput{
		FilePath:  path,
		OldString: "hello",
		NewString: "hello",
	})

	_, err := tool.Invoke(context.Background(), input, testState{dir})
	if err == nil {
		t.Fatal("expected error for no-op edit")
	}
	if !strings.Contains(err.Error(), "same") {
		t.Errorf("expected 'same' error, got: %v", err)
	}
}

func TestFileEditTool_CreateNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	tool := &Tool{}
	input, _ := json.Marshal(FileEditInput{
		FilePath:  path,
		OldString: "",
		NewString: "new content",
	})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content,"created") {
		t.Errorf("expected 'created', got: %s", result.Content)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "new content" {
		t.Errorf("expected 'new content', got: %s", string(content))
	}
}

func TestFileEditTool_EmptyOldStringExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	os.WriteFile(path, []byte("has content"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(FileEditInput{
		FilePath:  path,
		OldString: "",
		NewString: "replacement",
	})

	_, err := tool.Invoke(context.Background(), input, testState{dir})
	if err == nil {
		t.Fatal("expected error for empty old_string on non-empty file")
	}
}

func TestFileEditTool_NonexistentFileWithOldString(t *testing.T) {
	tool := &Tool{}
	input, _ := json.Marshal(FileEditInput{
		FilePath:  "/tmp/nonexistent_xyzzy.txt",
		OldString: "something",
		NewString: "replacement",
	})

	_, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	if err == nil {
		t.Fatal("expected error for nonexistent file with old_string")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected 'does not exist' error, got: %v", err)
	}
}

func TestFileEditTool_RejectIpynb(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.ipynb")
	os.WriteFile(path, []byte("{}"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(FileEditInput{
		FilePath:  path,
		OldString: "{}",
		NewString: "{\"cells\":[]}",
	})

	_, err := tool.Invoke(context.Background(), input, testState{dir})
	if err == nil {
		t.Fatal("expected error for .ipynb file")
	}
	if !strings.Contains(err.Error(), "Jupyter") {
		t.Errorf("expected Jupyter error, got: %v", err)
	}
}

func TestFileEditTool_CRLFNormalization(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "crlf.txt")
	os.WriteFile(path, []byte("line1\r\nline2\r\nline3\r\n"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(FileEditInput{
		FilePath:  path,
		OldString: "line2",
		NewString: "REPLACED",
	})

	_, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "REPLACED") {
		t.Errorf("expected replacement, got: %s", string(content))
	}
}
