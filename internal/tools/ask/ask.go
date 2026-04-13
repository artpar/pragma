package ask

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

type askInput struct {
	Question string `json:"question" desc:"The question to ask the user"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["question"],
	"properties": {
		"question": {
			"type": "string",
			"description": "The question to ask the user"
		}
	}
}`)

// Tool pauses execution and asks the user a question, returning their answer.
type Tool struct {
	Asker tool.Asker
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"AskUserQuestion\"")
	return "AskUserQuestion"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Asks the user a question to gather information, clarify ambiguity...\"")
	observe.GlobalTrace("return: askDescription")
	return askDescription
}

const askDescription = `Asks the user a question to gather information, clarify ambiguity, or get decisions.

Use this tool when you need to:
1. Gather user preferences or requirements
2. Clarify ambiguous instructions
3. Get decisions on implementation choices as you work
4. Offer choices to the user about what direction to take

Usage notes:
- Use this tool sparingly — prefer making reasonable decisions autonomously
- If you recommend a specific option, make that the first option
- In plan mode, use this tool to clarify requirements BEFORE finalizing your plan. Do NOT use this tool to ask "Is my plan ready?" — use ExitPlanMode for plan approval.`

func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: false}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "ask", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "ask", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "ask", "Tool.CheckPerm", "return: checker.Check(ctx, \"AskUserQuestion\", \"\")")
	return checker.Check(ctx, "AskUserQuestion", "")
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "ask", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "ask", "Tool.Invoke", "exit")
	var in askInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "ask", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "ask", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Question == "" {
		observe.TraceCtx(ctx, "ask", "Tool.Invoke", "if: in.Question == \"\"")
		observe.TraceCtx(ctx, "ask", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"question is required\")")
		return tool.InvokeResult{}, fmt.Errorf("question is required")
	}

	answer, err := t.Asker.Ask(ctx, in.Question)
	if err != nil {
		observe.TraceCtx(ctx, "ask", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "ask", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"ask user: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("ask user: %w", err)
	}
	observe.TraceCtx(ctx, "ask", "Tool.Invoke", "return: tool.InvokeResult{Content: answer}, nil")

	return tool.InvokeResult{Content: answer}, nil
}
