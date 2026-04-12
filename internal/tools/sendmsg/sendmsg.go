package sendmsg

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/gogent/internal/observe"
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

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"SendMessage\"")
	observe.GlobalTrace("return: \"SendMessage\"")
	return "SendMessage"
}
func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Send a message to a running or completed agent by name or task ID...\"")
	observe.GlobalTrace("return: sendMsgDescription")
	return sendMsgDescription
}

const sendMsgDescription = `Send a message to a running or completed agent by name or task ID. The agent will receive the message between turns and resume with its full context preserved.

Usage:
- Use the agent's name or task ID as the ` + "`to`" + ` field
- Your plain text output is NOT visible to other agents — to communicate, you MUST call this tool
- Refer to agents by name, never by UUID
- Use this to continue a previously spawned agent with follow-up instructions or additional context`

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "sendmsg", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "sendmsg", "Tool.CheckPerm", "exit")
	var in struct {
		To string `json:"to"`
	}
	if err := json.Unmarshal(input, &in); err == nil && in.To != "" {
		observe.TraceCtx(ctx, "sendmsg", "Tool.CheckPerm", "if: err == nil && in.To != \"\"")
		observe.TraceCtx(ctx, "sendmsg", "Tool.CheckPerm", "return: checker.Check(ctx, \"SendMessage\", in.To)")
		observe.TraceCtx(ctx, "sendmsg", "Tool.CheckPerm", "return: checker.Check(ctx, \"SendMessage\", in.To)")
		return checker.Check(ctx, "SendMessage", in.To)
	}
	observe.TraceCtx(ctx, "sendmsg", "Tool.CheckPerm", "return: checker.Check(ctx, \"SendMessage\", \"\")")
	observe.TraceCtx(ctx, "sendmsg", "Tool.CheckPerm", "return: checker.Check(ctx, \"SendMessage\", \"\")")
	return checker.Check(ctx, "SendMessage", "")
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var in sendInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.To == "" {
		observe.GlobalTrace("if: in.To == \"\"")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"to is required\")")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"to is required\")")
		return tool.InvokeResult{}, fmt.Errorf("to is required")
	}
	if in.Message == "" {
		observe.GlobalTrace("if: in.Message == \"\"")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"message is required\")")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"message is required\")")
		return tool.InvokeResult{}, fmt.Errorf("message is required")
	}

	tk, ok := t.Tasks.Get(in.To)
	if !ok {
		observe.GlobalTrace("if: !ok")
		tk, ok = t.Tasks.GetByName(in.To)
	}
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: tool.InvokeResult{Content: fmt.Sprintf(\"No agent found with name or ID %q\", i...")
		observe.GlobalTrace("return: tool.InvokeResult{Content: fmt.Sprintf(\"No agent found with name or ID %q\", i...")
		return tool.InvokeResult{Content: fmt.Sprintf("No agent found with name or ID %q", in.To)}, nil
	}

	if tk.Status != task.TaskRunning && tk.Status != task.TaskPending {
		observe.GlobalTrace("if: tk.Status != task.TaskRunning && tk.Status != task.TaskPending")
		observe.GlobalTrace("return: tool.InvokeResult{Content: fmt.Sprintf(\"Agent %q is %s, not running. Cannot d...")
		observe.GlobalTrace("return: tool.InvokeResult{Content: fmt.Sprintf(\"Agent %q is %s, not running. Cannot d...")
		return tool.InvokeResult{Content: fmt.Sprintf("Agent %q is %s, not running. Cannot deliver message.", in.To, tk.Status)}, nil
	}

	if err := t.Tasks.Update(tk.ID, func(tt *task.Task) {
		tt.PendingMessages = append(tt.PendingMessages, in.Message)
	}); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{Content: fmt.Sprintf(\"Failed to deliver message: %v\", err)}...")
		observe.GlobalTrace("return: tool.InvokeResult{Content: fmt.Sprintf(\"Failed to deliver message: %v\", err)}...")
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
	observe.GlobalTrace("return: tool.InvokeResult{Content: string(data)}, nil")
	observe.GlobalTrace("return: tool.InvokeResult{Content: string(data)}, nil")
	return tool.InvokeResult{Content: string(data)}, nil
}
