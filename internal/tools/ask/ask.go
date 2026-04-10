package ask

import (
	"context"
	"encoding/json"
	"fmt"

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

func (t *Tool) Name() string                { return "AskUserQuestion" }
func (t *Tool) Description() string          { return "Ask the user a question and wait for their response." }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, "AskUserQuestion", "")
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in askInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Question == "" {
		return tool.InvokeResult{}, fmt.Errorf("question is required")
	}

	answer, err := t.Asker.Ask(ctx, in.Question)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("ask user: %w", err)
	}

	return tool.InvokeResult{Content: answer}, nil
}
