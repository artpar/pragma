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

	"github.com/artpar/gogent/internal/observe"
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

func (t *EnterTool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"EnterWorktree\"")
	observe.GlobalTrace("return: \"EnterWorktree\"")
	return "EnterWorktree"
}
func (t *EnterTool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: enterDescription")
	observe.GlobalTrace("return: enterDescription")
	return enterDescription
}

const enterDescription = `Use this tool ONLY when the user explicitly asks to work in a worktree. This tool creates an isolated git worktree and switches the current session into it.

## When to Use

- The user explicitly says "worktree" (e.g., "start a worktree", "work in a worktree", "create a worktree")

## When NOT to Use

- The user asks to create a branch, switch branches, or work on a different branch — use git commands instead
- The user asks to fix a bug or work on a feature — use normal git workflow unless they specifically mention worktrees
- Never use this tool unless the user explicitly mentions "worktree"

## Requirements

- Must be in a git repository
- Must not already be in a worktree

## Behavior

- Creates a new git worktree inside ` + "`.gogent/worktrees/`" + ` with a new branch based on HEAD
- Switches the session's working directory to the new worktree
- Use ExitWorktree to leave the worktree mid-session (keep or remove)
- On session exit, if still in the worktree, the user will be prompted to keep or remove it`

func (t *EnterTool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: enterSchema")
	observe.GlobalTrace("return: enterSchema")
	return enterSchema
}
func (t *EnterTool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: false}")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: false}")
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *EnterTool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "worktree", "EnterTool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "worktree", "EnterTool.CheckPerm", "exit")
	var in struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.Slug == "" {
		observe.TraceCtx(ctx, "worktree", "EnterTool.CheckPerm", "if: err != nil || in.Slug == \"\"")
		observe.TraceCtx(ctx, "worktree", "EnterTool.CheckPerm", "return: checker.Check(ctx, \"EnterWorktree\", \"\")")
		observe.TraceCtx(ctx, "worktree", "EnterTool.CheckPerm", "return: checker.Check(ctx, \"EnterWorktree\", \"\")")
		return checker.Check(ctx, "EnterWorktree", "")
	}
	observe.TraceCtx(ctx, "worktree", "EnterTool.CheckPerm", "return: checker.Check(ctx, \"EnterWorktree\", in.Slug)")
	observe.TraceCtx(ctx, "worktree", "EnterTool.CheckPerm", "return: checker.Check(ctx, \"EnterWorktree\", in.Slug)")
	return checker.Check(ctx, "EnterWorktree", in.Slug)
}

func (t *EnterTool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "exit")
	var in enterInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}

	slug := in.Slug
	if slug == "" {
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "if: slug == \"\"")
		slug = fmt.Sprintf("wt-%d", time.Now().UnixMilli())
	}

	if err := validateSlug(slug); err != nil {
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid slug: %w\", err)")
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid slug: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid slug: %w", err)
	}

	flatSlug := FlattenSlug(slug)
	branch := "worktree-" + flatSlug
	dir := filepath.Join(state.WorkDir(), ".gogent", "worktrees", flatSlug)

	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"create worktree parent dir: %w\", err)")
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"create worktree parent dir: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("create worktree parent dir: %w", err)
	}

	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "-B", branch, dir, "HEAD")
	cmd.Dir = state.WorkDir()
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"git worktree add: %s: %w\", strings.TrimSpace...")
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"git worktree add: %s: %w\", strings.TrimSpace...")
		return tool.InvokeResult{}, fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(output)), err)
	}

	revCmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD")
	revOut, err := revCmd.Output()
	if err != nil {
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"git rev-parse HEAD: %w\", err)")
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"git rev-parse HEAD: %w\", err)")
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
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"marshal result: %w\", err)")
		observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"marshal result: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("marshal result: %w", err)
	}
	observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "return: tool.InvokeResult{Content: string(data)}, nil")
	observe.TraceCtx(ctx, "worktree", "EnterTool.Invoke", "return: tool.InvokeResult{Content: string(data)}, nil")
	return tool.InvokeResult{Content: string(data)}, nil
}
