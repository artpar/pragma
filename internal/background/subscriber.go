package background

import (
	"os"
	"sync"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// StatusSubscriber updates the PID file status based on engine events.
// Subscribes to the EventBus and fires updates as fire-and-forget.
type StatusSubscriber struct {
	registry         *Registry
	pid              int
	mu               sync.Mutex
	activeAPI        int
	activeToolBatch  int
	pendingToolBatch bool
	waiting          bool
}

// NewStatusSubscriber creates a subscriber for the current process.
func NewStatusSubscriber(registry *Registry) *StatusSubscriber {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &StatusSubscriber{registry: registry, pid: os.Getpid()}")
	return &StatusSubscriber{registry: registry, pid: os.Getpid()}
}

func (s *StatusSubscriber) HandleEvent(event observe.Event) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch e := event.(type) {
	case observe.APIRequestStarted:
		observe.GlobalTrace("typecase: observe.APIRequestStarted")
		s.mu.Lock()
		s.activeAPI++
		s.waiting = false
		s.updateStatusLocked()
		s.mu.Unlock()
	case observe.APIRequestCompleted:
		observe.GlobalTrace("typecase: observe.APIRequestCompleted")
		s.mu.Lock()
		if s.activeAPI > 0 {
			s.activeAPI--
		}
		s.pendingToolBatch = e.StopReason == model.StopToolUse
		s.updateStatusLocked()
		s.mu.Unlock()
	case observe.APIRequestFailed:
		observe.GlobalTrace("typecase: observe.APIRequestFailed")
		s.mu.Lock()
		if s.activeAPI > 0 {
			s.activeAPI--
		}
		s.pendingToolBatch = false
		s.updateStatusLocked()
		s.mu.Unlock()
	case observe.ToolBatchStarted:
		observe.GlobalTrace("typecase: observe.ToolBatchStarted")
		s.mu.Lock()
		s.pendingToolBatch = false
		s.activeToolBatch++
		s.waiting = false
		s.updateStatusLocked()
		s.mu.Unlock()
	case observe.ToolBatchCompleted:
		observe.GlobalTrace("typecase: observe.ToolBatchCompleted")
		s.mu.Lock()
		if s.activeToolBatch > 0 {
			s.activeToolBatch--
		}
		s.updateStatusLocked()
		s.mu.Unlock()
	case observe.ToolExecutionStarted:
		observe.GlobalTrace("typecase: observe.ToolExecutionStarted")
		s.mu.Lock()
		s.waiting = false
		s.updateStatusLocked()
		s.mu.Unlock()
	case observe.ToolPermissionPrompted:
		observe.GlobalTrace("typecase: observe.ToolPermissionPrompted")
		s.mu.Lock()
		s.waiting = true
		s.updateStatusLocked()
		s.mu.Unlock()
	case observe.SessionStarted:
		observe.GlobalTrace("typecase: observe.SessionStarted")
		s.registry.UpdateSessionID(s.pid, e.SessionID)
	}
}

func (s *StatusSubscriber) updateStatusLocked() {
	switch {
	case s.waiting:
		s.registry.UpdateStatus(s.pid, StatusWaiting)
	case s.activeAPI > 0 || s.activeToolBatch > 0 || s.pendingToolBatch:
		s.registry.UpdateStatus(s.pid, StatusBusy)
	default:
		s.registry.UpdateStatus(s.pid, StatusIdle)
	}
}
