package worktree

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

type enterInput struct {
	Slug string `json:"slug,omitempty" desc:"Optional slug for the worktree (max 64 chars, a-zA-Z0-9._-/ allowed)"`
}

var enterSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"slug": {
			"type": "string",
			"description": "Optional slug for the worktree (max 64 chars, a-zA-Z0-9._-/ per segment). If omitted, a timestamp-based slug is generated."
		}
	}
}`)

type enterResult struct {
	WorktreePath string `json:"worktree_path"`
	Branch       string `json:"branch"`
	HeadCommit   string `json:"head_commit"`
}

// EnterTool creates a git worktree for isolated work.
type EnterTool struct{}

func (t *EnterTool) Name() string                { return "EnterWorktree" }
func (t *EnterTool) Description() string          { return "Create a git worktree for isolated file operations." }
func (t *EnterTool) InputSchema() json.RawMessage { return enterSchema }
func (t *EnterTool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *EnterTool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	var in struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.Slug == "" {
		return checker.Check(ctx, "EnterWorktree", "")
	}
	return checker.Check(ctx, "EnterWorktree", in.Slug)
}

func (t *EnterTool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	var in enterInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}

	slug := in.Slug
	if slug == "" {
		slug = fmt.Sprintf("wt-%d", time.Now().UnixMilli())
	}

	if err := validateSlug(slug); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid slug: %w", err)
	}

	flatSlug := FlattenSlug(slug)
	branch := "worktree-" + flatSlug
	dir := filepath.Join(state.WorkDir(), ".gogent", "worktrees", flatSlug)

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("create worktree parent dir: %w", err)
	}

	// Create the worktree
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "-B", branch, dir, "HEAD")
	cmd.Dir = state.WorkDir()
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(output)), err)
	}

	// Get the HEAD commit of the new worktree
	revCmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD")
	revOut, err := revCmd.Output()
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	headCommit := strings.TrimSpace(string(revOut))

	result := enterResult{
		WorktreePath: dir,
		Branch:       branch,
		HeadCommit:   headCommit,
	}
	data, err := json.Marshal(result)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("marshal result: %w", err)
	}
	return tool.InvokeResult{Content: string(data)}, nil
}
