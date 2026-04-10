package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/query"
	"github.com/artpar/gogent/internal/task"
	"github.com/artpar/gogent/internal/tool"
)

type AgentInput struct {
	Prompt      string `json:"prompt" desc:"The task for the sub-agent to perform"`
	Description string `json:"description" desc:"A short (3-5 word) description of the task"`
	Model       string `json:"model,omitempty" desc:"Optional model override for the sub-agent"`
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
		}
	}
}`)

type agentResult struct {
	Status     string `json:"status"`
	Prompt     string `json:"prompt"`
	Result     string `json:"result"`
	TokensUsed int    `json:"tokens_used,omitempty"`
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

func (t *Tool) Name() string                { return "Agent" }
func (t *Tool) Description() string          { return "Launch a sub-agent to handle a complex task autonomously." }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, "Agent", "")
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in AgentInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Prompt == "" {
		return tool.InvokeResult{}, fmt.Errorf("prompt is required")
	}

	// Fork parent conversation
	snapshot := t.Store.Snapshot()
	forkedConv := snapshot.Conversation.Fork(model.NewUUID())

	// Scope tools: nil means EngineFactory uses all tools minus Agent
	scopedTools := excludeTool(nil, "Agent")

	// Track as a task
	subject := in.Description
	if subject == "" {
		subject = in.Prompt
		if len(subject) > 80 {
			subject = subject[:80] + "..."
		}
	}
	tk := t.Tasks.Create(subject, in.Prompt)
	t.Tasks.Update(tk.ID, func(tt *task.Task) {
		tt.Status = task.TaskRunning
	})

	// Create sub-engine
	engine, _ := t.EngineFactory(forkedConv, scopedTools, in.Model)

	// Run sub-engine synchronously
	events := engine.Run(ctx, in.Prompt)

	var result strings.Builder
	var tokensUsed int

	for ev := range events {
		switch e := ev.(type) {
		case query.TextEvent:
			result.WriteString(e.Text)
		case query.TurnCompleteEvent:
			tokensUsed = e.Response.Usage.InputTokens + e.Response.Usage.OutputTokens
		case query.ErrorEvent:
			t.Tasks.Update(tk.ID, func(tt *task.Task) {
				tt.Status = task.TaskFailed
				tt.Error = e.Err.Error()
			})
			return tool.InvokeResult{
				Content: fmt.Sprintf("Agent failed: %v", e.Err),
			}, nil
		}
	}

	// Mark task complete
	resultStr := result.String()
	t.Tasks.Update(tk.ID, func(tt *task.Task) {
		tt.Status = task.TaskCompleted
		tt.Result = resultStr
		tt.TokensUsed = tokensUsed
	})

	ar := agentResult{
		Status:     "completed",
		Prompt:     in.Prompt,
		Result:     resultStr,
		TokensUsed: tokensUsed,
	}
	data, _ := json.Marshal(ar)
	return tool.InvokeResult{Content: string(data)}, nil
}

func excludeTool(names []string, exclude string) []string {
	if names == nil {
		return nil // nil means "all except excluded" — EngineFactory handles this
	}
	result := make([]string, 0, len(names))
	for _, n := range names {
		if n != exclude {
			result = append(result, n)
		}
	}
	return result
}
