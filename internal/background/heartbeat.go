package background

import (
	"context"
	"time"

	"github.com/artpar/pragma/internal/observe"
)

// StartHeartbeat keeps the child-owned process record fresh while the runtime lives.
func StartHeartbeat(ctx context.Context, registry *Registry, pid int, ownerToken string) context.CancelFunc {
	observe.GlobalTrace("enter")
	if ctx == nil {
		ctx = context.Background()
	}
	heartbeatCtx, cancel := context.WithCancel(ctx)
	go func() {
		observe.GlobalTrace("background heartbeat start")
		defer observe.GlobalTrace("background heartbeat stop")
		registry.UpdateHeartbeat(pid, ownerToken)
		ticker := time.NewTicker(HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				registry.UpdateHeartbeat(pid, ownerToken)
			}
		}
	}()
	observe.GlobalTrace("return: cancel")
	return cancel
}
