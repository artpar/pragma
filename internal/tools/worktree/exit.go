package worktree

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

type exitInput struct {
	WorktreePath string `json:"worktree_path" desc:"The absolute path of the worktree to exit"`
	HeadCommit   string `json:"head_commit,omitempty" desc:"The HEAD commit from when the worktree was created (from EnterWorktree output)"`
}

var exitSchema = json.RawMessage(`{
	"type": "object",
	"required": ["worktree_path"],
	"properties": {
		"worktree_path": {
			"type": "string",
			"description": "The absolute path of the worktree to exit"
		},
		"head_commit": {
			"type": "string",
			"description": "The HEAD commit from when the worktree was created (returned by EnterWorktree). Used to detect new commits."
		}
	}
}`)

type exitResult struct {
	Status       string `json:"status"`
	WorktreePath string `json:"worktree_path"`
	Branch       string `json:"branch,omitempty"`
	DiffStat     string `json:"diff_stat,omitempty"`
	Message      string `json:"message,omitempty"`
}

// ExitTool cleans up a git worktree, preserving it if there are changes.
type ExitTool struct{}

func (t *ExitTool) Name() string                { return "ExitWorktree" }
func (t *ExitTool) Description() string          { return "Exit and optionally remove a git worktree." }
func (t *ExitTool) InputSchema() json.RawMessage { return exitSchema }
func (t *ExitTool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *ExitTool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	var in struct {
		WorktreePath string `json:"worktree_path"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.WorktreePath == "" {
		return checker.Check(ctx, "ExitWorktree", "")
	}
	return checker.Check(ctx, "ExitWorktree", in.WorktreePath)
}

func (t *ExitTool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	var in exitInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.WorktreePath == "" {
		return tool.InvokeResult{}, fmt.Errorf("worktree_path is required")
	}

	// Resolve to absolute path for safety
	wtPath, err := filepath.Abs(in.WorktreePath)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("resolve worktree path: %w", err)
	}

	// Get the branch name for this worktree
	branchCmd := exec.CommandContext(ctx, "git", "-C", wtPath, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("get worktree branch: %w", err)
	}
	branch := strings.TrimSpace(string(branchOut))

	// Use the head_commit from EnterWorktree if provided. Without it,
	// HasChanges can still detect uncommitted changes (git status) but
	// cannot detect new commits (needs the original HEAD to compare).
	headCommit := in.HeadCommit
	if headCommit == "" {
		// Without original head commit, get current HEAD so HasChanges
		// at least checks for uncommitted changes via git status.
		revCmd := exec.CommandContext(ctx, "git", "-C", wtPath, "rev-parse", "HEAD")
		revOut, revErr := revCmd.Output()
		if revErr != nil {
			return tool.InvokeResult{}, fmt.Errorf("get HEAD: %w", revErr)
		}
		headCommit = strings.TrimSpace(string(revOut))
	}

	changed, err := HasChanges(wtPath, headCommit)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("check changes: %w", err)
	}

	if !changed {
		// No changes — remove the worktree
		// Find the git root (main repo) to run worktree remove from there
		gitRootCmd := exec.CommandContext(ctx, "git", "-C", state.WorkDir(), "rev-parse", "--show-toplevel")
		gitRootOut, err := gitRootCmd.Output()
		if err != nil {
			return tool.InvokeResult{}, fmt.Errorf("find git root: %w", err)
		}
		gitRoot := strings.TrimSpace(string(gitRootOut))

		removeCmd := exec.CommandContext(ctx, "git", "-C", gitRoot, "worktree", "remove", "--force", wtPath)
		removeOut, err := removeCmd.CombinedOutput()
		if err != nil {
			return tool.InvokeResult{}, fmt.Errorf("git worktree remove: %s: %w", strings.TrimSpace(string(removeOut)), err)
		}

		result := exitResult{
			Status:       "removed",
			WorktreePath: wtPath,
			Message:      "Worktree removed (no changes detected)",
		}
		data, _ := json.Marshal(result)
		return tool.InvokeResult{Content: string(data)}, nil
	}

	// Has changes — keep worktree, report diff stat
	diffCmd := exec.CommandContext(ctx, "git", "-C", wtPath, "diff", "--stat")
	diffOut, _ := diffCmd.Output()
	diffStat := strings.TrimSpace(string(diffOut))

	// Also include staged changes
	diffStagedCmd := exec.CommandContext(ctx, "git", "-C", wtPath, "diff", "--cached", "--stat")
	diffStagedOut, _ := diffStagedCmd.Output()
	if staged := strings.TrimSpace(string(diffStagedOut)); staged != "" {
		if diffStat != "" {
			diffStat += "\n(staged)\n" + staged
		} else {
			diffStat = "(staged)\n" + staged
		}
	}

	result := exitResult{
		Status:       "kept",
		WorktreePath: wtPath,
		Branch:       branch,
		DiffStat:     diffStat,
		Message:      "Worktree kept — has uncommitted changes or new commits",
	}
	data, _ := json.Marshal(result)
	return tool.InvokeResult{Content: string(data)}, nil
}
