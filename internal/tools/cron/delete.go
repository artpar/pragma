package cron

import (
	"context"
	"encoding/json"
	"fmt"

	cronpkg "github.com/artpar/gogent/internal/cron"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

type deleteInput struct {
	ID string `json:"id" desc:"The cron job ID to delete"`
}

var deleteSchema = json.RawMessage(`{
	"type": "object",
	"required": ["id"],
	"properties": {
		"id": {
			"type": "string",
			"description": "The cron job ID to delete (e.g. cron-1)"
		}
	}
}`)

// DeleteTool removes a scheduled cron job.
type DeleteTool struct {
	Scheduler *cronpkg.Scheduler
}

func (t *DeleteTool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"CronDelete\"")
	return "CronDelete"
}
func (t *DeleteTool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Delete a scheduled cron job by ID.\"")
	return "Delete a scheduled cron job by ID."
}
func (t *DeleteTool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: deleteSchema")
	return deleteSchema
}
func (t *DeleteTool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: false, Concurrent: true}
}

func (t *DeleteTool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "cron", "DeleteTool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "cron", "DeleteTool.CheckPerm", "exit")
	var in struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &in); err == nil && in.ID != "" {
		observe.TraceCtx(ctx, "cron", "DeleteTool.CheckPerm", "if: err == nil && in.ID != \"\"")
		observe.TraceCtx(ctx, "cron", "DeleteTool.CheckPerm", "return: checker.Check(ctx, \"CronDelete\", in.ID)")
		return checker.Check(ctx, "CronDelete", in.ID)
	}
	observe.TraceCtx(ctx, "cron", "DeleteTool.CheckPerm", "return: checker.Check(ctx, \"CronDelete\", \"\")")
	return checker.Check(ctx, "CronDelete", "")
}

func (t *DeleteTool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var in deleteInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.ID == "" {
		observe.GlobalTrace("if: in.ID == \"\"")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"id is required\")")
		return tool.InvokeResult{}, fmt.Errorf("id is required")
	}

	if err := t.Scheduler.Delete(in.ID); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"delete cron job: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("delete cron job: %w", err)
	}

	result := struct {
		ID string `json:"id"`
	}{ID: in.ID}
	data, _ := json.Marshal(result)
	observe.GlobalTrace("return: tool.InvokeResult{Content: string(data)}, nil")
	return tool.InvokeResult{Content: string(data)}, nil
}
