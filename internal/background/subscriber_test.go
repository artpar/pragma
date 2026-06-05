package background

import (
	"os"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

func newTestStatusSubscriber(t *testing.T) (*Registry, *StatusSubscriber) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	reg, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	pid := os.Getpid()
	if err := reg.Register(ProcessInfo{
		PID:       pid,
		StartedAt: time.Now(),
		Status:    StatusStarting,
	}); err != nil {
		t.Fatal(err)
	}
	return reg, NewStatusSubscriber(reg)
}

func requireStatus(t *testing.T, reg *Registry, want Status) {
	t.Helper()
	info, err := reg.Get(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != want {
		t.Fatalf("status = %s, want %s", info.Status, want)
	}
}

func TestStatusSubscriberKeepsBusyBetweenToolUseResponseAndToolBatch(t *testing.T) {
	reg, sub := newTestStatusSubscriber(t)

	sub.HandleEvent(observe.APIRequestStarted{})
	requireStatus(t, reg, StatusBusy)

	sub.HandleEvent(observe.APIRequestCompleted{StopReason: model.StopToolUse})
	requireStatus(t, reg, StatusBusy)

	sub.HandleEvent(observe.ToolBatchStarted{TotalCount: 1})
	requireStatus(t, reg, StatusBusy)

	sub.HandleEvent(observe.ToolBatchCompleted{})
	requireStatus(t, reg, StatusIdle)
}

func TestStatusSubscriberSetsIdleAfterEndTurnResponse(t *testing.T) {
	reg, sub := newTestStatusSubscriber(t)

	sub.HandleEvent(observe.APIRequestStarted{})
	requireStatus(t, reg, StatusBusy)

	sub.HandleEvent(observe.APIRequestCompleted{StopReason: model.StopEndTurn})
	requireStatus(t, reg, StatusIdle)
}
