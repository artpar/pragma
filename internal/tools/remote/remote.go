package toolremote

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
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

// Tool manages scheduled remote Pragma agents via the Anthropic CCR API.
type Tool struct {
	HTTPClient  *http.Client
	BaseURL     string
	TokenSource func() (string, error)
	OrgUUID     func() (string, error)
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

	var method, urlPath string
	var body io.Reader

	switch in.Action {
	case "list":
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "case: \"list\"")
		method = http.MethodGet
		urlPath = "/v1/code/triggers"

	case "get":
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "case: \"get\"")
		if in.TriggerID == "" {
			observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"trigger_id required for get\")")
			return tool.InvokeResult{}, fmt.Errorf("trigger_id required for get")
		}
		method = http.MethodGet
		urlPath = "/v1/code/triggers/" + in.TriggerID

	case "create":
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "case: \"create\"")
		if len(in.Body) == 0 {
			observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"body required for create\")")
			return tool.InvokeResult{}, fmt.Errorf("body required for create")
		}
		method = http.MethodPost
		urlPath = "/v1/code/triggers"
		body = strings.NewReader(string(in.Body))

	case "update":
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "case: \"update\"")
		if in.TriggerID == "" {
			observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"trigger_id required for update\")")
			return tool.InvokeResult{}, fmt.Errorf("trigger_id required for update")
		}
		if len(in.Body) == 0 {
			observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"body required for update\")")
			return tool.InvokeResult{}, fmt.Errorf("body required for update")
		}
		method = http.MethodPost
		urlPath = "/v1/code/triggers/" + in.TriggerID
		body = strings.NewReader(string(in.Body))

	case "run":
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "case: \"run\"")
		if in.TriggerID == "" {
			observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"trigger_id required for run\")")
			return tool.InvokeResult{}, fmt.Errorf("trigger_id required for run")
		}
		method = http.MethodPost
		urlPath = "/v1/code/triggers/" + in.TriggerID + "/run"

	default:
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "default")
		return tool.InvokeResult{}, fmt.Errorf("unknown action: %s", in.Action)
	}

	token, err := t.TokenSource()
	if err != nil {
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"get auth token: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("get auth token: %w", err)
	}
	orgUUID, err := t.OrgUUID()
	if err != nil {
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"get org UUID: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("get org UUID: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, t.BaseURL+urlPath, body)
	if err != nil {
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"create request: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "ccr-triggers-2026-01-30")
	req.Header.Set("x-organization-uuid", orgUUID)

	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"request failed: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"read response body: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("read response body: %w", err)
	}
	content := fmt.Sprintf("HTTP %d\n%s", resp.StatusCode, string(respBody))
	observe.TraceCtx(ctx, "toolremote", "Tool.Invoke", "return: tool.InvokeResult{Content: content}, nil")

	return tool.InvokeResult{Content: content}, nil
}
