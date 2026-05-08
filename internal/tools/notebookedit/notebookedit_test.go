package notebookedit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	toolpkg "github.com/artpar/pragma/internal/tool"
)

const sampleNotebook = `{
 "cells": [
  {
   "cell_type": "code",
   "id": "cell-1",
   "source": ["print('hello')\n"],
   "metadata": {},
   "execution_count": 1,
   "outputs": [{"output_type": "stream", "text": ["hello\n"]}]
  },
  {
   "cell_type": "markdown",
   "id": "cell-2",
   "source": ["# Title\n"],
   "metadata": {}
  }
 ],
 "metadata": {
  "kernelspec": {"display_name": "Python 3", "language": "python", "name": "python3"}
 },
 "nbformat": 4,
 "nbformat_minor": 5
}
`

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

func writeNotebookFile(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "test.ipynb")
	if err := os.WriteFile(path, []byte(sampleNotebook), 0644); err != nil {
		t.Fatal(err)
	}
	return path
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

func readNotebookFile(t *testing.T, path string) notebookContent {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var nb notebookContent
	if err := json.Unmarshal(data, &nb); err != nil {
		t.Fatal(err)
	}
	return nb
}

func TestReplaceCell(t *testing.T) {
	dir := t.TempDir()
	nbPath := writeNotebookFile(t, dir)
	state := newTestState(dir)
	markRead(t, state, nbPath)

	tool := &Tool{}
	input := json.RawMessage(`{
		"notebook_path": "` + nbPath + `",
		"cell_id": "cell-1",
		"new_source": "print('world')\n",
		"edit_mode": "replace"
	}`)

	result, err := tool.Invoke(nil, input, state)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content == "" {
		t.Error("expected non-empty result")
	}

	nb := readNotebookFile(t, nbPath)
	// Cell source should be updated
	var source []string
	json.Unmarshal(nb.Cells[0].Source, &source)
	if len(source) != 1 || source[0] != "print('world')\n" {
		t.Errorf("source: got %v", source)
	}
	// Execution count and outputs should be cleared for code cells
	if nb.Cells[0].ExecutionCount != nil {
		t.Errorf("execution_count should be nil, got %v", nb.Cells[0].ExecutionCount)
	}
}

func TestInsertCell(t *testing.T) {
	dir := t.TempDir()
	nbPath := writeNotebookFile(t, dir)
	state := newTestState(dir)
	markRead(t, state, nbPath)

	tool := &Tool{}
	input := json.RawMessage(`{
		"notebook_path": "` + nbPath + `",
		"cell_id": "cell-1",
		"new_source": "x = 42\n",
		"cell_type": "code",
		"edit_mode": "insert"
	}`)

	result, err := tool.Invoke(nil, input, state)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content == "" {
		t.Error("expected non-empty result")
	}

	nb := readNotebookFile(t, nbPath)
	if len(nb.Cells) != 3 {
		t.Fatalf("expected 3 cells, got %d", len(nb.Cells))
	}
	// New cell should be at index 1 (after cell-1)
	if nb.Cells[1].CellType != "code" {
		t.Errorf("inserted cell type: %s", nb.Cells[1].CellType)
	}
}

func TestDeleteCell(t *testing.T) {
	dir := t.TempDir()
	nbPath := writeNotebookFile(t, dir)
	state := newTestState(dir)
	markRead(t, state, nbPath)

	tool := &Tool{}
	input := json.RawMessage(`{
		"notebook_path": "` + nbPath + `",
		"cell_id": "cell-2",
		"new_source": "",
		"edit_mode": "delete"
	}`)

	result, err := tool.Invoke(nil, input, state)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content == "" {
		t.Error("expected non-empty result")
	}

	nb := readNotebookFile(t, nbPath)
	if len(nb.Cells) != 1 {
		t.Fatalf("expected 1 cell, got %d", len(nb.Cells))
	}
	if nb.Cells[0].ID != "cell-1" {
		t.Errorf("remaining cell: %s", nb.Cells[0].ID)
	}
}

func TestCellByNumericIndex(t *testing.T) {
	dir := t.TempDir()
	nbPath := writeNotebookFile(t, dir)
	state := newTestState(dir)
	markRead(t, state, nbPath)

	tool := &Tool{}
	input := json.RawMessage(`{
		"notebook_path": "` + nbPath + `",
		"cell_id": "1",
		"new_source": "# New Title\n",
		"edit_mode": "replace"
	}`)

	result, err := tool.Invoke(nil, input, state)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content == "" {
		t.Error("expected non-empty result")
	}

	nb := readNotebookFile(t, nbPath)
	var source []string
	json.Unmarshal(nb.Cells[1].Source, &source)
	if len(source) != 1 || source[0] != "# New Title\n" {
		t.Errorf("source: got %v", source)
	}
}

func TestInvalidExtension(t *testing.T) {
	tool := &Tool{}
	input := json.RawMessage(`{"notebook_path": "/tmp/test.py", "new_source": "x"}`)
	_, err := tool.Invoke(nil, input, newTestState("/tmp"))
	if err == nil {
		t.Error("expected error for non-.ipynb file")
	}
}
