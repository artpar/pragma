package lifecycletool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/lifecycle"
	"github.com/artpar/pragma/internal/lifecycle/bridge"
	"github.com/artpar/pragma/internal/lifecycle/definition"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/tool"
)

type lifecycleInput struct {
	Structure   string `json:"structure" desc:"Natural language description of the execution structure"`
	Prompt      string `json:"prompt" desc:"The task to execute within the lifecycle graph"`
	Description string `json:"description,omitempty" desc:"Alias for prompt"`
	System      string `json:"system,omitempty" desc:"Optional system prompt override"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
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

const toolDescription = `Execute a task using a structured workflow with evaluation gates, retry logic, or multi-perspective analysis.

This is an advanced tool for tasks that specifically need structured control flow. For most tasks, use tools directly or delegate via the Agent tool.

Use LifecycleRun when the task specifically needs:
- Retry with reflection: "fix the code, run tests, if tests fail reflect on what went wrong and retry up to 3 times"
- Multi-perspective analysis: "analyze from a security perspective, then from a performance perspective, then merge findings"
- Evaluation gates: "attempt a fix, verify it works before moving on"

Do NOT use for:
- Simple questions, file reads, or single tool calls — respond directly
- Multi-file investigation or research — use the Agent tool
- Straightforward implementation — use tools directly

Describe the execution structure in natural language and the system compiles it into an executable workflow graph.

Structure examples:
- "tool-calling loop" — LLM calls tools in a loop until done
- "plan steps first, execute each with tools, verify the result"
- "try fixing, run tests, if fail reflect and retry up to 3 times"
- "analyze from security perspective, then performance, then merge findings"`

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
	observe.GlobalTrace("return: \"LifecycleRun\"")
	return "LifecycleRun"
}

func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: toolDescription")
	return toolDescription
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
	observe.TraceCtx(ctx, "lifecycle", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "lifecycle", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "lifecycletool", "Tool.CheckPerm", "return: checker.Check(ctx, \"LifecycleRun\", \"\")")
	return checker.Check(ctx, "LifecycleRun", "")
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "lifecycle", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "lifecycle", "Tool.Invoke", "exit")

	var in lifecycleInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "lifecycletool", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "lifecycletool", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Prompt == "" && in.Description != "" {
		observe.TraceCtx(ctx, "lifecycletool", "Tool.Invoke", "if: in.Prompt == \"\" && in.Description != \"\"")
		in.Prompt = in.Description
	}
	if in.Prompt == "" {
		observe.TraceCtx(ctx, "lifecycletool", "Tool.Invoke", "if: in.Prompt == \"\"")
		observe.TraceCtx(ctx, "lifecycletool", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"prompt is required (hint: you sent 'descript...")
		return tool.InvokeResult{}, fmt.Errorf("prompt is required (hint: you sent 'description' — use 'prompt' instead)")
	}
	if in.Structure == "" {
		observe.TraceCtx(ctx, "lifecycletool", "Tool.Invoke", "if: in.Structure == \"\"")
		observe.TraceCtx(ctx, "lifecycletool", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"structure is required\")")
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
		observe.TraceCtx(ctx, "lifecycletool", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "lifecycletool", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"resolve graph: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("resolve graph: %w", err)
	}

	initialState := t.buildInitialState(in, snap)

	// Get progress reporter from state if available (optional interface pattern).
	var progressCh tool.ProgressReporter
	if ps, ok := state.(tool.ProgressSource); ok {
		observe.TraceCtx(ctx, "lifecycletool", "Tool.Invoke", "if: ok")
		progressCh = ps.Progress()
	}

	executor := lifecycle.NewExecutor(graph, lifecycle.WithEventBus(t.Bus))
	events := executor.Stream(ctx, initialState)

	var finalState lifecycle.State
	var runErr error
	for ev := range events {
		observe.TraceCtx(ctx, "lifecycletool", "Tool.Invoke", "range events")

		if progressCh != nil {
			observe.TraceCtx(ctx, "lifecycletool", "Tool.Invoke", "if: progressCh != nil")
			pe := tool.ProgressEvent{
				Step:     ev.Step,
				Node:     ev.Node,
				Nodes:    ev.Nodes,
				Status:   ev.Type,
				Duration: ev.Duration,
				FromNode: ev.FromNode,
				ToNode:   ev.ToNode,
				RouteKey: ev.RouteKey,
			}
			if ev.Err != nil {
				observe.TraceCtx(ctx, "lifecycletool", "Tool.Invoke", "if: ev.Err != nil")
				pe.Error = ev.Err.Error()
			}
			progressCh <- pe
		}
		if ev.Type == "completed" {
			observe.TraceCtx(ctx, "lifecycletool", "Tool.Invoke", "if: ev.Type == \"completed\"")
			finalState = ev.State
			runErr = ev.Err
		}
	}

	observe.TraceCtx(ctx, "lifecycletool", "Tool.Invoke", "return: t.buildResult(finalState, runErr)")
	return t.buildResult(finalState, runErr)
}

func (t *Tool) resolveGraph(ctx context.Context, in lifecycleInput, infra bridge.Infra) (*lifecycle.Graph, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	modelID := t.SecondaryModel
	if modelID == "" {
		observe.TraceCtx(ctx, "lifecycletool", "Tool.resolveGraph", "if: modelID == \"\"")
		modelID = t.Store.Snapshot().Model
	}

	def, err := bridge.GenerateGraph(ctx, t.Provider, t.Bus, modelID, in.Structure)
	if err != nil {
		observe.TraceCtx(ctx, "lifecycletool", "Tool.resolveGraph", "if: err != nil")
		observe.TraceCtx(ctx, "lifecycletool", "Tool.resolveGraph", "return: nil, fmt.Errorf(\"generate graph: %w\", err)")
		return nil, fmt.Errorf("generate graph: %w", err)
	}

	if def.Graph.Reducers == nil {
		observe.TraceCtx(ctx, "lifecycletool", "Tool.resolveGraph", "if: def.Graph.Reducers == nil")
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
		observe.TraceCtx(ctx, "lifecycletool", "Tool.resolveGraph", "if: err != nil")
		observe.TraceCtx(ctx, "lifecycletool", "Tool.resolveGraph", "return: nil, fmt.Errorf(\"resolve generated graph: %w\", err)")
		return nil, fmt.Errorf("resolve generated graph: %w", err)
	}
	observe.TraceCtx(ctx, "lifecycletool", "Tool.resolveGraph", "return: g, nil")
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
		observe.GlobalTrace("if: in.System != \"\"")
		sys = model.SystemPrompt{Blocks: []model.SystemBlock{{Text: in.System, Cacheable: true}}}
	}
	observe.GlobalTrace("return: lifecycle.State{\n\tbridge.KeyMessages:\t[]model.Message{userMsg},\n\tbridge.KeySy...")

	allDefs := t.Registry.ToolDefs()
	filteredDefs := make([]model.ToolDef, 0, len(allDefs))
	for _, td := range allDefs {
		observe.GlobalTrace("range allDefs")
		if td.Name == "LifecycleRun" {
			observe.GlobalTrace("if: td.Name == \"LifecycleRun\"")
			continue
		}
		filteredDefs = append(filteredDefs, td)
	}
	observe.GlobalTrace("return: lifecycle.State{\n\tbridge.KeyMessages:\t[]model.Message{userMsg},\n\tbridge.KeySy...")

	return lifecycle.State{
		bridge.KeyMessages:  []model.Message{userMsg},
		bridge.KeySystem:    sys,
		bridge.KeyModelID:   snap.Model,
		bridge.KeyMaxTokens: snap.MaxTokens,
		bridge.KeyTools:     filteredDefs,
	}
}

// toolCallRecord captures one actual tool invocation for verification.
type toolCallRecord struct {
	Name    string `json:"name"`
	Input   string `json:"input"`
	Output  string `json:"output"`
	IsError bool   `json:"is_error,omitempty"`
}

func (t *Tool) buildResult(finalState lifecycle.State, runErr error) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	status := "completed"
	var errMsg string
	if runErr != nil {
		observe.GlobalTrace("if: runErr != nil")
		status = "failed"
		errMsg = runErr.Error()
	}

	// Extract the LLM's narrative (last assistant text).
	var resultText string
	msgs := bridge.Messages(finalState)
	for i := len(msgs) - 1; i >= 0; i-- {
		observe.GlobalTrace("for: i >= 0")
		if msgs[i].Role == model.RoleAssistant {
			observe.GlobalTrace("if: msgs[i].Role == model.RoleAssistant")
			var parts []string
			for _, part := range msgs[i].Content {
				observe.GlobalTrace("range msgs[i].Content")
				if tp, ok := part.(model.TextPart); ok && tp.Text != "" {
					observe.GlobalTrace("if: ok && tp.Text != \"\"")
					parts = append(parts, tp.Text)
				}
			}
			if len(parts) > 0 {
				observe.GlobalTrace("if: len(parts) > 0")
				resultText = strings.Join(parts, "\n")
				break
			}
		}
	}

	// Extract actual tool call receipts from conversation history.
	// This is verified data — what tools actually ran and their real results.
	var toolCalls []toolCallRecord
	modifiedSet := make(map[string]bool)
	readSet := make(map[string]bool)

	for mi := 0; mi < len(msgs); mi++ {
		observe.GlobalTrace("for: mi < len(msgs)")
		msg := msgs[mi]
		if msg.Role != model.RoleAssistant {
			observe.GlobalTrace("if: msg.Role != model.RoleAssistant")
			continue
		}

		// Collect tool calls from this assistant message.
		var calls []model.ToolCallPart
		for _, part := range msg.Content {
			observe.GlobalTrace("range msg.Content")
			if tc, ok := part.(model.ToolCallPart); ok {
				observe.GlobalTrace("if: ok (ToolCallPart)")
				calls = append(calls, tc)
			}
		}
		if len(calls) == 0 {
			observe.GlobalTrace("if: len(calls) == 0")
			continue
		}

		// Find matching results in the next user message.
		resultMap := make(map[string]model.ToolResultPart)
		if mi+1 < len(msgs) && msgs[mi+1].Role == model.RoleUser {
			observe.GlobalTrace("if: mi+1 < len(msgs) && msgs[mi+1].Role == model.RoleUser")
			for _, part := range msgs[mi+1].Content {
				observe.GlobalTrace("range msgs[mi+1].Content")
				if tr, ok := part.(model.ToolResultPart); ok {
					observe.GlobalTrace("if: ok (ToolResultPart)")
					resultMap[tr.ToolCallID] = tr
				}
			}
		}

		for _, tc := range calls {
			observe.GlobalTrace("range calls")
			rec := toolCallRecord{
				Name:  tc.Name,
				Input: truncate(string(tc.Input), 500),
			}
			if tr, ok := resultMap[tc.ID]; ok {
				observe.GlobalTrace("if: ok (result matched)")
				rec.Output = truncate(tr.Content, 500)
				rec.IsError = tr.IsError

				// Only track files for successful tool calls.
				if !tr.IsError {
					observe.GlobalTrace("if: !tr.IsError")
					trackFiles(tc, modifiedSet, readSet)
				}
			}
			toolCalls = append(toolCalls, rec)
		}
	}

	var filesModified, filesRead []string
	for f := range modifiedSet {
		observe.GlobalTrace("range modifiedSet")
		filesModified = append(filesModified, f)
	}
	for f := range readSet {
		observe.GlobalTrace("range readSet")
		filesRead = append(filesRead, f)
	}

	type result struct {
		Status        string           `json:"status"`
		Result        string           `json:"result,omitempty"`
		Steps         int              `json:"steps"`
		Passed        bool             `json:"passed,omitempty"`
		Score         float64          `json:"score,omitempty"`
		Reflections   []string         `json:"reflections,omitempty"`
		ToolCalls     []toolCallRecord `json:"tool_calls,omitempty"`
		FilesModified []string         `json:"files_modified,omitempty"`
		FilesRead     []string         `json:"files_read,omitempty"`
		Error         string           `json:"error,omitempty"`
		Note          string           `json:"note,omitempty"`
	}

	turnCount, _ := finalState[bridge.KeyTurnCount].(int)

	var note string
	if status == "completed" {
		observe.GlobalTrace("if: status == \"completed\"")
		note = "Cross-check the 'result' narrative against 'tool_calls' and 'files_modified'. If the narrative claims edits but files_modified is empty, report that no changes were actually made."
	}

	out, _ := json.Marshal(result{
		Status:        status,
		Result:        resultText,
		Steps:         turnCount,
		Passed:        bridge.Passed(finalState),
		Score:         bridge.Score(finalState),
		Reflections:   bridge.Reflections(finalState),
		ToolCalls:     toolCalls,
		FilesModified: filesModified,
		FilesRead:     filesRead,
		Error:         errMsg,
		Note:          note,
	})
	observe.GlobalTrace("return: tool.InvokeResult{Content: string(out)}, nil")

	return tool.InvokeResult{Content: string(out)}, nil
}

// truncate limits s to maxLen runes, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// trackFiles extracts file_path from Edit/Write/Read tool call inputs.
func trackFiles(tc model.ToolCallPart, modified, read map[string]bool) {
	var fp struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(tc.Input, &fp); err != nil || fp.FilePath == "" {
		return
	}
	switch tc.Name {
	case "Edit", "Write":
		modified[fp.FilePath] = true
	case "Read":
		read[fp.FilePath] = true
	}
}
