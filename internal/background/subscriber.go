package background

import (
	"os"

	"github.com/artpar/pragma/internal/observe"
)

// StatusSubscriber updates the PID file status based on engine events.
// Subscribes to the EventBus and fires updates as fire-and-forget.
type StatusSubscriber struct {
	registry *Registry
	pid      int
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
	switch event.(type) {
	case observe.APIRequestStarted:
		observe.GlobalTrace("typecase: observe.APIRequestStarted")
		s.registry.UpdateStatus(s.pid, StatusBusy)
	case observe.APIRequestCompleted:
		observe.GlobalTrace("typecase: observe.APIRequestCompleted")
		s.registry.UpdateStatus(s.pid, StatusIdle)
	case observe.APIRequestFailed:
		observe.GlobalTrace("typecase: observe.APIRequestFailed")
		s.registry.UpdateStatus(s.pid, StatusIdle)
	case observe.ToolPermissionPrompted:
		observe.GlobalTrace("typecase: observe.ToolPermissionPrompted")
		s.registry.UpdateStatus(s.pid, StatusWaiting)
	}
}
