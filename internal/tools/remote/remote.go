package toolremote

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/remote"
	"github.com/artpar/pragma/internal/tool"
)

type remoteInput struct {
	Action    string          `json:"action" desc:"One of: list, get, create, update, run"`
	TriggerID string          `json:"trigger_id,omitempty" desc:"Required for get, update, run"`
	Body      json.RawMessage `json:"body,omitempty" desc:"JSON body for create and update"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
	"required": ["action"],
	"properties": {
		"action": {
			"type": "string",
			"enum": ["list", "get", "create", "update", "run"],
			"description": "The trigger action to perform"
		},
		"trigger_id": {
			"type": "string",
			"description": "Trigger ID (required for get, update, run)"
		},
		"body": {
			"type": "object",
	"additionalProperties": false,
			"description": "JSON body for create and update actions"
		}
	}
}`)

// TriggerService owns runtime-visible remote trigger execution.
type TriggerService interface {
	Execute(ctx context.Context, req remote.Request) (remote.Response, error)
}

// Tool manages scheduled remote agents through the runtime remote service.
type Tool struct {
	Service TriggerService
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"RemoteTrigger\"")
	return "RemoteTrigger"
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
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: false, Concurrent: true}
}

func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: `Manages scheduled remote agents (triggers). Actions: list (all triggers), ge...")
	return `Manages scheduled remote agents (triggers). Actions: list (all triggers), get (one trigger), create (new trigger), update (modify trigger), run (execute trigger now).`
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "toolremote", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "toolremote", "Tool.CheckPerm", "exit")
	var in remoteInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "toolremote", "Tool.CheckPerm", "if: err != nil")
		observe.TraceCtx(ctx, "toolremote", "Tool.CheckPerm", "return: checker.Check(ctx, \"RemoteTrigger\", \"\")")
		return checker.Check(ctx, "RemoteTrigger", "")
	}
	content := "RemoteTrigger " + in.Action
	if in.TriggerID != "" {
		observe.TraceCtx(ctx, "toolremote", "Tool.CheckPerm", "if: in.TriggerID != \"\"")
		content += " " + in.TriggerID
	}
	observe.TraceCtx(ctx, "toolremote", "Tool.CheckPerm", "return: checker.Check(ctx, \"RemoteTrigger\", content)")
	return checker.Check(ctx, "RemoteTrigger", content)
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "exit")
	var in remoteInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}

	if in.Action == "" {
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "if: in.Action == \"\"")
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"action is required\")")
		return tool.InvokeResult{}, fmt.Errorf("action is required")
	}

	if t.Service == nil {
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "if: t.Service == nil")
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"remote trigger service is not configured\")")
		return tool.InvokeResult{}, fmt.Errorf("remote trigger service is not configured")
	}

	resp, err := t.Service.Execute(ctx, remote.Request{
		Action:    remote.Action(in.Action),
		TriggerID: in.TriggerID,
		Body:      in.Body,
	})
	if err != nil {
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "return: tool.InvokeResult{}, err")
		return tool.InvokeResult{}, err
	}
	content := fmt.Sprintf("HTTP %d\n%s", resp.StatusCode, string(resp.Body))
	observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "return: tool.InvokeResult{Content: content}, nil")

	return tool.InvokeResult{Content: content}, nil
}
