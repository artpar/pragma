package repl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
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
	"required": ["operations"],
	"properties": {
		"operations": {
			"type": "array",
			"description": "Sequence of tool operations to execute",
			"items": {
				"type": "object",
				"required": ["tool", "input"],
				"properties": {
					"tool": {
						"type": "string",
						"description": "Name of the tool to invoke (Read, Write, Edit, Glob, Grep, Bash, NotebookEdit, Agent)"
					},
					"input": {
						"type": "object",
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
	Registry *tool.Registry
	Bus      *observe.EventBus
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

When REPL mode is enabled, individual tools (Read, Write, Edit, Glob, Grep, Bash,
NotebookEdit, Agent) are batched through this interface. Each operation specifies
a tool name and its input parameters.

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

	var in struct {
		Operations []struct {
			Tool  string          `json:"tool"`
			Input json.RawMessage `json:"input"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(input, &in); err != nil || len(in.Operations) == 0 {
		observe.TraceCtx(ctx, "repl", "Tool.CheckPerm", "if: err != nil || len(in.Operations) == 0")
		observe.TraceCtx(ctx, "repl", "Tool.CheckPerm", "return: checker.Check(ctx, \"REPL\", \"\")")
		return checker.Check(ctx, "REPL", "")
	}

	if len(in.Operations) == 1 {
		observe.TraceCtx(ctx, "repl", "Tool.CheckPerm", "if: len(in.Operations) == 1")
		op := in.Operations[0]
		if !PrimitiveToolNames[op.Tool] {
			observe.TraceCtx(ctx, "repl", "Tool.CheckPerm", "if: !PrimitiveToolNames[op.Tool]")
			observe.TraceCtx(ctx, "repl", "Tool.CheckPerm", "return: permission.CheckResult{Decision: permission.DecisionDeny}")
			return permission.CheckResult{Decision: permission.DecisionDeny}
		}
		desc, ok := t.Registry.Get(op.Tool)
		if !ok {
			observe.TraceCtx(ctx, "repl", "Tool.CheckPerm", "if: !ok")
			observe.TraceCtx(ctx, "repl", "Tool.CheckPerm", "return: permission.CheckResult{Decision: permission.DecisionDeny}")
			return permission.CheckResult{Decision: permission.DecisionDeny}
		}
		observe.TraceCtx(ctx, "repl", "Tool.CheckPerm", "return: desc.CheckPerm(ctx, op.Input, checker)")
		return desc.CheckPerm(ctx, op.Input, checker)
	}
	observe.TraceCtx(ctx, "repl", "Tool.CheckPerm", "return: permission.CheckResult{Decision: permission.DecisionAsk}")

	return permission.CheckResult{Decision: permission.DecisionAsk}
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

	var results []string
	var allSupplements []model.ContentPart

	for i, op := range in.Operations {
		observe.TraceCtx(ctx, "repl", "Tool.Invoke", "range in.Operations")
		if err := ctx.Err(); err != nil {
			observe.TraceCtx(ctx, "repl", "Tool.Invoke", "if: err != nil")
			results = append(results, fmt.Sprintf("[%d] %s: cancelled", i, op.Tool))
			break
		}

		if !PrimitiveToolNames[op.Tool] {
			observe.TraceCtx(ctx, "repl", "Tool.Invoke", "if: !PrimitiveToolNames[op.Tool]")
			results = append(results, fmt.Sprintf("[%d] %s: error: not a REPL-allowed tool", i, op.Tool))
			continue
		}

		desc, ok := t.Registry.Get(op.Tool)
		if !ok {
			observe.TraceCtx(ctx, "repl", "Tool.Invoke", "if: !ok")
			results = append(results, fmt.Sprintf("[%d] %s: error: tool not found in registry", i, op.Tool))
			continue
		}

		observe.TraceCtx(ctx, "repl", "Tool.Invoke",
			fmt.Sprintf("executing operation %d: %s", i, op.Tool))

		result, err := desc.Invoke(ctx, op.Input, state)
		if err != nil {
			observe.TraceCtx(ctx, "repl", "Tool.Invoke", "if: err != nil")
			results = append(results, fmt.Sprintf("[%d] %s: error: %v", i, op.Tool, err))
			continue
		}

		results = append(results, fmt.Sprintf("[%d] %s:\n%s", i, op.Tool, result.Content))
		allSupplements = append(allSupplements, result.Supplements...)
	}
	observe.TraceCtx(ctx, "repl", "Tool.Invoke", "return: tool.InvokeResult{\n\tContent:\tstrings.Join(results, \"\\n---\\n\"),\n\tSupplements:\t...")

	return tool.InvokeResult{
		Content:     strings.Join(results, "\n---\n"),
		Supplements: allSupplements,
	}, nil
}
