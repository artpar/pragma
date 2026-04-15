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
	Structure string `json:"structure" desc:"Natural language description of the execution structure"`
	Prompt    string `json:"prompt" desc:"The task to execute within the lifecycle graph"`
	System    string `json:"system,omitempty" desc:"Optional system prompt override"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["prompt", "structure"],
	"properties": {
		"structure": {
			"type": "string",
			"description": "Natural language description of the execution structure you want. Describe the steps, evaluation gates, retry logic, and flow. The system compiles this into an executable workflow graph. Examples: 'tool-calling loop', 'plan steps first, execute each with tools, verify result', 'attempt with tools, evaluate, reflect on failure, retry'."
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

const toolDescription = `Execute a task using a structured workflow.

Describe the execution structure you want in natural language, and the system compiles it into an executable workflow graph. You do not need to know the graph format — just describe the steps, evaluation gates, and retry logic you need.

Structure examples:
- "tool-calling loop" — LLM calls tools in a loop until done
- "plan the refactoring steps first, then execute each step with tools, then verify the result"
- "try fixing the code, run the tests, if tests fail reflect on what went wrong and retry up to 3 times"
- "analyze from a security perspective, then from a performance perspective, then merge the findings"
- "attempt the task with tools, evaluate if it succeeded, if not reflect and try a different approach"`

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
	if in.Structure == "" {
		return tool.InvokeResult{}, fmt.Errorf("structure is required")
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

	modelID := t.SecondaryModel
	if modelID == "" {
		modelID = t.Store.Snapshot().Model
	}

	def, err := bridge.GenerateGraph(ctx, t.Provider, t.Bus, modelID, in.Structure)
	if err != nil {
		return nil, fmt.Errorf("generate graph: %w", err)
	}

	if def.Graph.Reducers == nil {
		def.Graph.Reducers = make(map[string]string)
	}
	def.Graph.Reducers["total_usage"] = "total_usage"
	def.Graph.Reducers["turn_count"] = "sum"

	factory := bridge.NewNodeFactory(infra)
	opts := &definition.ResolveOptions{
		CustomReducers: map[string]lifecycle.ReducerFunc{
			"messages":    bridge.MessageReducer,
			"reflections": bridge.ReflectionReducer,
			"total_usage": bridge.UsageReducer,
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

	sys := snap.Conversation.System
	if in.System != "" {
		sys = model.SystemPrompt{Blocks: []model.SystemBlock{{Text: in.System, Cacheable: true}}}
	}

	return lifecycle.State{
		bridge.KeyMessages:  []model.Message{userMsg},
		bridge.KeySystem:    sys,
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
		Note        string   `json:"note,omitempty"`
	}

	turnCount, _ := finalState[bridge.KeyTurnCount].(int)

	var note string
	if status == "completed" {
		note = "The structured workflow completed successfully. Present these findings to the user as-is — do not take additional actions unless the user explicitly asks."
	}

	out, _ := json.Marshal(result{
		Status:      status,
		Result:      resultText,
		Steps:       turnCount,
		Passed:      bridge.Passed(finalState),
		Score:       bridge.Score(finalState),
		Reflections: bridge.Reflections(finalState),
		Error:       errMsg,
		Note:        note,
	})

	return tool.InvokeResult{Content: string(out)}, nil
}
