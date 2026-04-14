package lifecycletool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/lifecycle"
	"github.com/artpar/gogent/internal/lifecycle/bridge"
	"github.com/artpar/gogent/internal/lifecycle/definition"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/provider"
	"github.com/artpar/gogent/internal/tool"
)

type lifecycleInput struct {
	Pattern   string `json:"pattern,omitempty" desc:"Built-in pattern shortcut"`
	Structure string `json:"structure,omitempty" desc:"Natural language description of the execution structure"`
	Prompt    string `json:"prompt" desc:"The task to execute within the lifecycle graph"`
	System    string `json:"system,omitempty" desc:"Optional system prompt override"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["prompt"],
	"properties": {
		"pattern": {
			"type": "string",
			"enum": ["react", "plan-execute", "reflexion"],
			"description": "Built-in pattern shortcut. Use instead of structure for common cases."
		},
		"structure": {
			"type": "string",
			"description": "Natural language description of the execution structure you want. Describe the steps, evaluation gates, retry logic, and flow. The system compiles this into an executable workflow graph."
		},
		"prompt": {
			"type": "string",
			"description": "The task to execute within the structured workflow"
		},
		"system": {
			"type": "string",
			"description": "Optional system prompt override for the workflow's LLM nodes"
		}
	}
}`)

const toolDescription = `Execute a task using a structured workflow instead of ad-hoc tool calling.

Describe the execution structure you want in natural language, and the system compiles it into an executable workflow graph. You do not need to know the graph format — just describe the steps, evaluation gates, and retry logic you need.

Examples of structure descriptions:
- "try fixing the code, run the tests, if tests fail reflect on what went wrong and retry up to 3 times"
- "plan the refactoring steps first, then execute each step with tools, then verify the result"
- "analyze from a security perspective, then from a performance perspective, then merge the findings"
- "attempt the task, evaluate if it succeeded, if not reflect and try a different approach"

Use this tool when:
- Your previous attempt at a task failed and you want structured retry with self-evaluation
- The task has clear phases that should execute in a guaranteed order
- You need evaluation gates between steps to verify progress
- You want forced self-critique before retrying a failed approach

Do NOT use for:
- Simple tasks that need 1-3 tool calls — just do them directly
- Research or exploration — use Agent instead
- Your first attempt at any task — try direct tool calls first

Built-in pattern shortcuts (use pattern instead of structure):
- react: standard tool-calling loop
- plan-execute: plan, execute, replan loop
- reflexion: attempt, evaluate, reflect on failure, retry`

// Tool implements the LifecycleRun tool for executing structured workflows.
type Tool struct {
	Provider       provider.Provider
	Orchestrator   *tool.Orchestrator
	Registry       *tool.Registry
	Bus            *observe.EventBus
	Store          *app.StateStore
	SecondaryModel string
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	return "LifecycleRun"
}

func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	return toolDescription
}

func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	return inputSchema
}

func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "lifecycle", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "lifecycle", "Tool.CheckPerm", "exit")
	return checker.Check(ctx, "LifecycleRun", "")
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "lifecycle", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "lifecycle", "Tool.Invoke", "exit")

	var in lifecycleInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Prompt == "" {
		return tool.InvokeResult{}, fmt.Errorf("prompt is required")
	}
	if in.Pattern != "" && in.Structure != "" {
		return tool.InvokeResult{}, fmt.Errorf("provide either pattern or structure, not both")
	}
	if in.Pattern == "" && in.Structure == "" {
		return tool.InvokeResult{}, fmt.Errorf("either pattern or structure is required")
	}

	snap := t.Store.Snapshot()
	infra := bridge.Infra{
		Provider:     t.Provider,
		Orchestrator: t.Orchestrator,
		Registry:     t.Registry,
		Bus:          t.Bus,
		Cwd:          snap.CWD,
	}

	graph, err := t.resolveGraph(ctx, in, infra)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("resolve graph: %w", err)
	}

	initialState := t.buildInitialState(in, snap)

	executor := lifecycle.NewExecutor(graph, lifecycle.WithEventBus(t.Bus))
	finalState, err := executor.Run(ctx, initialState)

	return t.buildResult(finalState, err)
}

func (t *Tool) resolveGraph(ctx context.Context, in lifecycleInput, infra bridge.Infra) (*lifecycle.Graph, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	if in.Pattern != "" {
		patterns := bridge.PatternMap(infra)
		g, ok := patterns[in.Pattern]
		if !ok {
			return nil, fmt.Errorf("unknown pattern %q (available: react, plan-execute, reflexion)", in.Pattern)
		}
		return g, nil
	}

	// Generate graph from natural language structure description
	modelID := t.SecondaryModel
	if modelID == "" {
		modelID = t.Store.Snapshot().Model
	}

	def, err := bridge.GenerateGraph(ctx, t.Provider, t.Bus, modelID, in.Structure)
	if err != nil {
		return nil, fmt.Errorf("generate graph: %w", err)
	}

	factory := bridge.NewNodeFactory(infra)
	opts := &definition.ResolveOptions{
		CustomReducers: map[string]lifecycle.ReducerFunc{
			"messages":    bridge.MessageReducer,
			"reflections": bridge.ReflectionReducer,
		},
	}

	g, err := definition.Resolve(def, factory.Create, definition.DefaultRouterCreator(), opts)
	if err != nil {
		return nil, fmt.Errorf("resolve generated graph: %w", err)
	}
	return g, nil
}

func (t *Tool) buildInitialState(in lifecycleInput, snap app.AppState) lifecycle.State {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	userMsg := model.Message{
		Role:    model.RoleUser,
		Content: []model.ContentPart{model.TextPart{Text: in.Prompt}},
	}

	sysText := "You are a helpful AI assistant."
	if in.System != "" {
		sysText = in.System
	}

	return lifecycle.State{
		bridge.KeyMessages:  []model.Message{userMsg},
		bridge.KeySystem:    model.SystemPrompt{Blocks: []model.SystemBlock{{Text: sysText, Cacheable: true}}},
		bridge.KeyModelID:   snap.Model,
		bridge.KeyMaxTokens: snap.MaxTokens,
		bridge.KeyTools:     t.Registry.ToolDefs(),
	}
}

func (t *Tool) buildResult(finalState lifecycle.State, runErr error) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	status := "completed"
	var errMsg string
	if runErr != nil {
		status = "failed"
		errMsg = runErr.Error()
	}

	var resultText string
	msgs := bridge.Messages(finalState)
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == model.RoleAssistant {
			var parts []string
			for _, part := range msgs[i].Content {
				if tp, ok := part.(model.TextPart); ok && tp.Text != "" {
					parts = append(parts, tp.Text)
				}
			}
			if len(parts) > 0 {
				resultText = strings.Join(parts, "\n")
				break
			}
		}
	}

	type result struct {
		Status      string   `json:"status"`
		Result      string   `json:"result,omitempty"`
		Steps       int      `json:"steps"`
		Passed      bool     `json:"passed,omitempty"`
		Score       float64  `json:"score,omitempty"`
		Reflections []string `json:"reflections,omitempty"`
		Error       string   `json:"error,omitempty"`
	}

	turnCount, _ := finalState[bridge.KeyTurnCount].(int)

	out, _ := json.Marshal(result{
		Status:      status,
		Result:      resultText,
		Steps:       turnCount,
		Passed:      bridge.Passed(finalState),
		Score:       bridge.Score(finalState),
		Reflections: bridge.Reflections(finalState),
		Error:       errMsg,
	})

	return tool.InvokeResult{Content: string(out)}, nil
}
