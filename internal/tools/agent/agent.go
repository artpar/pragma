package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/query"
	"github.com/artpar/gogent/internal/task"
	"github.com/artpar/gogent/internal/tool"
	"github.com/artpar/gogent/internal/tools/worktree"
)

// AgentInput defines the parameters for the Agent tool.
type AgentInput struct {
	Prompt          string `json:"prompt" desc:"The task for the sub-agent to perform"`
	Description     string `json:"description" desc:"A short (3-5 word) description of the task"`
	Model           string `json:"model,omitempty" desc:"Optional model override for the sub-agent"`
	RunInBackground bool   `json:"run_in_background,omitempty" desc:"Run the agent asynchronously in the background"`
	Isolation       string `json:"isolation,omitempty" desc:"Isolation mode: 'worktree' for git worktree isolation, or empty for shared workspace"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["prompt"],
	"properties": {
		"prompt": {
			"type": "string",
			"description": "The task for the sub-agent to perform"
		},
		"description": {
			"type": "string",
			"description": "A short (3-5 word) description of the task"
		},
		"model": {
			"type": "string",
			"description": "Optional model override for the sub-agent"
		},
		"run_in_background": {
			"type": "boolean",
			"description": "Run the agent asynchronously in the background. Returns immediately with a task ID."
		},
		"isolation": {
			"type": "string",
			"enum": ["worktree", ""],
			"description": "Isolation mode: 'worktree' creates a git worktree so the agent works on a separate copy of the repo."
		}
	}
}`)

type agentResult struct {
	Status       string `json:"status"`
	Prompt       string `json:"prompt"`
	Result       string `json:"result,omitempty"`
	TokensUsed   int    `json:"tokens_used,omitempty"`
	AgentID      string `json:"agent_id,omitempty"`
	TaskID       string `json:"task_id,omitempty"`
	WorktreePath string `json:"worktree_path,omitempty"`
	Branch       string `json:"branch,omitempty"`
}

// EngineFactory creates a sub-Engine for a forked conversation with scoped tools.
// Returns the engine and the sub-store (for reading final conversation state).
type EngineFactory func(forkedConv model.Conversation, scopedToolNames []string, modelOverride string) (*query.Engine, *app.StateStore)

// Tool implements the Agent tool for spawning sub-agents.
type Tool struct {
	EngineFactory EngineFactory
	Store         *app.StateStore // parent store — for forking the conversation
	Tasks         *task.Registry
	Bus           *observe.EventBus
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Agent\"")
	return "Agent"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Launch a sub-agent to handle a complex task autonomously.\"")
	return "Launch a sub-agent to handle a complex task autonomously."
}
func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: false}")
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "agent", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "agent", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "agent", "Tool.CheckPerm", "return: checker.Check(ctx, \"Agent\", \"\")")
	return checker.Check(ctx, "Agent", "")
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "agent", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "agent", "Tool.Invoke", "exit")
	var in AgentInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Prompt == "" {
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: in.Prompt == \"\"")
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"prompt is required\")")
		return tool.InvokeResult{}, fmt.Errorf("prompt is required")
	}
	if in.Isolation != "" && in.Isolation != "worktree" {
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: in.Isolation != \"\" && in.Isolation != \"worktree\"")
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"isolation must be 'worktree' or empty, got %...")
		return tool.InvokeResult{}, fmt.Errorf("isolation must be 'worktree' or empty, got %q", in.Isolation)
	}

	snapshot := t.Store.Snapshot()
	forkedConv := snapshot.Conversation.Fork(model.NewUUID())

	scopedTools := excludeTool(nil, "Agent")

	subject := in.Description
	if subject == "" {
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: subject == \"\"")
		subject = in.Prompt
		if len(subject) > 80 {
			observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: len(subject) > 80")
			subject = subject[:80] + "..."
		}
	}
	tk := t.Tasks.Create(subject, in.Prompt)
	_ = t.Tasks.Update(tk.ID, func(tt *task.Task) {
		tt.Status = task.TaskRunning
		tt.AgentName = subject
	})

	// Set up worktree isolation if requested
	var wtPath, wtBranch, wtHeadCommit string
	if in.Isolation == "worktree" {
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: in.Isolation == \"worktree\"")
		var err error
		wtPath, wtBranch, wtHeadCommit, err = t.createWorktree(ctx, state.WorkDir(), tk.ID)
		if err != nil {
			observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: err != nil")
			_ = t.Tasks.Update(tk.ID, func(tt *task.Task) {
				tt.Status = task.TaskFailed
				tt.Error = err.Error()
			})
			observe.TraceCtx(ctx, "agent", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"create worktree: %w\", err)")
			return tool.InvokeResult{}, fmt.Errorf("create worktree: %w", err)
		}
	}

	engine, subStore := t.EngineFactory(forkedConv, scopedTools, in.Model)

	if wtPath != "" {
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: wtPath != \"\"")
		subStore.Update(func(s *app.AppState) {
			s.CWD = wtPath
		})
	}

	t.Bus.Emit(observe.SubAgentSpawned{
		EventHeader: observe.NewEventHeader("SubAgentSpawned", "", observe.NewSpanID(), ""),
		SubAgentID:  tk.ID,
		AgentName:   subject,
		Model:       in.Model,
	})

	if in.RunInBackground {
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: in.RunInBackground")
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "return: t.runBackground(tk, engine, subStore, in, wtPath, wtBranch, wtHeadCommit)")
		return t.runBackground(tk, engine, subStore, in, wtPath, wtBranch, wtHeadCommit)
	}
	observe.TraceCtx(ctx, "agent", "Tool.Invoke", "return: t.runSync(ctx, tk, engine, in, wtPath, wtBranch, wtHeadCommit)")
	return t.runSync(ctx, tk, engine, in, wtPath, wtBranch, wtHeadCommit)
}

// runSync runs the sub-agent synchronously and returns the result.
func (t *Tool) runSync(
	ctx context.Context,
	tk *task.Task,
	engine *query.Engine,
	in AgentInput,
	wtPath, wtBranch, wtHeadCommit string,
) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "agent", "Tool.runSync", "enter")
	defer observe.TraceCtx(ctx, "agent", "Tool.runSync", "exit")
	startTime := time.Now()
	events := engine.Run(ctx, in.Prompt)

	var result strings.Builder
	var usage model.TokenUsage
	var turnCount int

	for ev := range events {
		observe.TraceCtx(ctx, "agent", "Tool.runSync", "range events")
		switch e := ev.(type) {
		case query.TextEvent:
			observe.TraceCtx(ctx, "agent", "Tool.runSync", "typecase: query.TextEvent")
			result.WriteString(e.Text)
		case query.TurnCompleteEvent:
			observe.TraceCtx(ctx, "agent", "Tool.runSync", "typecase: query.TurnCompleteEvent")
			turnCount++
			usage.InputTokens += e.Response.Usage.InputTokens
			usage.OutputTokens += e.Response.Usage.OutputTokens
			usage.CacheCreationInputTokens += e.Response.Usage.CacheCreationInputTokens
			usage.CacheReadInputTokens += e.Response.Usage.CacheReadInputTokens
		case query.ErrorEvent:
			observe.TraceCtx(ctx, "agent", "Tool.runSync", "typecase: query.ErrorEvent")
			_ = t.Tasks.Update(tk.ID, func(tt *task.Task) {
				tt.Status = task.TaskFailed
				tt.Error = e.Err.Error()
			})
			t.Bus.Emit(observe.SubAgentFailed{
				EventHeader:  observe.NewEventHeader("SubAgentFailed", "", observe.NewSpanID(), ""),
				SubAgentID:   tk.ID,
				ErrorType:    "agent_error",
				ErrorMessage: e.Err.Error(),
			})
			t.cleanupWorktreeIfEmpty(wtPath, wtHeadCommit)
			return tool.InvokeResult{
				Content: fmt.Sprintf("Agent failed: %v", e.Err),
			}, nil
		}
	}

	tokensUsed := usage.InputTokens + usage.OutputTokens

	resultStr := result.String()
	_ = t.Tasks.Update(tk.ID, func(tt *task.Task) {
		tt.Status = task.TaskCompleted
		tt.Result = resultStr
		tt.TokensUsed = tokensUsed
	})

	t.Bus.Emit(observe.SubAgentCompleted{
		EventHeader: observe.NewEventHeader("SubAgentCompleted", "", observe.NewSpanID(), ""),
		SubAgentID:  tk.ID,
		DurationMs:  time.Since(startTime).Milliseconds(),
		TurnCount:   turnCount,
		Usage:       usage,
	})

	t.cleanupWorktreeIfEmpty(wtPath, wtHeadCommit)

	ar := agentResult{
		Status:       "completed",
		Prompt:       in.Prompt,
		Result:       resultStr,
		TokensUsed:   tokensUsed,
		WorktreePath: wtPath,
		Branch:       wtBranch,
	}
	data, err := json.Marshal(ar)
	if err != nil {
		observe.TraceCtx(ctx, "agent", "Tool.runSync", "if: err != nil")
		observe.TraceCtx(ctx, "agent", "Tool.runSync", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Agent completed but failed to marshal...")
		return tool.InvokeResult{Content: fmt.Sprintf("Agent completed but failed to marshal result: %v", err)}, nil
	}
	observe.TraceCtx(ctx, "agent", "Tool.runSync", "return: tool.InvokeResult{Content: string(data)}, nil")
	return tool.InvokeResult{Content: string(data)}, nil
}

// runBackground launches the sub-agent in a goroutine and returns immediately.
func (t *Tool) runBackground(
	tk *task.Task,
	engine *query.Engine,
	subStore *app.StateStore,
	in AgentInput,
	wtPath, wtBranch, wtHeadCommit string,
) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	childCtx, cancelFn := context.WithCancel(context.Background())
	_ = t.Tasks.Update(tk.ID, func(tt *task.Task) {
		tt.Cancel = cancelFn
	})
	_ = subStore

	go func() {
		defer cancelFn()
		startTime := time.Now()

		events := engine.Run(childCtx, in.Prompt)

		var result strings.Builder
		var usage model.TokenUsage
		var turnCount int
		var failed bool

		for ev := range events {
			observe.GlobalTrace("range events")
			switch e := ev.(type) {
			case query.TextEvent:
				observe.GlobalTrace("typecase: query.TextEvent")
				result.WriteString(e.Text)
			case query.TurnCompleteEvent:
				observe.GlobalTrace("typecase: query.TurnCompleteEvent")
				turnCount++
				usage.InputTokens += e.Response.Usage.InputTokens
				usage.OutputTokens += e.Response.Usage.OutputTokens
				usage.CacheCreationInputTokens += e.Response.Usage.CacheCreationInputTokens
				usage.CacheReadInputTokens += e.Response.Usage.CacheReadInputTokens
			case query.ErrorEvent:
				observe.GlobalTrace("typecase: query.ErrorEvent")
				_ = t.Tasks.Update(tk.ID, func(tt *task.Task) {
					tt.Status = task.TaskFailed
					tt.Error = e.Err.Error()
				})
				t.Bus.Emit(observe.SubAgentFailed{
					EventHeader:  observe.NewEventHeader("SubAgentFailed", "", observe.NewSpanID(), ""),
					SubAgentID:   tk.ID,
					ErrorType:    "agent_error",
					ErrorMessage: e.Err.Error(),
				})
				failed = true
			}
		}

		if !failed {
			observe.GlobalTrace("if: !failed")
			tokensUsed := usage.InputTokens + usage.OutputTokens
			resultStr := result.String()
			_ = t.Tasks.Update(tk.ID, func(tt *task.Task) {
				tt.Status = task.TaskCompleted
				tt.Result = resultStr
				tt.TokensUsed = tokensUsed
			})
			t.Bus.Emit(observe.SubAgentCompleted{
				EventHeader: observe.NewEventHeader("SubAgentCompleted", "", observe.NewSpanID(), ""),
				SubAgentID:  tk.ID,
				DurationMs:  time.Since(startTime).Milliseconds(),
				TurnCount:   turnCount,
				Usage:       usage,
			})
		}

		t.cleanupWorktreeIfEmpty(wtPath, wtHeadCommit)
	}()

	ar := agentResult{
		Status:       "async_launched",
		Prompt:       in.Prompt,
		AgentID:      tk.ID,
		TaskID:       tk.ID,
		WorktreePath: wtPath,
		Branch:       wtBranch,
	}
	data, err := json.Marshal(ar)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{Content: fmt.Sprintf(\"Agent launched but failed to marshal ...")
		return tool.InvokeResult{Content: fmt.Sprintf("Agent launched but failed to marshal result: %v", err)}, nil
	}
	observe.GlobalTrace("return: tool.InvokeResult{Content: string(data)}, nil")
	return tool.InvokeResult{Content: string(data)}, nil
}

// createWorktree creates a git worktree for isolated agent work.
// Returns (worktreePath, branch, headCommit, error).
func (t *Tool) createWorktree(ctx context.Context, workDir, taskID string) (string, string, string, error) {
	observe.TraceCtx(ctx, "agent", "Tool.createWorktree", "enter")
	defer observe.TraceCtx(ctx, "agent", "Tool.createWorktree", "exit")
	slug := "agent-" + taskID + "-" + fmt.Sprintf("%d", time.Now().UnixMilli())
	flatSlug := worktree.FlattenSlug(slug)
	branch := "worktree-" + flatSlug
	dir := filepath.Join(workDir, ".gogent", "worktrees", flatSlug)

	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		observe.TraceCtx(ctx, "agent", "Tool.createWorktree", "if: err != nil")
		observe.TraceCtx(ctx, "agent", "Tool.createWorktree", "return: \"\", \"\", \"\", fmt.Errorf(\"create worktree parent dir: %w\", err)")
		return "", "", "", fmt.Errorf("create worktree parent dir: %w", err)
	}

	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "-B", branch, dir, "HEAD")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		observe.TraceCtx(ctx, "agent", "Tool.createWorktree", "if: err != nil")
		observe.TraceCtx(ctx, "agent", "Tool.createWorktree", "return: \"\", \"\", \"\", fmt.Errorf(\"git worktree add: %s: %w\", strings.TrimSpace(string(o...")
		return "", "", "", fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(output)), err)
	}

	revCmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD")
	revOut, err := revCmd.Output()
	if err != nil {
		observe.TraceCtx(ctx, "agent", "Tool.createWorktree", "if: err != nil")
		observe.TraceCtx(ctx, "agent", "Tool.createWorktree", "return: \"\", \"\", \"\", fmt.Errorf(\"git rev-parse HEAD: %w\", err)")
		return "", "", "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	headCommit := strings.TrimSpace(string(revOut))
	observe.TraceCtx(ctx, "agent", "Tool.createWorktree", "return: dir, branch, headCommit, nil")

	return dir, branch, headCommit, nil
}

// cleanupWorktreeIfEmpty removes a worktree if it has no changes.
// Does nothing if wtPath is empty.
func (t *Tool) cleanupWorktreeIfEmpty(wtPath, headCommit string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if wtPath == "" {
		observe.GlobalTrace("if: wtPath == \"\"")
		return
	}
	changed, err := worktree.HasChanges(wtPath, headCommit)
	if err != nil || changed {
		observe.GlobalTrace("if: err != nil || changed")
		return
	}

	cmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
	_ = cmd.Run()
}

func excludeTool(names []string, exclude string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if names == nil {
		observe.GlobalTrace("if: names == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	result := make([]string, 0, len(names))
	for _, n := range names {
		observe.GlobalTrace("range names")
		if n != exclude {
			observe.GlobalTrace("if: n != exclude")
			result = append(result, n)
		}
	}
	observe.GlobalTrace("return: result")
	return result
}
