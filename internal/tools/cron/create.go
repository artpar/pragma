package cron

import (
	"context"
	"encoding/json"
	"fmt"

	cronpkg "github.com/artpar/gogent/internal/cron"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

type createInput struct {
	Cron      string `json:"cron" desc:"5-field cron expression (minute hour dom month dow)"`
	Prompt    string `json:"prompt" desc:"The prompt to execute on schedule"`
	Recurring *bool  `json:"recurring" desc:"Whether the job repeats (default true)"`
	Durable   *bool  `json:"durable" desc:"Whether the job persists across restarts (default false)"`
}

var createSchema = json.RawMessage(`{
	"type": "object",
	"required": ["cron", "prompt"],
	"properties": {
		"cron": {
			"type": "string",
			"description": "A 5-field cron expression: minute hour day-of-month month day-of-week. Examples: '*/5 * * * *' (every 5 min), '0 9 * * 1-5' (9 AM weekdays)"
		},
		"prompt": {
			"type": "string",
			"description": "The prompt to execute when the schedule fires"
		},
		"recurring": {
			"type": "boolean",
			"description": "Whether the job repeats after firing (default true)"
		},
		"durable": {
			"type": "boolean",
			"description": "Whether the job persists across session restarts (default false)"
		}
	}
}`)

// CreateTool creates a new scheduled cron job.
type CreateTool struct {
	Scheduler *cronpkg.Scheduler
}

func (t *CreateTool) Name() string                { return "CronCreate" }
func (t *CreateTool) Description() string          { return "Schedule a prompt to run on a cron schedule." }
func (t *CreateTool) InputSchema() json.RawMessage { return createSchema }
func (t *CreateTool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: false, Concurrent: true}
}

func (t *CreateTool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	var in struct {
		Cron string `json:"cron"`
	}
	if err := json.Unmarshal(input, &in); err == nil && in.Cron != "" {
		return checker.Check(ctx, "CronCreate", in.Cron)
	}
	return checker.Check(ctx, "CronCreate", "")
}

func (t *CreateTool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in createInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Cron == "" {
		return tool.InvokeResult{}, fmt.Errorf("cron expression is required")
	}
	if in.Prompt == "" {
		return tool.InvokeResult{}, fmt.Errorf("prompt is required")
	}

	recurring := true
	if in.Recurring != nil {
		recurring = *in.Recurring
	}
	durable := false
	if in.Durable != nil {
		durable = *in.Durable
	}

	job, err := t.Scheduler.Create(in.Cron, in.Prompt, recurring, durable)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("create cron job: %w", err)
	}

	result := struct {
		ID            string `json:"id"`
		HumanSchedule string `json:"human_schedule"`
		Recurring     bool   `json:"recurring"`
		Durable       bool   `json:"durable"`
	}{
		ID:            job.ID,
		HumanSchedule: cronpkg.ToHuman(job.Cron),
		Recurring:     job.Recurring,
		Durable:       job.Durable,
	}
	data, _ := json.Marshal(result)
	return tool.InvokeResult{Content: string(data)}, nil
}
