package background

import (
	"os"

	"github.com/artpar/gogent/internal/observe"
)

// StatusSubscriber updates the PID file status based on engine events.
// Subscribes to the EventBus and fires updates as fire-and-forget.
type StatusSubscriber struct {
	registry *Registry
	pid      int
}

// NewStatusSubscriber creates a subscriber for the current process.
func NewStatusSubscriber(registry *Registry) *StatusSubscriber {
	return &StatusSubscriber{registry: registry, pid: os.Getpid()}
}

func (s *StatusSubscriber) HandleEvent(event observe.Event) {
	switch event.(type) {
	case observe.APIRequestStarted:
		s.registry.UpdateStatus(s.pid, StatusBusy)
	case observe.APIRequestCompleted:
		s.registry.UpdateStatus(s.pid, StatusIdle)
	case observe.APIRequestFailed:
		s.registry.UpdateStatus(s.pid, StatusIdle)
	case observe.ToolPermissionPrompted:
		s.registry.UpdateStatus(s.pid, StatusWaiting)
	}
}
