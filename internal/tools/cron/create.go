package cron

import (
	"context"
	"encoding/json"
	"fmt"

	cronpkg "github.com/artpar/pragma/internal/cron"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
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

func (t *CreateTool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"CronCreate\"")
	return "CronCreate"
}
func (t *CreateTool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: createDescription")
	return createDescription
}

const createDescription = `Schedule a prompt to run automatically on a cron schedule.

## Parameters

- cron: A standard 5-field cron expression (minute hour day-of-month month day-of-week). Examples: "*/5 * * * *" (every 5 min), "0 9 * * 1-5" (9 AM weekdays), "0 */2 * * *" (every 2 hours)
- prompt: The prompt to execute when the schedule fires
- recurring: Whether the job repeats after firing (default true). Set to false for one-shot schedules.
- durable: Whether the job persists across session restarts (default false)

## When to Use

- The user asks to run something on a schedule or interval
- The user wants periodic checks, polling, or recurring automation
- Use CronList to see existing jobs, CronDelete to remove them`

func (t *CreateTool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: createSchema")
	return createSchema
}
func (t *CreateTool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: false, Concurrent: true}
}

func (t *CreateTool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "cron", "CreateTool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "cron", "CreateTool.CheckPerm", "exit")
	var in struct {
		Cron string `json:"cron"`
	}
	if err := json.Unmarshal(input, &in); err == nil && in.Cron != "" {
		observe.TraceCtx(ctx, "cron", "CreateTool.CheckPerm", "if: err == nil && in.Cron != \"\"")
		observe.TraceCtx(ctx, "cron", "CreateTool.CheckPerm", "return: checker.Check(ctx, \"CronCreate\", in.Cron)")
		return checker.Check(ctx, "CronCreate", in.Cron)
	}
	observe.TraceCtx(ctx, "cron", "CreateTool.CheckPerm", "return: checker.Check(ctx, \"CronCreate\", \"\")")
	return checker.Check(ctx, "CronCreate", "")
}

func (t *CreateTool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var in createInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Cron == "" {
		observe.GlobalTrace("if: in.Cron == \"\"")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"cron expression is required\")")
		return tool.InvokeResult{}, fmt.Errorf("cron expression is required")
	}
	if in.Prompt == "" {
		observe.GlobalTrace("if: in.Prompt == \"\"")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"prompt is required\")")
		return tool.InvokeResult{}, fmt.Errorf("prompt is required")
	}

	recurring := true
	if in.Recurring != nil {
		observe.GlobalTrace("if: in.Recurring != nil")
		recurring = *in.Recurring
	}
	durable := false
	if in.Durable != nil {
		observe.GlobalTrace("if: in.Durable != nil")
		durable = *in.Durable
	}

	job, err := t.Scheduler.Create(in.Cron, in.Prompt, recurring, durable)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"create cron job: %w\", err)")
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
	observe.GlobalTrace("return: tool.InvokeResult{Content: string(data)}, nil")
	return tool.InvokeResult{Content: string(data)}, nil
}
