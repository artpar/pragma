package sendmsg

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/task"
	"github.com/artpar/gogent/internal/tool"
)

type sendInput struct {
	To      string `json:"to" desc:"Agent name or task ID to send message to"`
	Message string `json:"message" desc:"The message to send"`
	Summary string `json:"summary" desc:"5-10 word summary of the message (optional)"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["to", "message"],
	"properties": {
		"to": {
			"type": "string",
			"description": "The agent name or task ID to send the message to"
		},
		"message": {
			"type": "string",
			"description": "The message content to send"
		},
		"summary": {
			"type": "string",
			"description": "A 5-10 word summary of the message"
		}
	}
}`)

// Tool sends a message to a running or completed agent.
type Tool struct {
	Tasks *task.Registry
}

func (t *Tool) Name() string                { return "SendMessage" }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) Description() string {
	return "Send a message to a running or completed agent by name or task ID. The agent will receive the message between turns."
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	var in struct {
		To string `json:"to"`
	}
	if err := json.Unmarshal(input, &in); err == nil && in.To != "" {
		return checker.Check(ctx, "SendMessage", in.To)
	}
	return checker.Check(ctx, "SendMessage", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in sendInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.To == "" {
		return tool.InvokeResult{}, fmt.Errorf("to is required")
	}
	if in.Message == "" {
		return tool.InvokeResult{}, fmt.Errorf("message is required")
	}

	// Look up by task ID first, then by agent name
	tk, ok := t.Tasks.Get(in.To)
	if !ok {
		tk, ok = t.Tasks.GetByName(in.To)
	}
	if !ok {
		return tool.InvokeResult{Content: fmt.Sprintf("No agent found with name or ID %q", in.To)}, nil
	}

	if tk.Status != task.TaskRunning && tk.Status != task.TaskPending {
		return tool.InvokeResult{Content: fmt.Sprintf("Agent %q is %s, not running. Cannot deliver message.", in.To, tk.Status)}, nil
	}

	// Append message to pending queue — agent checks between turns
	if err := t.Tasks.Update(tk.ID, func(tt *task.Task) {
		tt.PendingMessages = append(tt.PendingMessages, in.Message)
	}); err != nil {
		return tool.InvokeResult{Content: fmt.Sprintf("Failed to deliver message: %v", err)}, nil
	}

	result := struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}{
		Success: true,
		Message: fmt.Sprintf("Message delivered to agent %q (task %s)", in.To, tk.ID),
	}
	data, _ := json.Marshal(result)
	return tool.InvokeResult{Content: string(data)}, nil
}
