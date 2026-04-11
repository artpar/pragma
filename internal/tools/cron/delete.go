package cron

import (
	"context"
	"encoding/json"
	"fmt"

	cronpkg "github.com/artpar/gogent/internal/cron"
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

func (t *DeleteTool) Name() string                { return "CronDelete" }
func (t *DeleteTool) Description() string          { return "Delete a scheduled cron job by ID." }
func (t *DeleteTool) InputSchema() json.RawMessage { return deleteSchema }
func (t *DeleteTool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: false, Concurrent: true}
}

func (t *DeleteTool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	var in struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &in); err == nil && in.ID != "" {
		return checker.Check(ctx, "CronDelete", in.ID)
	}
	return checker.Check(ctx, "CronDelete", "")
}

func (t *DeleteTool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in deleteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.ID == "" {
		return tool.InvokeResult{}, fmt.Errorf("id is required")
	}

	if err := t.Scheduler.Delete(in.ID); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("delete cron job: %w", err)
	}

	result := struct {
		ID string `json:"id"`
	}{ID: in.ID}
	data, _ := json.Marshal(result)
	return tool.InvokeResult{Content: string(data)}, nil
}
