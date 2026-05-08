package tool_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	toolpkg "github.com/artpar/pragma/internal/tool"
	"github.com/artpar/pragma/internal/tools/fileedit"
	"github.com/artpar/pragma/internal/tools/fileread"
)

type allowChecker struct{}

func (allowChecker) Check(_ context.Context, _ string, _ string) permission.CheckResult {
	return permission.CheckResult{Decision: permission.DecisionAllow}
}

func (allowChecker) AddSessionRule(_ permission.Rule) {}

type fileFlowState struct {
	cwd   string
	cache *toolpkg.FileStateCache
}

func (s fileFlowState) WorkDir() string { return s.cwd }
func (s fileFlowState) ReadFileState() *toolpkg.FileStateCache {
	return s.cache
}

func TestReadThenEditSameBatchUsesReadState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(path, []byte("hello world\n"), 0644); err != nil {
		t.Fatal(err)
	}

	bus := observe.NewEventBus(10000)
	defer bus.Drain()
	registry := toolpkg.NewRegistry(bus)
	if err := registry.Register(&fileread.Tool{}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(&fileedit.Tool{}); err != nil {
		t.Fatal(err)
	}
	orch := toolpkg.NewOrchestrator(registry, allowChecker{}, &permission.NonInteractivePrompter{}, bus)

	readInput, _ := json.Marshal(fileread.FileReadInput{FilePath: path})
	editInput, _ := json.Marshal(fileedit.FileEditInput{
		FilePath:  path,
		OldString: "hello",
		NewString: "goodbye",
	})
	result := orch.Execute(context.Background(), []model.ToolCallPart{
		{ID: "read-1", Name: "Read", Input: readInput},
		{ID: "edit-1", Name: "Edit", Input: editInput},
	}, fileFlowState{cwd: dir, cache: toolpkg.NewFileStateCache()})

	if len(result.Results) != 2 {
		t.Fatalf("results: got %d, want 2", len(result.Results))
	}
	for _, part := range result.Results {
		if part.IsError {
			t.Fatalf("tool %s failed: %s", part.ToolCallID, part.Content)
		}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "goodbye world") {
		t.Fatalf("file was not edited: %q", string(content))
	}
}
