package repl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
)

// PrimitiveToolNames is the set of tools hidden from the LLM when REPL mode is on.
// These tools remain callable by the REPL tool via Registry.Get().
var PrimitiveToolNames = map[string]bool{
	"Read":         true,
	"Write":        true,
	"Edit":         true,
	"Glob":         true,
	"Grep":         true,
	"Bash":         true,
	"NotebookEdit": true,
	"Agent":        true,
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
	"required": ["operations"],
	"properties": {
		"operations": {
			"type": "array",
			"description": "Sequence of tool operations to execute",
			"items": {
				"type": "object",
	"additionalProperties": false,
				"required": ["tool", "input"],
				"properties": {
					"tool": {
						"type": "string",
						"description": "Name of the available primitive tool to invoke"
					},
					"input": {
						"type": "object",
	"additionalProperties": false,
						"description": "Input parameters for the tool"
					}
				}
			}
		}
	}
}`)

// Tool implements the REPL tool that wraps primitive tools into a single
// batched execution interface. When enabled, primitive tools are hidden from
// the LLM's tool list but remain callable through this tool.
type Tool struct {
	Registry     *tool.Registry
	Orchestrator *tool.Orchestrator
	Bus          *observe.EventBus
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"REPL\"")
	return "REPL"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: replDescription")
	return replDescription
}

const replDescription = `Execute one or more primitive tool operations in sequence.

When REPL mode is enabled, individual primitive tools are batched through this
interface. Each operation specifies a tool name and its input parameters.

Operations execute sequentially. If one fails, subsequent operations still execute.
Results are returned as a combined output with separators between each operation.`

func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: false, Destructive: true}")
	return tool.ToolFlags{ReadOnly: false, Concurrent: false, Destructive: true}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "repl", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "repl", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "repl", "Tool.CheckPerm", "return: checker.Check(ctx, \"REPL\", \"\")")
	return checker.Check(ctx, "REPL", "")
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "repl", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "repl", "Tool.Invoke", "exit")

	var in struct {
		Operations []struct {
			Tool  string          `json:"tool"`
			Input json.RawMessage `json:"input"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "repl", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "repl", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if len(in.Operations) == 0 {
		observe.TraceCtx(ctx, "repl", "Tool.Invoke", "if: len(in.Operations) == 0")
		observe.TraceCtx(ctx, "repl", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"at least one operation is required\")")
		return tool.InvokeResult{}, fmt.Errorf("at least one operation is required")
	}

	var calls []model.ToolCallPart
	var callIndexes []int
	results := make([]string, len(in.Operations))

	for i, op := range in.Operations {
		observe.TraceCtx(ctx, "repl", "Tool.Invoke", "range in.Operations")
		if err := ctx.Err(); err != nil {
			observe.TraceCtx(ctx, "repl", "Tool.Invoke", "if: err != nil")
			for j := i; j < len(in.Operations); j++ {
				results[j] = fmt.Sprintf("[%d] %s: cancelled", j, in.Operations[j].Tool)
			}
			break
		}

		if !PrimitiveToolNames[op.Tool] {
			observe.TraceCtx(ctx, "repl", "Tool.Invoke", "if: !PrimitiveToolNames[op.Tool]")
			results[i] = fmt.Sprintf("[%d] %s: error: not a REPL-allowed tool", i, op.Tool)
			continue
		}

		calls = append(calls, model.ToolCallPart{
			ID:    fmt.Sprintf("repl-%d", i),
			Name:  op.Tool,
			Input: op.Input,
		})
		callIndexes = append(callIndexes, i)
	}

	var allSupplements []model.ContentPart
	if len(calls) > 0 {
		if t.Orchestrator == nil {
			return tool.InvokeResult{}, fmt.Errorf("REPL orchestrator is unavailable")
		}
		execResult := t.Orchestrator.Execute(ctx, calls, state)
		allSupplements = append(allSupplements, execResult.Supplements...)
		for j, part := range execResult.Results {
			i := callIndexes[j]
			if part.IsError {
				results[i] = fmt.Sprintf("[%d] %s: error: %s", i, calls[j].Name, part.Content)
				continue
			}
			results[i] = fmt.Sprintf("[%d] %s:\n%s", i, calls[j].Name, part.Content)
		}
	}
	observe.TraceCtx(ctx, "repl", "Tool.Invoke", "return: tool.InvokeResult{\n\tContent:\tstrings.Join(results, \"\\n---\\n\"),\n\tSupplements:\t...")

	return tool.InvokeResult{
		Content:     strings.Join(results, "\n---\n"),
		Supplements: allSupplements,
	}, nil
}
