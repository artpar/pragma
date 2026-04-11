package cron

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	cronpkg "github.com/artpar/gogent/internal/cron"
	"github.com/artpar/gogent/internal/observe"
)

func setupScheduler(t *testing.T) *cronpkg.Scheduler {
	t.Helper()
	bus := observe.NewEventBus(16)
	t.Cleanup(func() { bus.Drain() })
	storePath := filepath.Join(t.TempDir(), "cron.json")
	store := cronpkg.NewStore(storePath)
	return cronpkg.NewScheduler(bus, store)
}

func TestCreateTool_Name(t *testing.T) {
	tl := &CreateTool{}
	if tl.Name() != "CronCreate" {
		t.Fatalf("expected CronCreate, got %s", tl.Name())
	}
}

func TestCreateTool_Invoke(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{"valid recurring", `{"cron":"*/5 * * * *","prompt":"check disk"}`, false, ""},
		{"valid non-recurring", `{"cron":"0 9 * * 1-5","prompt":"standup","recurring":false}`, false, ""},
		{"valid durable", `{"cron":"0 0 * * *","prompt":"backup","durable":true}`, false, ""},
		{"missing cron", `{"cron":"","prompt":"test"}`, true, "cron expression is required"},
		{"missing prompt", `{"cron":"*/5 * * * *","prompt":""}`, true, "prompt is required"},
		{"invalid cron", `{"cron":"bad","prompt":"test"}`, true, "create cron job"},
		{"invalid json", `{bad`, true, "invalid input"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched := setupScheduler(t)
			tl := &CreateTool{Scheduler: sched}
			result, err := tl.Invoke(context.Background(), json.RawMessage(tt.input), nil)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.errMsg)
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var out struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if !strings.HasPrefix(out.ID, "cron-") {
				t.Fatalf("expected cron- prefix, got %q", out.ID)
			}
		})
	}
}

func TestDeleteTool_Name(t *testing.T) {
	tl := &DeleteTool{}
	if tl.Name() != "CronDelete" {
		t.Fatalf("expected CronDelete, got %s", tl.Name())
	}
}

func TestDeleteTool_Invoke(t *testing.T) {
	sched := setupScheduler(t)
	createTool := &CreateTool{Scheduler: sched}
	deleteTool := &DeleteTool{Scheduler: sched}

	// Create a job first.
	result, err := createTool.Invoke(context.Background(), json.RawMessage(`{"cron":"0 0 * * *","prompt":"test"}`), nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(result.Content), &created)

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"delete existing", `{"id":"` + created.ID + `"}`, false},
		{"delete non-existent", `{"id":"cron-9999"}`, true},
		{"empty id", `{"id":""}`, true},
		{"invalid json", `{bad`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := deleteTool.Invoke(context.Background(), json.RawMessage(tt.input), nil)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestListTool_Name(t *testing.T) {
	tl := &ListTool{}
	if tl.Name() != "CronList" {
		t.Fatalf("expected CronList, got %s", tl.Name())
	}
}

func TestListTool_Invoke_Empty(t *testing.T) {
	sched := setupScheduler(t)
	tl := &ListTool{Scheduler: sched}
	result, err := tl.Invoke(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out struct {
		Jobs []json.RawMessage `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Jobs) != 0 {
		t.Fatalf("expected 0 jobs, got %d", len(out.Jobs))
	}
}

func TestListTool_Invoke_WithJobs(t *testing.T) {
	sched := setupScheduler(t)
	createTool := &CreateTool{Scheduler: sched}
	listTool := &ListTool{Scheduler: sched}

	// Create two jobs.
	createTool.Invoke(context.Background(), json.RawMessage(`{"cron":"0 0 * * *","prompt":"backup"}`), nil)
	createTool.Invoke(context.Background(), json.RawMessage(`{"cron":"*/10 * * * *","prompt":"health check"}`), nil)

	result, err := listTool.Invoke(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out struct {
		Jobs []struct {
			ID            string `json:"id"`
			HumanSchedule string `json:"human_schedule"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(out.Jobs))
	}
	for _, j := range out.Jobs {
		if j.HumanSchedule == "" {
			t.Fatalf("expected human_schedule for job %s", j.ID)
		}
	}
}
