package synthetic

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
)

// Tool implements the StructuredOutput tool for returning validated JSON output
// in non-interactive/SDK mode. The input schema is the user-provided JSON Schema,
// and the tool validates that the LLM's output conforms to it.
//
// Issue #37904: name kept neutral to avoid prompt-injection refusal.
// Issue #40022: tool is stateless — caller manages per-turn lifecycle.
// Issue #42828: validates in Invoke, does not trust model claims.
type Tool struct {
	Schema   json.RawMessage // user-provided JSON schema (returned by InputSchema)
	compiled *jsonschema.Schema
	once     sync.Once
	compErr  error
}

// New creates a SyntheticOutputTool with a pre-validated JSON schema.
func New(schema json.RawMessage) (*Tool, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	t := &Tool{Schema: schema}
	if err := t.compile(); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"invalid JSON schema: %w\", err)")
		return nil, fmt.Errorf("invalid JSON schema: %w", err)
	}
	observe.GlobalTrace("return: t, nil")
	return t, nil
}

func (t *Tool) compile() error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	t.once.Do(func() {
		var schemaObj any
		if err := json.Unmarshal(t.Schema, &schemaObj); err != nil {
			t.compErr = fmt.Errorf("schema is not valid JSON: %w", err)
			return
		}
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource("schema.json", schemaObj); err != nil {
			t.compErr = fmt.Errorf("invalid schema resource: %w", err)
			return
		}
		compiled, err := compiler.Compile("schema.json")
		if err != nil {
			t.compErr = fmt.Errorf("schema compilation failed: %w", err)
			return
		}
		t.compiled = compiled
	})
	observe.GlobalTrace("return: t.compErr")
	return t.compErr
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"StructuredOutput\"")
	return "StructuredOutput"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: syntheticDescription")
	return syntheticDescription
}

const syntheticDescription = `Use this tool to return a structured JSON response matching the required schema. Call this tool ONCE at the end of your response with the final output. The input to this tool must conform to the JSON schema provided as the tool's input schema.`

// InputSchema returns the user-provided JSON schema — the LLM sees it as this tool's input schema.
func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: t.Schema")
	return t.Schema
}

func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) CheckPerm(_ context.Context, _ json.RawMessage, _ permission.Checker) permission.CheckResult {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: permission.CheckResult{Decision: permission.DecisionAllow}")
	return permission.CheckResult{Decision: permission.DecisionAllow}
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "synthetic", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "synthetic", "Tool.Invoke", "exit")

	if err := t.compile(); err != nil {
		observe.TraceCtx(ctx, "synthetic", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "synthetic", "Tool.Invoke", "return: tool.InvokeResult{}, err")
		return tool.InvokeResult{}, err
	}

	var inputObj any
	if err := json.Unmarshal(input, &inputObj); err != nil {
		observe.TraceCtx(ctx, "synthetic", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "synthetic", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"input is not valid JSON: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("input is not valid JSON: %w", err)
	}

	err := t.compiled.Validate(inputObj)
	if err != nil {
		observe.TraceCtx(ctx, "synthetic", "Tool.Invoke", "if: err != nil")
		errMsg := err.Error()

		if len(errMsg) > 150 {
			observe.TraceCtx(ctx, "synthetic", "Tool.Invoke", "if: len(errMsg) > 150")
			errMsg = errMsg[:150]
		}
		observe.TraceCtx(ctx, "synthetic", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"output does not match required schema: %s\", ...")
		return tool.InvokeResult{}, fmt.Errorf("output does not match required schema: %s", errMsg)
	}
	observe.TraceCtx(ctx, "synthetic", "Tool.Invoke", "return: tool.InvokeResult{Content: \"Structured output provided successfully\"}, nil")

	return tool.InvokeResult{Content: "Structured output provided successfully"}, nil
}
