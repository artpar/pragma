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

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/lifecycle"
	"github.com/artpar/pragma/internal/lifecycle/bridge"
	"github.com/artpar/pragma/internal/lifecycle/definition"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/task"
	"github.com/artpar/pragma/internal/tool"
	"github.com/artpar/pragma/internal/tools/worktree"
)

// AgentInput defines the parameters for the Agent tool.
type AgentInput struct {
	Prompt          string `json:"prompt" desc:"The task for the sub-agent to perform"`
	Description     string `json:"description" desc:"A short (3-5 word) description of the task"`
	Model           string `json:"model,omitempty" desc:"Optional model override for the sub-agent"`
	RunInBackground bool   `json:"run_in_background,omitempty" desc:"Run the agent asynchronously in the background"`
	Isolation       string `json:"isolation,omitempty" desc:"Isolation mode: 'worktree' for git worktree isolation, or empty for shared workspace"`
	Structure       string `json:"structure,omitempty" desc:"Natural language description of the execution structure for this sub-agent"`
	Teammate        bool   `json:"teammate,omitempty" desc:"If true, agent runs as a persistent teammate that stays alive to receive messages via SendMessage"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
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
		},
		"structure": {
			"type": "string",
			"description": "Natural language description of the execution structure for this sub-agent. When provided, the sub-agent executes as a structured workflow with evaluation gates or retry logic. Examples: 'try fixing, run tests, if fail reflect and retry 3x', 'plan steps first, execute each, verify result before next'."
		},
		"teammate": {
			"type": "boolean",
			"description": "If true, agent runs as a persistent teammate that stays alive to receive messages via SendMessage. Requires a description (used as the teammate name for message routing). Returns immediately with a task ID."
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
	EngineFactory  EngineFactory
	Store          *app.StateStore // parent store — for forking the conversation
	Tasks          *task.Registry
	Bus            *observe.EventBus
	Provider       provider.Provider // for lifecycle graph generation
	SecondaryModel string            // cheaper model for graph compilation
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
	observe.GlobalTrace("return: \"Launch a new agent to handle complex, multi-step tasks autonomously...\"")
	observe.GlobalTrace("return: agentDescription")
	return agentDescription
}

const agentDescription = `Launch a new agent to handle complex, multi-step tasks autonomously.

This tool launches specialized agents (subprocesses) that autonomously handle complex tasks. Each agent type has specific capabilities and tools available to it.

When using this tool, specify a subagent_type parameter to select which agent type to use. If omitted, the general-purpose agent is used.

## When to use this tool

Use this tool when work benefits from autonomous exploration, parallel execution, or isolated context:
- **Multi-file investigation**: understanding how a feature works across the codebase, tracing call chains, reading 5+ files to build understanding
- **Parallel subtasks**: launch multiple agents concurrently for independent pieces of work (e.g., "fix tests in pkg A" + "fix tests in pkg B" + "update docs")
- **Research and analysis**: exploring unfamiliar code, finding all usages of a pattern, auditing for issues across the codebase
- **Isolated changes**: use worktree isolation for risky refactors that might need to be discarded
- **Delegated implementation**: when you have a clear task specification and want autonomous execution without consuming main context

When NOT to use this tool:
- For reading a specific known file — use an available file-reading capability directly
- For a single targeted search — use an available search capability directly

Usage notes:
- Always include a short description (3-5 words) summarizing what the agent will do
- Launch multiple agents concurrently whenever possible, to maximize performance; to do that, use a single message with multiple tool uses
- When the agent is done, it will return a single message back to you. The result returned by the agent is not visible to the user. To show the user the result, you should send a text message back to the user with a concise summary of the result.
- You can optionally run agents in the background using the run_in_background parameter. When an agent runs in the background, you will be automatically notified when it completes — do NOT sleep, poll, or proactively check on its progress. Continue with other work or respond to the user instead.
- **Foreground vs background**: Use foreground (default) when you need the agent's results before you can proceed — e.g., research agents whose findings inform your next steps. Use background when you have genuinely independent work to do in parallel.
- To continue a previously spawned agent, use the available follow-up messaging capability with the agent's ID or name. The agent resumes with its full context preserved. Each invocation starts fresh — provide a complete task description.
- Provide clear, detailed prompts so the agent can work autonomously and return exactly the information you need.
- The agent's outputs should generally be trusted
- Clearly tell the agent whether you expect it to write code or just to do research (search, file reads, web fetches, etc.), since it is not aware of the user's intent
- If the user specifies that they want you to run agents "in parallel", you MUST send a single message with multiple tool-use content blocks.
- You can optionally set ` + "`isolation: \"worktree\"`" + ` to run the agent in a temporary git worktree, giving it an isolated copy of the repository. The worktree is automatically cleaned up if the agent makes no changes; if changes are made, the worktree path and branch are returned in the result.

## Writing the prompt

Brief the agent like a smart colleague who just walked into the room — it hasn't seen this conversation, doesn't know what you've tried, doesn't understand why this task matters.
- Explain what you're trying to accomplish and why.
- Describe what you've already learned or ruled out.
- Give enough context about the surrounding problem that the agent can make judgment calls rather than just following a narrow instruction.
- If you need a short response, say so ("report in under 200 words").
- Lookups: hand over the exact command. Investigations: hand over the question — prescribed steps become dead weight when the premise is wrong.

Terse command-style prompts produce shallow, generic work.

**Never delegate understanding.** Don't write "based on your findings, fix the bug" or "based on the research, implement it." Those phrases push synthesis onto the agent instead of doing it yourself. Write prompts that prove you understood: include file paths, line numbers, what specifically to change.`

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
	if err := t.Tasks.Update(tk.ID, func(tt *task.Task) {
		tt.Status = task.TaskRunning
		tt.AgentName = subject
	}); err != nil {
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: err != nil")
		t.Bus.Emit(observe.ErrorOccurred{
			EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
			Severity:     "warn",
			Component:    "agent",
			ErrorType:    "task_update_failed",
			ErrorMessage: fmt.Sprintf("set task %s to running: %v", tk.ID, err),
		})
	}

	// Set up worktree isolation if requested
	var wtPath, wtBranch, wtHeadCommit string
	if in.Isolation == "worktree" {
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: in.Isolation == \"worktree\"")
		var err error
		wtPath, wtBranch, wtHeadCommit, err = t.createWorktree(ctx, state.WorkDir(), tk.ID)
		if err != nil {
			observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: err != nil")
			t.updateTask(tk.ID, func(tt *task.Task) {
				tt.Status = task.TaskFailed
				tt.Error = err.Error()
			})
			observe.TraceCtx(ctx, "agent", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"create worktree: %w\", err)")
			return tool.InvokeResult{}, fmt.Errorf("create worktree: %w", err)
		}
	}

	engine, subStore := t.EngineFactory(forkedConv, scopedTools, in.Model)

	if in.Teammate {
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: in.Teammate")
		engine.SetTaskRegistry(t.Tasks)
		engine.SetTaskID(tk.ID)
	}

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

	// Extract progress reporter for real-time agent visibility (ADR-043).
	// Same optional interface pattern as LifecycleRun (ADR-042).
	var progressCh tool.ProgressReporter
	if ps, ok := state.(tool.ProgressSource); ok {
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: ok")
		progressCh = ps.Progress()
	}

	if in.Structure != "" {
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: in.Structure != \"\"")
		graph, err := t.compileStructure(ctx, in.Structure, engine, subStore)
		if err != nil {
			observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: err != nil")
			t.updateTask(tk.ID, func(tt *task.Task) {
				tt.Status = task.TaskFailed
				tt.Error = err.Error()
			})
			t.cleanupWorktreeIfEmpty(wtPath, wtHeadCommit)
			observe.TraceCtx(ctx, "agent", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"compile structure: %w\", err)")
			return tool.InvokeResult{}, fmt.Errorf("compile structure: %w", err)
		}
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "return: t.runGraphSync(ctx, tk.ID, engine, graph, in, progressCh, subject, wtPath, wtBranch, wtHeadCommit)")
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "return: t.runGraphSync(ctx, tk.ID, engine, graph, in, progressCh, subject, wtPath, wt...")
		return t.runGraphSync(ctx, tk.ID, engine, graph, in, progressCh, subject, wtPath, wtBranch, wtHeadCommit)
	}

	if in.Teammate {
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: in.Teammate")
		if subject == "" {
			observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: subject == \"\"")
			subject = "teammate"
		}
		emitAgentProgress(progressCh, tk.ID, subject, "initializing", 0, 0, "", true)
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "return: t.runTeammate(tk.ID, engine, in, progressCh, subject)")
		return t.runTeammate(tk.ID, engine, in, progressCh, subject)
	}

	if in.RunInBackground {
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "if: in.RunInBackground")
		emitAgentProgress(progressCh, tk.ID, subject, "initializing", 0, 0, "", true)
		observe.TraceCtx(ctx, "agent", "Tool.Invoke", "return: t.runBackground(tk.ID, engine, subStore, in, wtPath, wtBranch, wtHeadCommit)")
		return t.runBackground(tk.ID, engine, subStore, in, subject, wtPath, wtBranch, wtHeadCommit)
	}
	observe.TraceCtx(ctx, "agent", "Tool.Invoke", "return: t.runSync(ctx, tk.ID, engine, in, progressCh, subject, wtPath, wtBranch, wtHeadCommit)")
	observe.TraceCtx(ctx, "agent", "Tool.Invoke", "return: t.runSync(ctx, tk.ID, engine, in, progressCh, subject, wtPath, wtBranch, wtHe...")
	return t.runSync(ctx, tk.ID, engine, in, progressCh, subject, wtPath, wtBranch, wtHeadCommit)
}

// runSync runs the sub-agent synchronously and returns the result.
func (t *Tool) runSync(
	ctx context.Context,
	taskID string,
	engine *query.Engine,
	in AgentInput,
	progressCh tool.ProgressReporter,
	subject string,
	wtPath, wtBranch, wtHeadCommit string,
) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "agent", "Tool.runSync", "enter")
	defer observe.TraceCtx(ctx, "agent", "Tool.runSync", "exit")
	startTime := time.Now()
	events := engine.Run(ctx, in.Prompt)

	emitAgentProgress(progressCh, taskID, subject, "initializing", 0, 0, "", false)

	drain := t.drainAgentRunEvents(ctx, events, taskID, subject, progressCh, false)
	if drain.Err != nil {
		t.failAgentTask(taskID, "agent_error", drain.Err)
		t.cleanupWorktreeIfEmpty(wtPath, wtHeadCommit)
		return tool.InvokeResult{Content: fmt.Sprintf("Agent failed: %v", drain.Err)}, nil
	}
	emitAgentProgress(progressCh, taskID, subject, "completed", drain.ToolCount,
		drain.ProgressTokens(), drain.LastToolName, false)
	resultStr := drain.Result
	t.completeAgentTask(taskID, resultStr, drain.TokensUsed(), time.Since(startTime), drain.TurnCount, drain.Usage)

	t.cleanupWorktreeIfEmpty(wtPath, wtHeadCommit)

	ar := agentResult{
		Status:       "completed",
		Prompt:       in.Prompt,
		Result:       resultStr,
		TokensUsed:   drain.TokensUsed(),
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

type agentRunDrain struct {
	Result                 string
	Usage                  model.TokenUsage
	TurnCount              int
	ToolCount              int
	LastToolName           string
	LatestInputTokens      int
	CumulativeOutputTokens int
	Err                    error
}

func (r agentRunDrain) TokensUsed() int {
	return r.Usage.InputTokens + r.Usage.OutputTokens
}

func (r agentRunDrain) ProgressTokens() int {
	return r.LatestInputTokens + r.CumulativeOutputTokens
}

func (t *Tool) drainAgentRunEvents(
	ctx context.Context,
	events <-chan query.LoopEvent,
	taskID, subject string,
	progressCh tool.ProgressReporter,
	background bool,
) agentRunDrain {
	observe.TraceCtx(ctx, "agent", "Tool.drainAgentRunEvents", "enter")
	defer observe.TraceCtx(ctx, "agent", "Tool.drainAgentRunEvents", "exit")
	var result strings.Builder
	var out agentRunDrain
	for ev := range events {
		observe.TraceCtx(ctx, "agent", "Tool.drainAgentRunEvents", "range events")
		switch e := ev.(type) {
		case query.TextEvent:
			observe.TraceCtx(ctx, "agent", "Tool.drainAgentRunEvents", "typecase: query.TextEvent")
			result.WriteString(e.Text)
		case query.ToolCallEvent:
			observe.TraceCtx(ctx, "agent", "Tool.drainAgentRunEvents", "typecase: query.ToolCallEvent")
			out.LastToolName = e.Call.Name
			emitAgentProgress(progressCh, taskID, subject, "running", out.ToolCount,
				out.ProgressTokens(), out.LastToolName, background)
		case query.ToolResultEvent:
			observe.TraceCtx(ctx, "agent", "Tool.drainAgentRunEvents", "typecase: query.ToolResultEvent")
			out.ToolCount++
			emitAgentProgress(progressCh, taskID, subject, "running", out.ToolCount,
				out.ProgressTokens(), out.LastToolName, background)
		case query.TurnCompleteEvent:
			observe.TraceCtx(ctx, "agent", "Tool.drainAgentRunEvents", "typecase: query.TurnCompleteEvent")
			out.TurnCount++
			out.Usage.InputTokens += e.Response.Usage.InputTokens
			out.Usage.OutputTokens += e.Response.Usage.OutputTokens
			out.Usage.CacheCreationInputTokens += e.Response.Usage.CacheCreationInputTokens
			out.Usage.CacheReadInputTokens += e.Response.Usage.CacheReadInputTokens
			out.LatestInputTokens = e.Response.Usage.InputTokens +
				e.Response.Usage.CacheCreationInputTokens + e.Response.Usage.CacheReadInputTokens
			out.CumulativeOutputTokens += e.Response.Usage.OutputTokens
			emitAgentProgress(progressCh, taskID, subject, "running", out.ToolCount,
				out.ProgressTokens(), out.LastToolName, background)
		case query.ErrorEvent:
			observe.TraceCtx(ctx, "agent", "Tool.drainAgentRunEvents", "typecase: query.ErrorEvent")
			out.Err = e.Err
			emitAgentProgress(progressCh, taskID, subject, "error", out.ToolCount,
				out.ProgressTokens(), out.LastToolName, background, e.Err.Error())
		}
	}
	out.Result = result.String()
	return out
}

func (t *Tool) failAgentTask(taskID, errorType string, err error) {
	t.updateTask(taskID, func(tt *task.Task) {
		tt.Status = task.TaskFailed
		tt.Error = err.Error()
	})
	t.Bus.Emit(observe.SubAgentFailed{
		EventHeader:  observe.NewEventHeader("SubAgentFailed", "", observe.NewSpanID(), ""),
		SubAgentID:   taskID,
		ErrorType:    errorType,
		ErrorMessage: err.Error(),
	})
}

func (t *Tool) completeAgentTask(taskID, result string, tokensUsed int, duration time.Duration, turnCount int, usage model.TokenUsage) {
	t.updateTask(taskID, func(tt *task.Task) {
		tt.Status = task.TaskCompleted
		tt.Result = result
		tt.TokensUsed = tokensUsed
	})
	t.Bus.Emit(observe.SubAgentCompleted{
		EventHeader: observe.NewEventHeader("SubAgentCompleted", "", observe.NewSpanID(), ""),
		SubAgentID:  taskID,
		DurationMs:  duration.Milliseconds(),
		TurnCount:   turnCount,
		Usage:       usage,
	})
}

// runBackground launches the sub-agent in a goroutine and returns immediately.
func (t *Tool) runBackground(
	taskID string,
	engine *query.Engine,
	subStore *app.StateStore,
	in AgentInput,
	subject string,
	wtPath, wtBranch, wtHeadCommit string,
) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	childCtx, cancelFn := context.WithCancel(context.Background())
	t.updateTask(taskID, func(tt *task.Task) {
		tt.Cancel = cancelFn
	})
	_ = subStore

	go func() {
		defer cancelFn()
		startTime := time.Now()

		events := engine.Run(childCtx, in.Prompt)
		drain := t.drainAgentRunEvents(childCtx, events, taskID, subject, nil, true)
		if drain.Err != nil {
			t.failAgentTask(taskID, "agent_error", drain.Err)
		} else {
			t.completeAgentTask(taskID, drain.Result, drain.TokensUsed(), time.Since(startTime), drain.TurnCount, drain.Usage)
		}

		t.cleanupWorktreeIfEmpty(wtPath, wtHeadCommit)
	}()

	ar := agentResult{
		Status:       "async_launched",
		Prompt:       in.Prompt,
		AgentID:      taskID,
		TaskID:       taskID,
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

// runTeammate launches a persistent teammate agent that stays alive to receive messages.
// Initial prompt runs first, then the agent waits for messages via the Notify channel.
// The teammate exits on context cancellation or ShutdownRequested.
func (t *Tool) runTeammate(
	taskID string,
	engine *query.Engine,
	in AgentInput,
	progressCh tool.ProgressReporter,
	subject string,
) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	childCtx, cancelFn := context.WithCancel(context.Background())
	t.updateTask(taskID, func(tt *task.Task) {
		tt.Cancel = cancelFn
		tt.AgentName = subject
	})

	notifyCh := t.Tasks.GetNotifyChannel(taskID)

	go func() {
		defer cancelFn()
		var totalUsage model.TokenUsage
		var totalToolCount int

		t.drainTeammateEvents(childCtx, engine.Run(childCtx, in.Prompt), taskID, subject, progressCh, &totalUsage, &totalToolCount)

		t.updateTask(taskID, func(tt *task.Task) {
			tt.IsIdle = true
			tt.IdleSince = time.Now()
			tt.TokensUsed = totalUsage.InputTokens + totalUsage.OutputTokens
		})

	waitLoop:
		for {
			if t.Tasks.IsShutdownRequested(taskID) {
				break
			}

			select {
			case <-childCtx.Done():
				t.updateTask(taskID, func(tt *task.Task) {
					tt.Status = task.TaskCancelled
				})
				return
			case <-notifyCh:
				if t.Tasks.IsShutdownRequested(taskID) {
					break waitLoop
				}
				msgs := t.Tasks.DrainPendingMessages(taskID)
				if len(msgs) > 0 {

					t.updateTask(taskID, func(tt *task.Task) {
						tt.IsIdle = false
						tt.IdleSince = time.Time{}
					})
					joined := strings.Join(msgs, "\n")
					emitAgentProgress(progressCh, taskID, subject, "running", totalToolCount,
						int(totalUsage.InputTokens+totalUsage.OutputTokens), "processing message", false)
					t.drainTeammateEvents(childCtx, engine.Run(childCtx, joined), taskID, subject, progressCh, &totalUsage, &totalToolCount)

					t.updateTask(taskID, func(tt *task.Task) {
						tt.IsIdle = true
						tt.IdleSince = time.Now()
						tt.TokensUsed = totalUsage.InputTokens + totalUsage.OutputTokens
					})
				}
			}
		}

		t.updateTask(taskID, func(tt *task.Task) {
			tt.Status = task.TaskCompleted
			tt.TokensUsed = totalUsage.InputTokens + totalUsage.OutputTokens
		})
		t.Bus.Emit(observe.SubAgentCompleted{
			EventHeader: observe.NewEventHeader("SubAgentCompleted", "", observe.NewSpanID(), ""),
			SubAgentID:  taskID,
			TurnCount:   0,
			Usage:       totalUsage,
		})
		emitAgentProgress(progressCh, taskID, subject, "completed", totalToolCount,
			int(totalUsage.InputTokens+totalUsage.OutputTokens), "", false)
	}()

	ar := agentResult{
		Status:  "teammate_launched",
		Prompt:  in.Prompt,
		AgentID: taskID,
		TaskID:  taskID,
	}
	data, _ := json.Marshal(ar)
	observe.GlobalTrace("return: tool.InvokeResult{Content: string(data)}, nil")
	return tool.InvokeResult{Content: string(data)}, nil
}

// drainTeammateEvents processes all events from a single engine run, updating usage and tool counts.
func (t *Tool) drainTeammateEvents(
	ctx context.Context,
	events <-chan query.LoopEvent,
	taskID, subject string,
	progressCh tool.ProgressReporter,
	usage *model.TokenUsage,
	toolCount *int,
) {
	observe.TraceCtx(ctx, "agent", "Tool.drainTeammateEvents", "enter")
	defer observe.TraceCtx(ctx, "agent", "Tool.drainTeammateEvents", "exit")
	drain := t.drainAgentRunEvents(ctx, events, taskID, subject, progressCh, false)
	usage.InputTokens += drain.Usage.InputTokens
	usage.OutputTokens += drain.Usage.OutputTokens
	usage.CacheCreationInputTokens += drain.Usage.CacheCreationInputTokens
	usage.CacheReadInputTokens += drain.Usage.CacheReadInputTokens
	*toolCount += drain.ToolCount
	if drain.Err != nil {
		t.Bus.Emit(observe.ErrorOccurred{
			EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
			Severity:     "error",
			Component:    "agent",
			ErrorType:    "teammate_error",
			ErrorMessage: fmt.Sprintf("teammate %q: %v", subject, drain.Err),
		})
	}
}

// updateTask applies a mutation to a task, emitting an error event on failure.
func (t *Tool) updateTask(taskID string, fn func(*task.Task)) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if err := t.Tasks.Update(taskID, fn); err != nil {
		observe.GlobalTrace("if: err != nil")
		t.Bus.Emit(observe.ErrorOccurred{
			EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
			Severity:     "warn",
			Component:    "agent",
			ErrorType:    "task_update_failed",
			ErrorMessage: fmt.Sprintf("update task %s: %v", taskID, err),
		})
	}
}

// createWorktree creates a git worktree for isolated agent work.
// Returns (worktreePath, branch, headCommit, error).
func (t *Tool) createWorktree(ctx context.Context, workDir, taskID string) (string, string, string, error) {
	observe.TraceCtx(ctx, "agent", "Tool.createWorktree", "enter")
	defer observe.TraceCtx(ctx, "agent", "Tool.createWorktree", "exit")
	slug := "agent-" + taskID + "-" + fmt.Sprintf("%d", time.Now().UnixMilli())
	flatSlug := worktree.FlattenSlug(slug)
	branch := "worktree-" + flatSlug
	dir := filepath.Join(workDir, ".pragma", "worktrees", flatSlug)

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

// compileStructure generates a lifecycle graph from a natural language description.
func (t *Tool) compileStructure(ctx context.Context, structure string, engine *query.Engine, subStore *app.StateStore) (*lifecycle.Graph, error) {
	observe.TraceCtx(ctx, "agent", "Tool.compileStructure", "enter")
	defer observe.TraceCtx(ctx, "agent", "Tool.compileStructure", "exit")

	modelID := t.SecondaryModel
	if modelID == "" {
		observe.TraceCtx(ctx, "agent", "Tool.compileStructure", "if: modelID == \"\"")
		modelID = t.Store.Snapshot().Model
	}

	def, err := bridge.GenerateGraph(ctx, t.Provider, t.Bus, modelID, structure)
	if err != nil {
		observe.TraceCtx(ctx, "agent", "Tool.compileStructure", "if: err != nil")
		observe.TraceCtx(ctx, "agent", "Tool.compileStructure", "return: nil, err")
		return nil, err
	}

	if def.Graph.Reducers == nil {
		observe.TraceCtx(ctx, "agent", "Tool.compileStructure", "if: def.Graph.Reducers == nil")
		def.Graph.Reducers = make(map[string]string)
	}
	def.Graph.Reducers["total_usage"] = "total_usage"

	snap := subStore.Snapshot()
	infra := bridge.Infra{
		Provider:     t.Provider,
		Orchestrator: engine.Orchestrator(),
		Registry:     engine.Registry(),
		Bus:          t.Bus,
		Cwd:          snap.CWD,
	}

	factory := bridge.NewNodeFactory(infra)
	opts := &definition.ResolveOptions{
		CustomReducers: map[string]lifecycle.ReducerFunc{
			"messages":    bridge.MessageReducer,
			"reflections": bridge.ReflectionReducer,
			"total_usage": bridge.UsageReducer,
		},
	}
	observe.TraceCtx(ctx, "agent", "Tool.compileStructure", "return: definition.Resolve(def, factory.Create, definition.DefaultRouterCreator(), opts)")

	return definition.Resolve(def, factory.Create, definition.DefaultRouterCreator(), opts)
}

// runGraphSync runs a lifecycle graph synchronously and returns the result.
func (t *Tool) runGraphSync(
	ctx context.Context,
	taskID string,
	engine *query.Engine,
	graph *lifecycle.Graph,
	in AgentInput,
	progressCh tool.ProgressReporter,
	subject string,
	wtPath, wtBranch, wtHeadCommit string,
) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "agent", "Tool.runGraphSync", "enter")
	defer observe.TraceCtx(ctx, "agent", "Tool.runGraphSync", "exit")
	startTime := time.Now()

	events := engine.RunGraph(ctx, graph, in.Prompt)

	emitAgentProgress(progressCh, taskID, subject, "initializing", 0, 0, "", false)

	drain := t.drainAgentRunEvents(ctx, events, taskID, subject, progressCh, false)
	if drain.Err != nil {
		t.failAgentTask(taskID, "graph_error", drain.Err)
		t.cleanupWorktreeIfEmpty(wtPath, wtHeadCommit)
		return tool.InvokeResult{Content: fmt.Sprintf("Agent graph failed: %v", drain.Err)}, nil
	}
	emitAgentProgress(progressCh, taskID, subject, "completed", drain.ToolCount,
		drain.ProgressTokens(), drain.LastToolName, false)
	resultStr := drain.Result
	t.completeAgentTask(taskID, resultStr, drain.TokensUsed(), time.Since(startTime), drain.TurnCount, drain.Usage)

	t.cleanupWorktreeIfEmpty(wtPath, wtHeadCommit)

	ar := agentResult{
		Status:       "completed",
		Prompt:       in.Prompt,
		Result:       resultStr,
		WorktreePath: wtPath,
		Branch:       wtBranch,
	}
	data, _ := json.Marshal(ar)
	observe.TraceCtx(ctx, "agent", "Tool.runGraphSync", "return: tool.InvokeResult{Content: string(data)}, nil")
	return tool.InvokeResult{Content: string(data)}, nil
}

// emitAgentProgress sends an agent progress event if the channel is available.
// No-op when progressCh is nil (ProgressSource not available).
func emitAgentProgress(ch tool.ProgressReporter, agentID, desc, status string,
	toolCount, tokenCount int, lastTool string, background bool, errMsg ...string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if ch == nil {
		observe.GlobalTrace("if: ch == nil")
		return
	}
	var errStr string
	if len(errMsg) > 0 {
		observe.GlobalTrace("if: len(errMsg) > 0")
		errStr = errMsg[0]
	}
	ch <- tool.ProgressEvent{
		Kind:        "agent",
		AgentID:     agentID,
		Description: desc,
		Status:      status,
		ToolCount:   toolCount,
		TokenCount:  tokenCount,
		LastTool:    lastTool,
		Background:  background,
		Error:       errStr,
	}
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
