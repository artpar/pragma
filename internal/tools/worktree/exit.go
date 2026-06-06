package worktree

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
)

type exitInput struct {
	WorktreePath string `json:"worktree_path" desc:"The absolute path of the worktree to exit"`
	HeadCommit   string `json:"head_commit,omitempty" desc:"The HEAD commit from when the worktree was created (from EnterWorktree output)"`
}

var exitSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
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
	RestoredCWD  string `json:"restored_cwd,omitempty"`
	DiffStat     string `json:"diff_stat,omitempty"`
	Message      string `json:"message,omitempty"`
}

// ExitTool cleans up a git worktree, preserving it if there are changes.
type ExitTool struct {
	Store                  *app.StateStore
	SystemPromptForWorkDir func(string) model.SystemPrompt
}

func (t *ExitTool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"ExitWorktree\"")
	return "ExitWorktree"
}
func (t *ExitTool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: exitDescription")
	return exitDescription
}

const exitDescription = `Exit a worktree session created by EnterWorktree and return the session to the original working directory.

## Scope

This tool ONLY operates on worktrees created by EnterWorktree in this session. It will NOT touch:
- Worktrees you created manually with ` + "`git worktree add`" + `
- Worktrees from a previous session (even if created by EnterWorktree then)
- The directory you're in if EnterWorktree was never called

If called outside an EnterWorktree session, the tool is a no-op: it reports that no worktree session is active and takes no action.

## When to Use

- The user explicitly asks to "exit the worktree", "leave the worktree", "go back", or otherwise end the worktree session
- Do NOT call this proactively — only when the user asks

## Behavior

- If the worktree has no changes (no uncommitted files, no new commits), it is automatically removed
- If the worktree has changes, it is kept and the diff stat is returned so the user can decide what to do
- Restores the session's working directory to where it was before EnterWorktree`

func (t *ExitTool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: exitSchema")
	return exitSchema
}
func (t *ExitTool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: false}")
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *ExitTool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "worktree", "ExitTool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "worktree", "ExitTool.CheckPerm", "exit")
	var in struct {
		WorktreePath string `json:"worktree_path"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.WorktreePath == "" {
		observe.TraceCtx(ctx, "worktree", "ExitTool.CheckPerm", "if: err != nil || in.WorktreePath == \"\"")
		observe.TraceCtx(ctx, "worktree", "ExitTool.CheckPerm", "return: checker.Check(ctx, \"ExitWorktree\", \"\")")
		return checker.Check(ctx, "ExitWorktree", "")
	}
	observe.TraceCtx(ctx, "worktree", "ExitTool.CheckPerm", "return: checker.Check(ctx, \"ExitWorktree\", in.WorktreePath)")
	return checker.Check(ctx, "ExitWorktree", in.WorktreePath)
}

func (t *ExitTool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "exit")
	var in exitInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.WorktreePath == "" {
		observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "if: in.WorktreePath == \"\"")
		observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"worktree_path is required\")")
		return tool.InvokeResult{}, fmt.Errorf("worktree_path is required")
	}

	wtPath, err := filepath.Abs(in.WorktreePath)
	if err != nil {
		observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"resolve worktree path: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("resolve worktree path: %w", err)
	}
	wtPath = filepath.Clean(wtPath)

	originalWorkDir := ""
	if t.Store != nil {
		snap := t.Store.Snapshot()
		if snap.Worktree == nil {
			result := exitResult{
				Status:       "no_active_worktree",
				WorktreePath: wtPath,
				Message:      "No worktree session is active",
			}
			data, _ := json.Marshal(result)
			return tool.InvokeResult{Content: string(data)}, nil
		}
		activePath, err := filepath.Abs(snap.Worktree.WorktreePath)
		if err != nil {
			return tool.InvokeResult{}, fmt.Errorf("resolve active worktree path: %w", err)
		}
		activePath = filepath.Clean(activePath)
		if activePath != wtPath {
			return tool.InvokeResult{}, fmt.Errorf("active worktree is %s, not %s", activePath, wtPath)
		}
		originalWorkDir = snap.Worktree.OriginalCWD
		if in.HeadCommit == "" {
			in.HeadCommit = snap.Worktree.HeadCommit
		}
	}

	branchCmd := exec.CommandContext(ctx, "git", "-C", wtPath, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	if err != nil {
		observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"get worktree branch: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("get worktree branch: %w", err)
	}
	branch := strings.TrimSpace(string(branchOut))

	headCommit := in.HeadCommit
	if headCommit == "" {
		observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "if: headCommit == \"\"")

		revCmd := exec.CommandContext(ctx, "git", "-C", wtPath, "rev-parse", "HEAD")
		revOut, revErr := revCmd.Output()
		if revErr != nil {
			observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "if: revErr != nil")
			observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"get HEAD: %w\", revErr)")
			return tool.InvokeResult{}, fmt.Errorf("get HEAD: %w", revErr)
		}
		headCommit = strings.TrimSpace(string(revOut))
	}

	changed, err := HasChanges(wtPath, headCommit)
	if err != nil {
		observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"check changes: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("check changes: %w", err)
	}

	if !changed {
		observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "if: !changed")

		gitBase := state.WorkDir()
		if originalWorkDir != "" {
			gitBase = originalWorkDir
		}
		gitRootCmd := exec.CommandContext(ctx, "git", "-C", gitBase, "rev-parse", "--show-toplevel")
		gitRootOut, err := gitRootCmd.Output()
		if err != nil {
			observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "if: err != nil")
			observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"find git root: %w\", err)")
			return tool.InvokeResult{}, fmt.Errorf("find git root: %w", err)
		}
		gitRoot := strings.TrimSpace(string(gitRootOut))

		removeCmd := exec.CommandContext(ctx, "git", "-C", gitRoot, "worktree", "remove", "--force", wtPath)
		removeOut, err := removeCmd.CombinedOutput()
		if err != nil {
			observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "if: err != nil")
			observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"git worktree remove: %s: %w\", strings.TrimSp...")
			return tool.InvokeResult{}, fmt.Errorf("git worktree remove: %s: %w", strings.TrimSpace(string(removeOut)), err)
		}

		result := exitResult{
			Status:       "removed",
			WorktreePath: wtPath,
			RestoredCWD:  originalWorkDir,
			Message:      "Worktree removed (no changes detected)",
		}
		t.restoreSessionCWD(originalWorkDir)
		data, _ := json.Marshal(result)
		observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "return: tool.InvokeResult{Content: string(data)}, nil")
		return tool.InvokeResult{Content: string(data)}, nil
	}

	diffCmd := exec.CommandContext(ctx, "git", "-C", wtPath, "diff", "--stat")
	diffOut, _ := diffCmd.Output()
	diffStat := strings.TrimSpace(string(diffOut))

	diffStagedCmd := exec.CommandContext(ctx, "git", "-C", wtPath, "diff", "--cached", "--stat")
	diffStagedOut, _ := diffStagedCmd.Output()
	if staged := strings.TrimSpace(string(diffStagedOut)); staged != "" {
		observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "if: staged != \"\"")
		if diffStat != "" {
			observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "if: diffStat != \"\"")
			diffStat += "\n(staged)\n" + staged
		} else {
			observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "else: diffStat != \"\"")
			diffStat = "(staged)\n" + staged
		}
	}

	result := exitResult{
		Status:       "kept",
		WorktreePath: wtPath,
		Branch:       branch,
		RestoredCWD:  originalWorkDir,
		DiffStat:     diffStat,
		Message:      "Worktree kept — has uncommitted changes or new commits",
	}
	t.restoreSessionCWD(originalWorkDir)
	data, _ := json.Marshal(result)
	observe.TraceCtx(ctx, "worktree", "ExitTool.Invoke", "return: tool.InvokeResult{Content: string(data)}, nil")
	return tool.InvokeResult{Content: string(data)}, nil
}

func (t *ExitTool) restoreSessionCWD(originalWorkDir string) {
	if t.Store == nil || originalWorkDir == "" {
		return
	}
	system := t.systemPromptForWorkDir(originalWorkDir)
	t.Store.Update(func(s *app.AppState) {
		s.CWD = originalWorkDir
		s.Conversation.WorkDir = originalWorkDir
		if len(system.Blocks) > 0 {
			s.Conversation.System = system
		}
		s.Worktree = nil
	})
}

func (t *ExitTool) systemPromptForWorkDir(workDir string) model.SystemPrompt {
	if t.SystemPromptForWorkDir == nil {
		return model.SystemPrompt{}
	}
	return t.SystemPromptForWorkDir(workDir)
}
