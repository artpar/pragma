package cron

import (
	"context"
	"encoding/json"

	cronpkg "github.com/artpar/gogent/internal/cron"
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

func (t *ListTool) Name() string                { return "CronList" }
func (t *ListTool) Description() string          { return "List all scheduled cron jobs." }
func (t *ListTool) InputSchema() json.RawMessage { return listSchema }
func (t *ListTool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *ListTool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
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
	jobs := t.Scheduler.List()

	entries := make([]jobEntry, len(jobs))
	for i, j := range jobs {
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
	return tool.InvokeResult{Content: string(data)}, nil
}
