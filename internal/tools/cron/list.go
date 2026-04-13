package cron

import (
	"context"
	"encoding/json"

	cronpkg "github.com/artpar/gogent/internal/cron"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

var listSchema = json.RawMessage(`{
	"type": "object",
	"properties": {}
}`)

// ListTool lists all scheduled cron jobs.
type ListTool struct {
	Scheduler *cronpkg.Scheduler
}

func (t *ListTool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"CronList\"")
	return "CronList"
}
func (t *ListTool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"List all scheduled cron jobs...\"")
	observe.GlobalTrace("return: \"List all scheduled cron jobs. Returns each job's ID, cron expression, prompt...")
	return "List all scheduled cron jobs. Returns each job's ID, cron expression, prompt, next fire time, and whether it is recurring or durable. Use this to check what is currently scheduled before creating or deleting jobs."
}
func (t *ListTool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: listSchema")
	return listSchema
}
func (t *ListTool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *ListTool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "cron", "ListTool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "cron", "ListTool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "cron", "ListTool.CheckPerm", "return: checker.Check(ctx, \"CronList\", \"\")")
	return checker.Check(ctx, "CronList", "")
}

type jobEntry struct {
	ID            string `json:"id"`
	Cron          string `json:"cron"`
	HumanSchedule string `json:"human_schedule"`
	Prompt        string `json:"prompt"`
	Recurring     bool   `json:"recurring"`
	Durable       bool   `json:"durable"`
}

func (t *ListTool) Invoke(_ context.Context, _ json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	jobs := t.Scheduler.List()

	entries := make([]jobEntry, len(jobs))
	for i, j := range jobs {
		observe.GlobalTrace("range jobs")
		entries[i] = jobEntry{
			ID:            j.ID,
			Cron:          j.Cron,
			HumanSchedule: cronpkg.ToHuman(j.Cron),
			Prompt:        j.Prompt,
			Recurring:     j.Recurring,
			Durable:       j.Durable,
		}
	}

	result := struct {
		Jobs []jobEntry `json:"jobs"`
	}{Jobs: entries}
	data, _ := json.Marshal(result)
	observe.GlobalTrace("return: tool.InvokeResult{Content: string(data)}, nil")
	return tool.InvokeResult{Content: string(data)}, nil
}
